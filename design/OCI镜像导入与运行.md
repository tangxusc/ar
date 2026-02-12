# OCI 镜像导入与运行 — 实现方案

## 一、背景与目标

### 1.1 当前设计（`设计/加载流水线流程.md`）

- `ar load -i 流水线.tar.gz`（OCI 规范镜像）
- 当前描述：**调用 podman load api 加载镜像**，再 **podman run** 运行一次性容器；流水线结束后 **podman load** 宿主机目录下子镜像并清理。

### 1.2 目标

- **不调用 podman/buildah/skopeo 等命令**，在进程内**直接依赖 OCI 相关规范与实现**完成：
  1. **导入镜像**：从 OCI 归档（如 `流水线.tar.gz`）或 OCI 目录/Registry 将镜像导入到本地（如 `/var/lib/ar/镜像名/images/`）。
  2. **运行容器**：用已导入的镜像创建运行时 bundle（rootfs + config.json），再按 OCI Runtime Spec 启动容器（可与现有 libcontainer 方案衔接）。

---

## 二、OCI 规范分工

| 规范 | 作用 | 本项目用法 |
|------|------|------------|
| **OCI Image Spec** | 镜像格式：manifest、layers、config、index | 解析/生成镜像内容；导入时读写 OCI 镜像 |
| **OCI Distribution Spec** | Registry 拉取/推送协议 | 若需从 registry pull，则按该协议 |
| **OCI Runtime Spec** | 容器运行：bundle（rootfs + config.json）、state/kill/delete | 运行容器用 libcontainer（见《OCI容器操作改造计划》） |

**导入镜像**属于 **Image Spec** 范畴：把「镜像」（归档或 registry）变成**本地可用的镜像表示**（如 OCI layout 目录）。  
**运行容器**需要再把「镜像」**解包成 rootfs + 生成 config.json**，交给 **Runtime Spec**（libcontainer）执行。

---

## 三、导入镜像：实现方式

### 3.1 推荐：直接依赖 `github.com/containers/image`

- **containers/image** 是 Podman/Buildah/Skopeo 使用的同一套库，支持 OCI 与 Docker 格式，**不依赖任何 CLI**。
- 提供多种 **Transport**（来源/目标）：
  - **oci-archive**：单个 OCI 镜像的 tar（或 tar.gz）文件。
  - **oci**（dir）：本地 OCI layout 目录（`index.json` + `blobs/sha256/...`）。
  - **docker**：Docker Registry（pull/push）。
- 核心操作：**copy.Image(ctx, policyContext, destRef, srcRef, options)**，把镜像从 `srcRef` 复制到 `destRef`。

### 3.2 典型流程：从归档文件导入到本地目录

1. **源**：OCI 归档文件（如 `流水线.tar.gz`）。
   - 若为 `.tar.gz`：可先解压为 `.tar`，或使用支持流式解压的 API（部分版本支持）。
   - 引用形式：`oci-archive:/path/to/流水线.tar`（或库支持的路径形式）。
   - 使用：`ociarchive.Transport.ParseReference(...)` 或 `NewReference(filePath, imageName)` 得到 `srcRef`。

2. **目标**：本地 OCI layout 目录（与设计中的「宿主机 `/var/lib/ar/镜像名/images/`」一致）。
   - 例如：`/var/lib/ar/镜像名/images/流水线/`，目录内为 OCI layout（`index.json` + `blobs/sha256/...`）。
   - 使用：`dir.Transport.ParseReference("/var/lib/ar/镜像名/images/流水线")` 得到 `destRef`。

3. **策略与复制**
   - 创建 `signature.PolicyContext`（默认允许未签名镜像，或按需配置拒绝非签名）。
   - 调用 `copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{...})`。
   - 复制成功后，该目录即为「已导入」的 OCI 镜像，可供后续 unpack 与运行使用。

4. **多镜像 / 多架构**
   - 若归档内为 **image index**（多平台），可通过 `copy.Options.ImageListSelection` 选择：仅当前系统、或全部、或指定 digest。
   - 流水线场景多为单镜像，使用默认 `CopySystemImage` 即可。

### 3.3 依赖

- 在 `go.mod` 中增加：**`github.com/containers/image/v5`**（建议 v5，与当前 Podman/Buildah 主流一致）。
- 该库会间接引入 `github.com/opencontainers/image-spec` 等 OCI 类型定义。

### 3.4 与「加载流水线」的对应关系

- **ar load -i 流水线.tar.gz**：  
  - 输入：`流水线.tar.gz`（OCI 镜像归档）。  
  - 实现：用 **containers/image** 从 `oci-archive` 复制到 **dir**（如 `/var/lib/ar/<镜像名>/images/流水线/`）。  
  - 不调用 podman load；若为 gz，可先解压到临时 tar 再传路径，或使用库支持的压缩路径（视具体 API 文档而定）。
- **流水线执行完成后，加载宿主机目录下所有子镜像**：  
  - 遍历 `/var/lib/ar/镜像名/images/` 下各子目录（或约定好的 `.tar` 文件），对每个子镜像做同样操作：**srcRef = oci-archive 或 dir**，**destRef = 本地某 OCI layout 目录**（或统一导入到同一 store 的多个 tag）。  
  - 加载失败则返回错误；加载完成后按设计清理目录。

---

## 四、从镜像到运行容器（unpack + runtime）

导入后得到的是 **OCI 镜像**（manifest + layers + config），而 **libcontainer 需要的是 bundle**（rootfs 目录 + config.json）。因此需要一步「**unpack**」：

1. **取层并解压到 rootfs**
   - 用 **containers/image** 打开已导入的 `destRef`，得到 `types.Image`，从中读取 **Layer 列表** 与 **Config**。
   - 按顺序将每一层解压到同一 rootfs 目录（上层覆盖下层）。层 blob 通常在 `blobs/sha256/<digest>`，格式多为 tar，可用 `archive.DecompressStream` 等解压并 Apply 到目录。

2. **生成 config.json（OCI Runtime Spec）**
   - 根据镜像 **Config**（entrypoint、cmd、env、user、labels 等）生成 **OCI Runtime 的 config.json**（process、root、mounts、linux 等）。
   - 可参考 **github.com/opencontainers/runtime-spec/specs-go** 的类型，或使用 **runc 的 specconv**（若依赖 runc 库）从 OCI image config 转成 libcontainer 的 `configs.Config`。

3. **创建并启动容器**
   - 使用 **libcontainer**：`libcontainer.Create(ociRuntimeRoot, containerID, config)`，再 `container.Run(process)` 或 `Start` + 等待。
   - 挂载点：按设计将宿主机 `/var/lib/ar/pipelines/`、`/var/lib/ar/镜像名/images/` 等挂载到容器内，在 `config.Mounts` 中配置。

若希望减少自行处理层与 config 的复杂度，可考虑：
- **containers/storage**：在本地维护镜像与容器层，提供「从 image 创建 container 层 + 拿到 rootfs 路径」的 API，再配合 libcontainer 使用；或  
- **containerd**：用 containerd 拉取/导入镜像并 unpack，用 containerd 的 runc 驱动创建 task（即 runc 容器），由 containerd 管理生命周期。

本方案优先说明「**导入镜像**」；unpack 与运行可在同一设计下拆为后续步骤实现。

---

## 五、实施清单（导入镜像部分）

| 步骤 | 内容 |
|------|------|
| 1 | 在 `go.mod` 增加 `github.com/containers/image/v5`，执行 `go mod tidy`。 |
| 2 | 新增包（如 `pkg/ociimage` 或 `pkg/importer`）：封装「从 oci-archive 或 dir 复制到 dir」的逻辑；输入为源路径（归档或 OCI 目录）、目标 OCI layout 目录。 |
| 3 | 实现「单次导入」：`ParseReference`（oci-archive / dir）→ `copy.Image` → 目标 dir；处理 gz 时先解压或使用库支持形式。 |
| 4 | 在 `ar load` 或对应命令中调用上述封装：`-i 流水线.tar.gz` → 导入到 `/var/lib/ar/<镜像名>/images/流水线/`（或当前设计约定的路径）。 |
| 5 | 实现「加载目录下所有子镜像」：遍历指定目录，对每个子镜像执行导入；失败即返回错误，成功后再清理目录。 |
| 6 | 策略与安全：配置 `signature.PolicyContext`（例如默认 InsecureAcceptAnything 仅用于内网，或按需严格校验签名）。 |

---

## 六、小结

- **导入镜像**：不调 podman/skopeo，直接使用 **containers/image** 的 **copy.Image**，从 **oci-archive**（或 dir/registry）复制到本地 **OCI layout 目录**，即完成「按 OCI 规范导入」。
- **运行容器**：导入后的镜像是「镜像层 + config」；需要再 **unpack 成 rootfs** 并 **生成 runtime config.json**，用 **libcontainer**（或 containerd）按 OCI Runtime Spec 创建并启动容器。
- 与《OCI容器操作改造计划》一致：**容器生命周期**用 libcontainer；**镜像的获取与存储**用 OCI Image Spec + **containers/image** 实现。
