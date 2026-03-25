# lvscare VIP 不通故障排查与修复

## 故障情况

- **环境**：Kubernetes 集群，网络插件使用 Cilium，apiserver 负载均衡使用 lvscare
- **VIP**：`169.254.1.5:6443`（link-local 地址，仅本机使用）
- **Real Server（RS）**：`172.19.16.14:6443`
- **问题节点**：`119.28.180.201`（worker 节点，内网 IP `172.19.16.11`）

## 故障表现

- `ping 169.254.1.5` 成功，VIP 本机可达
- `curl https://169.254.1.5:6443/healthz` 无响应，超时
- `curl https://172.19.16.14:6443/healthz` 返回 `ok`，RS 直连正常
- `ipvsadm -Ln` 规则存在，VIP → RS 转发配置正确
- lvscare 容器运行正常，IPVS 模块已加载，ipset 和 iptables 规则均已写入

## 排查确认过程

### 1. 确认 VIP 和 IPVS 规则

```bash
$ ip addr show lvscare
4: lvscare: <BROADCAST,NOARP,UP,LOWER_UP> ...
    inet 169.254.1.5/32 scope global lvscare

$ ipvsadm -Ln
TCP  169.254.1.5:6443 rr
  -> 172.19.16.14:6443  Masq  1  57  0
```

VIP 存在，IPVS 规则正确，RS 权重正常。

### 2. 确认 ipset 和 iptables 规则

```bash
$ ipset list VIRTUAL-IP
Members:
169.254.1.5,tcp:6443

$ iptables -t nat -S VIRTUAL-POSTROUTING
-A VIRTUAL-POSTROUTING -m mark ! --mark 0x1 -j RETURN
-A VIRTUAL-POSTROUTING -j MASQUERADE --random-fully
```

ipset 存在，iptables 链规则存在，表面上看 MASQUERADE 配置无误。

### 3. 通过 conntrack 发现关键线索

```bash
$ conntrack -L -d 169.254.1.5
tcp  SYN_SENT src=169.254.1.5 dst=169.254.1.5 sport=55358 dport=6443 [UNREPLIED]
                               src=172.19.16.14 dst=169.254.1.5 sport=6443 dport=55358
```

所有连接均为 `[UNREPLIED]` 状态，说明 RS 的回包从未到达本机。
conntrack 期望的回包 `dst=169.254.1.5` 表明 SNAT **没有生效**——包以 `src=169.254.1.5` 发出，RS 回包目标为 `169.254.1.5`，在 RS 节点上该地址不存在，回包被丢弃。

### 4. 定位 MASQUERADE 失效原因

lvscare 的包转发流程：

1. `nat/OUTPUT`：lvscare 给包打 mark `0x1`，命中 `VIRTUAL-SERVICES` → 打 `VIRTUAL-MARK-MASQ`
2. `nat/POSTROUTING`：进入 `VIRTUAL-POSTROUTING`

但 Cilium 在 `filter/OUTPUT` 执行：

```
-A CILIUM_OUTPUT ... -j MARK --set-xmark 0xc00/0xf00
```

该规则在 lvscare 打完 mark 之后，对 mark 做 `set-xmark 0xc00/0xf00`，结果：

```
0x0001 | (0xc00 & ~0xf00) = 0x0001 | 0xc00 = 0x0c01
```

mark 从 `0x1` 变为 `0xc01`。

`VIRTUAL-POSTROUTING` 第 1 条规则是精确匹配（无掩码）：

```
-m mark ! --mark 0x1 -j RETURN
```

`0xc01 ≠ 0x1`，条件成立 → 执行 RETURN → MASQUERADE 被跳过 → SNAT 不生效。

### 5. 验证修复

将 RETURN 规则改为带掩码匹配，只检查低位：

```bash
iptables -t nat -D VIRTUAL-POSTROUTING -m mark ! --mark 0x1 -j RETURN
iptables -t nat -I VIRTUAL-POSTROUTING 1 -m mark ! --mark 0x1/0x1 -j RETURN
```

修复后立即测试：

```bash
$ curl -sk --connect-timeout 3 https://169.254.1.5:6443/healthz
ok
```

VIP 恢复正常。

## 根本原因

lvscare 与 Cilium 共存时，Cilium 在 `filter/OUTPUT` 链通过 `set-xmark 0xc00/0xf00` 修改了 lvscare 打的 mark `0x1`，使其变为 `0xc01`。`VIRTUAL-POSTROUTING` 使用精确匹配 `! --mark 0x1` 判断是否需要 MASQUERADE，精确匹配在 mark 被修改后失败，导致 MASQUERADE 跳过，SNAT 不生效，RS 回包无法送达。

## 解决方案

在 `install-lvs-care.sh` 中，lvscare 容器启动后执行以下修复逻辑：

```bash
fix_virtual_postrouting() {
  # 检查 VIRTUAL-POSTROUTING 链是否存在
  if ! sudo iptables -t nat -L VIRTUAL-POSTROUTING -n >/dev/null 2>&1; then
    return 0
  fi
  # 检查是否存在无掩码的精确匹配规则
  if sudo iptables -t nat -S VIRTUAL-POSTROUTING | grep -qF '! --mark 0x1 -j RETURN'; then
    echo "修复 VIRTUAL-POSTROUTING：将 mark 精确匹配改为带掩码匹配（兼容 Cilium）"
    sudo iptables -t nat -D VIRTUAL-POSTROUTING -m mark ! --mark 0x1 -j RETURN
    sudo iptables -t nat -I VIRTUAL-POSTROUTING 1 -m mark ! --mark 0x1/0x1 -j RETURN
    echo "VIRTUAL-POSTROUTING 修复完成"
  fi
}

fix_virtual_postrouting
```

**修复原理**：`! --mark 0x1/0x1` 带掩码匹配，只检查 mark 的最低位是否为 `1`。lvscare 打的 mark `0x1` 低位为 `1`，即使 Cilium 追加了高位，低位仍为 `1`，条件不成立，不执行 RETURN，MASQUERADE 正常执行。

## 注意事项

- 此问题仅在 **lvscare + Cilium 同时部署**时出现，单独使用任意一个均不受影响
- 若后续 lvscare 或 Cilium 版本更新修改了各自的 mark 策略，需重新确认兼容性
- 该修复已内置于 `pipelines/containerd-k8s-1.35.0-amd64-v2/steps/base/ar/scripts/install-lvs-care.sh`，幂等执行，重复运行无副作用
