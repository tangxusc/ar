# containerd-k8s-1.35.0-amd64 Design

## 1. 目标

本目录用于实现一条通过 `allrun pipeline run` 执行的 Kubernetes `v1.35.0` 二进制安装流水线，目标特性如下：

- 运行时使用 `containerd`
- Kubernetes 组件通过二进制方式安装
- 网络插件使用 `Cilium`，通过 `cilium CLI` 安装
- apiserver 负载均衡使用 `lvs-care`，通过 `static pod` 方式启动
- 支持离线环境
- 支持带 `registry=true` 标签的节点部署本地镜像仓库
- 不修改当前 `backend/pkg/pipeline/run_pipeline.go` 的执行模型

## 2. 当前执行模型约束

### 2.1 `pipeline run` 运行目录

`allrun pipeline run` 执行时，会创建单次任务运行目录：

```text
/var/lib/ar/tasks/<pipelineName>/<taskID>/
```

例如：

```text
/var/lib/ar/tasks/containerd-k8s-1_35_0-amd64/1772378271218914836_4916/
```

步骤容器运行时：

- `/tasks` 挂载到本次任务的 `runDir`
- `/current-task` 挂载到当前步骤独占的 `nodeDir`

因此：

- `/tasks` 只用于保存本次任务的动态产物
- `/current-task` 只用于保存当前步骤的临时文件

### 2.2 不允许修改当前 `pipeline run`

当前设计明确约束：

- 不修改 `backend/pkg/pipeline/run_pipeline.go`
- 不增加运行期额外注入 `artifacts` 的后端逻辑

因此离线制品不能依赖“流水线镜像根目录注入”，必须由各个 `step` 镜像自行携带。

## 3. 总体设计原则

### 3.1 `artifacts` 在 step 镜像中

每个步骤镜像都可以携带自己的 `/artifacts`：

- `/artifacts/bin`
- `/artifacts/tarballs`
- `/artifacts/images`
- `/artifacts/manifests`
- `/artifacts/checksums`

运行时步骤容器直接从自身镜像内的 `/artifacts/...` 读取离线制品，再通过 `scp`、`ssh`、`ctr`、`nerdctl`、`skopeo` 等方式分发或安装到目标节点。

### 3.2 `/tasks` 只保存运行态数据

`/tasks` 中只保存动态结果，不承载 step 自带的静态离线制品。建议保留这些动态目录：

- `/tasks/cluster/`
- `/tasks/inventory/`
- `/tasks/rendered/`
- `/tasks/pki/`
- `/tasks/kubeconfig/`
- `/tasks/bootstrap/`
- `/tasks/reports/`
- `/tasks/diagnostics/`

### 3.3 节点标签约定

节点标签采用 `key/value` 结构。

当前设计保留如下语义：

- `role=master`
- `role=worker`
- `registry=true`

其中：

- `master` 节点承担控制面角色
- `worker` 节点承担工作节点角色
- `registry=true` 表示该节点同时承担本地镜像仓库角色

## 4. 组件与版本基线

优先采用设计文档中已经验证过的版本组合：

- `containerd` `2.2.1`
- `cni-plugins` `v1.9.0`
- `crictl` `v1.35.0`
- `etcd` `v3.6.7`
- `cfssl` `1.6.5`
- `kubernetes-server` `1.35.0`
- `runc` `1.4.0`
- `cilium CLI` 固定版本
- `lvs-care` 固定版本
- `helm` `4.0.4`

## 5. step 镜像目录设计

推荐结构：

```text
steps/<step>/
├── Dockerfile
├── Makefile
├── artifacts/
│   ├── bin/
│   ├── tarballs/
│   ├── images/
│   ├── manifests/
│   └── checksums/
├── scripts/
│   └── remote/
├── templates/
│   ├── systemd/
│   ├── config/
│   ├── pki/
│   └── manifests/
└── metadata/
    ├── versions.lock
    └── manifest.json
```

其中：

- `artifacts/` 是运行期静态离线制品
- `scripts/remote/` 是远程节点执行脚本
- `templates/` 是配置渲染输入
- `metadata/` 保存制品来源与版本元数据

## 6. 离线制品设计

### 6.1 二进制与压缩包

离线二进制和压缩包包括：

- `containerd-2.2.1-linux-amd64.tar.gz`
- `cni-plugins-linux-amd64-v1.9.0.tgz`
- `crictl-v1.35.0-linux-amd64.tar.gz`
- `kubernetes-server-linux-amd64.tar.gz`
- `etcd-v3.6.7-linux-amd64.tar.gz`
- `cilium-linux-amd64.tar.gz`
- `lvscare-linux-amd64.tar.gz`
- `runc`
- `cfssl`
- `cfssljson`
- `skopeo`

### 6.2 容器镜像离线包

需要离线准备并随步骤镜像携带的镜像包括：

- `registry`
- `pause`
- `cilium`
- `hubble`
- `coredns`
- `lvscare`
- 可选组件镜像

### 6.3 镜像包格式

step 镜像中的离线镜像包需兼容：

- `docker-archive`
- `oci-archive`

每个镜像包必须在元数据中记录：

- `archive_type`
- `target_registry`
- `target_repository`
- `target_image`
- `target_tag`

不依赖文件后缀推断格式，以 `manifest.json` 中的 `archive_type` 为准。

## 7. registry 设计

### 7.1 registry 节点选择规则

如果某个 `master` 或 `worker` 节点标签中包含 `registry=true`：

- 该节点在 containerd 安装后启动本地 `docker registry`
- 该节点作为本地镜像仓库 mirror 候选

如果存在多个 `registry=true` 节点：

- 全部节点都启动 registry
- 全部都作为 mirror 候选
- 所有节点的 `hosts.toml` 中写入多个 registry endpoint
- 离线镜像通过 `skopeo` 推送到全部 registry 节点

### 7.2 registry 地址规则

固定采用：

```text
registryNodeIP:5000
```

### 7.3 registry 启动方式

registry 使用 `nerdctl` 启动。

要求：

- 先通过 `ctr image import` 导入离线 `registry` 镜像
- 再通过 `nerdctl` 启动 registry 容器
- 容器数据目录必须映射到：

```text
/var/lib/registry/
```

即：

- 容器内 `/var/lib/registry`
- 节点上 `/var/lib/registry`

这样可避免节点重启后仓库内容丢失。

### 7.4 registry 启动顺序

registry 节点上的本地 registry 启动顺序固定为：

1. 安装并启动 containerd
2. `ctr image import` 导入 `registry` 镜像
3. 使用 `nerdctl` 启动本地 `docker registry`
4. 使用 `skopeo` 推送其余离线镜像到本地 registry
5. 回写所有节点的 `/etc/containerd/certs.d/_default/hosts.toml`
6. 重启或 reload containerd 使配置生效

## 8. containerd 镜像源配置

### 8.1 默认配置位置

统一使用：

```text
/etc/containerd/certs.d/_default/hosts.toml
```

### 8.2 多 registry mirror 配置

多 registry 节点时，`hosts.toml` 中写多个 endpoint。

示例：

```toml
# /etc/containerd/certs.d/_default/hosts.toml

[host."http://10.0.0.11:5000"]
  capabilities = ["pull", "resolve"]
  skip_verify = true

[host."http://10.0.0.12:5000"]
  capabilities = ["pull", "resolve"]
  skip_verify = true
```

第一版可先按内网 HTTP mirror 处理。

## 9. `skopeo copy` 模板

推送离线镜像到 registry 时，按 `archive_type` 选择源格式。

### 9.1 `docker-archive`

```bash
skopeo copy docker-archive:/path/to/your-image.tar.gz docker://<目标Registry地址>/<项目>/<镜像名>:<标签>
```

### 9.2 `oci-archive`

```bash
skopeo copy oci-archive:/path/to/your-image.tar.gz docker://<目标Registry地址>/<项目>/<镜像名>:<标签>
```

### 9.3 推送规则

- 遍历全部 `registry=true` 节点
- 对每个 registry 节点执行一次 `skopeo copy`
- 任一 registry 推送失败则步骤失败

## 10. HA 设计

### 10.1 apiserver 负载均衡

apiserver 负载均衡组件使用：

- `lvs-care`

启动方式：

- `static pod`

static pod manifest 下发位置：

```text
/etc/kubernetes/manifests/lvscare.yaml
```

### 10.2 `lvs-care` 关键参数

建议使用：

- `care`
- `--vs <vip:6443>`
- 多个 `--rs <master-ip:6443>`
- `--health-path /healthz`
- `--interval 5`
- `--mode link`

### 10.3 `lvs-care` 启动时序

重要约束：

- VIP 可以提前生成
- kubelet 配置中直接使用 VIP
- `lvs-care` static pod manifest 可以提前写入
- 但 static pod 真正启动依赖 master kubelet

## 11. kubelet 时序

当前明确采用如下顺序：

1. 安装控制面 systemd 组件
2. `install-kubelet-master-*`
3. `verify-ha`
4. `install-kubelet-worker-*`

也就是说：

- master kubelet 先启动
- 先让 `lvs-care` static pod 真正被 kubelet 托管
- HA 验证通过后
- 再启动 worker kubelet

## 12. Cilium 设计

### 12.1 安装方式

网络插件固定使用：

- `cilium CLI`

### 12.2 离线制品

需要离线准备：

- `cilium CLI` 二进制包
- `cilium` 组件镜像
- `hubble` 组件镜像
- `install.env` 或 values 片段

### 12.3 与 `lvs-care` 兼容

需要显式调整参数，使其兼容 `lvs-care`，例如：

- `prepend-iptables-chains: false`

第一版应通过离线配置片段把这类兼容项固定下来。

## 13. CoreDNS 设计

CoreDNS 需要：

- 离线 chart 或静态配置
- 离线镜像包

安装时保证：

- `clusterDNS`
- `serviceCIDR`
- `CoreDNS service IP`

三者一致。

## 14. 建议的流水线阶段

推荐主链路如下：

1. `print-plan`
2. `validate-inventory`
3. `precheck-all-*`
4. `os-baseline-all-*`
5. `all-runtime-binaries-*`
6. `all-install-containerd-*`
7. `registry-install-*`
8. `registry-push-images-*`
9. `all-configure-containerd-registry-*`
10. `master-binaries-*`
11. `worker-binaries-*`
12. `generate-pki-master01`
13. `distribute-pki-and-kubeconfig-*`
14. `render-etcd-config-master-*`
15. `install-etcd-master-*`
16. `verify-etcd`
17. `render-ha`
18. `install-ha-master-*`
19. `install-apiserver-master-*`
20. `install-controller-manager-master-*`
21. `install-scheduler-master-*`
22. `apply-bootstrap-rbac-master01`
23. `render-kubelet-all-*`
24. `install-kubelet-master-*`
25. `verify-ha`
26. `install-kubelet-worker-*`
27. `verify-node-bootstrap`
28. `install-kube-proxy-all-*`
29. `install-cni`
30. `install-coredns`
31. `install-optional-addons`
32. `verify-cluster-final`

## 15. Makefile 目标建议

顶层建议包含这些目标：

- `prepare-dirs`
- `download-artifacts`
- `verify-artifacts`
- `package-artifacts`
- `build-step-images`
- `install-local-pipeline`
- `build-pipeline-image`
- `build`
- `dist`
- `test`

其中：

- `download-artifacts` 负责下载所有 step 所需离线二进制、离线镜像包和配置片段
- `package-artifacts` 负责把离线制品分配到各 step 的构建上下文
- `build-step-images` 负责构建每个 step 镜像，并将各自 `/artifacts` 打入镜像
- `build-pipeline-image` 只负责模板和步骤镜像引用，不承担运行时离线制品注入职责

## 16. 最终边界

必须始终保持以下边界清晰：

- step 镜像中的 `/artifacts` 是静态离线制品
- `/tasks` 是运行态动态产物
- `/current-task` 是当前步骤临时目录
- pipeline image 不承担运行期 `artifacts` 载体职责
- 不修改当前 `pipeline run` 执行模型
