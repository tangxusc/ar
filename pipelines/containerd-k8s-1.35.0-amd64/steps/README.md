# K8s 节点初始化流水线步骤

本目录包含用于初始化 Kubernetes 节点的流水线步骤，与设计文档 `design/v1.35.0-CentOS-binary-install-IPv6-IPv4-Three-Masters-Two-Slaves-Offline.md` 中「1.环境」基础系统环境配置对应。

## 步骤顺序与说明

| 步骤文件 | 说明 |
|---------|------|
| `01-disable-firewall.json` | 关闭防火墙（CentOS: firewalld，Ubuntu 可忽略） |
| `02-disable-selinux.json` | 关闭 SELinux（仅 CentOS） |
| `03-disable-swap.json` | 关闭交换分区并设置 vm.swappiness=0 |
| `04-network-config.json` | 配置 NetworkManager 忽略 Calico 相关网卡 |
| `05-chrony-sync.json` | 配置 chrony 时间同步（客户端，指向 192.168.1.31） |
| `06-ulimit.json` | 配置 ulimit 与 limits.conf（nofile、nproc、memlock） |
| `07-ipvs-modules.json` | 加载 IPVS 及相关内核模块 |
| `08-sysctl.json` | 写入 k8s 内核参数 `/etc/sysctl.d/k8s.conf` 并 `sysctl --system` |
| `09-hosts.json` | 配置 `/etc/hosts`（master/node 主机名与 IPv4/IPv6） |

步骤间通过 `nodes` 字段串联为顺序 DAG：`01` → `02` → … → `09`。

## 使用方式

- **环境变量**：每个步骤均支持模板变量 `{{node_ip}}`、`{{node_port}}`、`{{node_username}}`、`{{node_password}}`、`{{node_labels}}`，用于在目标节点上执行（通常由执行器通过 SSH 连接 `target_host`）。
- **合并为流水线模板**：在流水线目录下执行  
  `jq -s '.' steps/01-*.json steps/02-*.json ... steps/09-*.json > init-nodes.template.json`  
  可生成完整 DAG 模板；或使用仓库内已有的 `init-nodes.template.json`（若已提供）。

## 注意事项

- 步骤默认通过 SSH 在目标节点执行命令，运行器镜像需具备 `ssh` 客户端；若使用密码认证，需配合 `sshpass` 或密钥。
- 部分步骤（如防火墙、SELinux、chrony）在 Ubuntu 上可能无对应服务，步骤中已用 `|| true` 等做容错。
- `09-hosts` 中 IP 与主机名按设计文档示例填写，实际部署请按环境修改或通过变量注入。
