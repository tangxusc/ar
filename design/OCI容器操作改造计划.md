# 使用 OpenContainer (OCI) 规范操作容器 — 详细计划

## 一、现状与目标

### 1.1 当前实现（`backend/pkg/web/cmd.go`）

- **stop 命令**：停止后端时可选地「移除以 `ar_` 开头的 buildah 工作容器」并停止本地 gin server。
- **实现方式**：
  - 使用 **buildah 专有接口**：`buildah containers --json` 列出容器，`buildah rm <id>` 删除。
  - 类型 `buildahContainerEntry` 解析 buildah 的 JSON 输出（id, containername, builder, imageid, imagename）。
  - 通过 `--buildah-container-remove` 控制是否移除容器，`--container-prefix` 指定前缀。

### 1.2 设计意图（`design/停止流程.md`）

- 停止后端时「调用 **runc api**，停止以 `ar_` 开头的所有容器」。
- 即：从 **buildah 专有** 改为 **OCI 标准** 的容器操作。

### 1.3 目标（明确：直接依赖 OCI 规范实现，不调 CLI）

- 在 `backend/pkg/web/cmd.go` 中改为依据 **Open Container Initiative (OCI) Runtime Specification** 操作容器。
- **不调用 runc/buildah 等命令行**，而是**直接依赖 OCI 规范的 Go 实现**（如 `github.com/opencontainers/runc/libcontainer`），在进程内完成：**state、kill、delete** 等规范定义的操作。

---

## 二、OCI 规范要点（与本改造相关）

### 2.1 规范中的容器操作

| 操作    | 含义                     | 本方案所用实现（libcontainer） |
|---------|--------------------------|--------------------------------|
| **state**  | 查询容器当前状态         | `container.State()` / `container.OCIState()` |
| **create** | 根据 bundle 创建容器     | `libcontainer.Create(root, id, config)`（本项目 stop 流程不需） |
| **start**  | 启动已创建容器           | `container.Run(process)`（本项目 stop 流程不需） |
| **kill**   | 向容器进程发信号         | `container.Signal(signal)` |
| **delete** | 删除容器并释放资源       | `container.Destroy()` |
| **list**   | 列出容器                 | `os.ReadDir(root)` 取 ID 列表 + `libcontainer.Load(root, id)` |

### 2.2 容器状态与标识

- **id**：容器唯一标识（我们按前缀 `ar_` 过滤）。
- **status**：`creating` | `created` | `running` | `stopped`。
- **bundle**：容器 bundle 目录（含 `config.json` + rootfs）。
- **root**：runc 存放容器状态的根目录，默认 `/run/runc`（或 rootless 时 `$XDG_RUNTIME_DIR/runc`）。

### 2.3 与本项目的关系

- **停止流程**只需：**list → 按前缀筛选 → (可选) kill → delete**。
- 不需要在本命令中实现 create/start；创建/启动容器由其它流程（如 buildah 或其它工具）完成，只要最终由 runc 管理即可。

---

## 三、方案：直接依赖 OCI 规范实现（libcontainer）

### 3.1 选用 libcontainer（runc 内嵌的 OCI 实现）

- **不调用 runc/buildah 等任何外部命令**，在 Go 中直接使用 **`github.com/opencontainers/runc/libcontainer`**。
- libcontainer 是 runc 项目内的**原生 Go 实现**，提供 OCI Runtime Spec 的完整生命周期：Create、Load、State、Signal、Destroy 等，与规范一一对应。
- **优点**：无 CLI 依赖、无输出解析、类型安全、与 OCI 规范同源；list 通过读取 state 根目录 + `Load` 实现，kill 用 `Signal`，delete 用 `Destroy`。
- **依赖**：在 `go.mod` 中增加 `github.com/opencontainers/runc`（仅使用其 `libcontainer` 子包及 runc 的 `utils` 等必要部分）。

### 3.2 与 OCI 规范的对应关系

| OCI 规范操作 | libcontainer API |
|--------------|------------------|
| **state**    | `container.State()` / `container.OCIState()` |
| **kill**     | `container.Signal(os.Signal)` |
| **delete**   | `container.Destroy()` |
| **list**     | `os.ReadDir(root)` 得到容器 ID 列表，再对每个 ID `libcontainer.Load(root, id)` 加载并取状态 |

### 3.3 注意事项

- libcontainer 需在 **Linux** 上使用（依赖 namespaces、cgroups 等）；若需跨平台，可 build tag 限定或运行时判断。
- 容器 state 的 **root** 目录与 runc 默认一致：一般为 `/run/runc`（rootless 时为 `$XDG_RUNTIME_DIR/runc`），可通过 flag 配置。

---

## 四、详细实施步骤

### 4.1 行为与兼容

- 保持 **stop** 的现有语义：先（可选）清理以某前缀开头的容器，再停止 gin server。
- 将「buildah 容器」改为「**OCI 容器**」（由 runc 管理、id 以 `ar_` 开头）。
- 新增/保留 flag（见下），便于与现有脚本兼容。

### 4.2 命令行与配置

建议保留并复用现有 flag，仅将含义从「buildah」改为「OCI」：

| Flag                          | 建议含义                     | 默认值   |
|-------------------------------|------------------------------|----------|
| `--container-prefix`          | 要操作的 OCI 容器 ID 前缀    | `ar_`    |
| `--buildah-container-remove`   | 保留原名，语义改为「是否移除 OCI 容器」 | `false`  |
| （新增）`--oci-runtime-root`  | OCI 容器 state 根目录（libcontainer 的 root） | `/run/runc` |

若希望命名更一致，可新增 `--oci-container-remove` 并与 `--buildah-container-remove` 做别名或弃用说明。

### 4.3 代码改造清单（基于 libcontainer API）

1. **依赖**
   - 在 `go.mod` 中增加：`github.com/opencontainers/runc`（使用 `libcontainer` 及必要时 `libcontainer/utils`）。
   - 仅需 Linux 构建时可用；若项目需跨平台，用 `//go:build linux` 将 OCI 相关代码放在单独文件。

2. **列出容器（list）**
   - 使用 `os.ReadDir(ociRuntimeRoot)` 读取 state 根目录，得到所有子目录名即为容器 ID。
   - 过滤：只保留 `strings.HasPrefix(id, containerNamePrefix)` 的 ID。
   - 对每个 ID 调用 `libcontainer.Load(ociRuntimeRoot, id)` 得到 `*libcontainer.Container`；若 Load 失败（如已被删除）则跳过并 continue。

3. **状态（state）**
   - 对已 Load 的 container 调用 `container.Status()` 得到 `libcontainer.Status`（Running、Stopped、Created、Paused 等）。
   - 需要 OCI 标准 state 时可调用 `container.OCIState()` 得到 `*specs.State`。

4. **停止与删除（kill + delete）**
   - 对每个匹配的 container：
     - 若 `container.Status() == libcontainer.Running`（或 `Created`）：先 `container.Signal(syscall.SIGTERM)`，可选短暂等待后再 `container.Signal(syscall.SIGKILL)`。
     - 然后调用 `container.Destroy()`（对应 OCI 的 delete）。
   - 错误处理：单个容器 Signal/Destroy 失败记 Warn 日志，不中断其余容器；root 目录不存在或不可读时打 Debug/Warn 并 return nil。

5. **替换 stop 命令中的调用**
   - 将 `removeBuildahContainers(containerNamePrefix)` 替换为 `stopAndRemoveOCIContainers(ociRuntimeRoot, containerNamePrefix)`，内部完全使用 libcontainer API，无 exec。

6. **类型与命名**
   - 移除 `buildahContainerEntry` 及 buildah 相关类型。
   - 不再需要「解析 JSON 输出」的代码；状态来自 `container.State()` / `container.Status()`。
   - 函数命名：`removeBuildahContainers` → `stopAndRemoveOCIContainers`；注释标明「使用 OCI Runtime Specification 实现（libcontainer）」。

7. **root 不可用时的行为**
   - 若 `os.ReadDir(ociRuntimeRoot)` 返回 `os.ErrNotExist`：视为「尚无容器」，打 Debug 日志并 return nil。
   - 其他错误（权限等）：打 Warn 并 return nil，不把 stop 命令整体视为失败（与现 buildah 行为一致）。

### 4.4 依赖与构建

- **必须**：在 `go.mod` 中增加 `github.com/opencontainers/runc`（版本建议与当前 runc 稳定版对齐，如 v1.2.x / v1.4.x）。
- libcontainer 依赖 Linux 系统调用；若后端需在非 Linux 编译或运行，需用 build tag 将 OCI 相关代码隔离，非 Linux 时跳过容器清理或提供空实现。

### 4.5 测试建议

- **单元测试**：对「按前缀过滤 ID 列表」等纯逻辑做表驱动测试；可 mock `os.ReadDir` 或抽象「列出 ID」接口便于测试。
- **集成测试**：在 Linux 环境中，用 runc 或 libcontainer 创建若干以 `ar_` 开头的容器，执行 stop，断言这些容器被 Signal + Destroy，且 gin 进程按 PID 文件停止。
- **构建**：在非 Linux 环境（如 macOS）上若 OCI 代码受 build tag 限制，需确保 `go build ./...` 仍通过。

---

## 五、文件与修改点汇总

| 文件 | 修改内容 |
|------|----------|
| `backend/pkg/web/cmd.go` | 1) 增加 `--oci-runtime-root` flag；2) 引入 `libcontainer`，实现基于 `Load`+`Status`+`Signal`+`Destroy` 的 `stopAndRemoveOCIContainers`；3) stop 命令改为调用该函数；4) 移除 buildah 相关类型与 exec 调用。 |
| `backend/pkg/web/oci_linux.go`（可选） | 若需隔离 Linux 专用代码，可将 OCI 容器列表/停止/删除逻辑放在 `//go:build linux` 文件中，非 Linux 提供空实现或跳过。 |
| `design/停止流程.md` | 可选：将「调用 runc api」改为「按 OCI 规范通过 libcontainer 停止/删除容器」。 |
| `backend/go.mod` | 增加 `github.com/opencontainers/runc` 依赖。 |

---

## 六、实施顺序建议

1. 在 `go.mod` 中增加 `github.com/opencontainers/runc`，执行 `go mod tidy`。
2. 在 `cmd.go` 中实现 `stopAndRemoveOCIContainers(root, prefix string)`：`ReadDir` → 按前缀过滤 → 对每个 ID `libcontainer.Load` → `Signal(SIGTERM)`（必要时 SIGKILL）→ `Destroy()`，并处理 `ErrNotExist` 等。
3. 增加 `--oci-runtime-root`，stop 命令中根据 flag 调用 `stopAndRemoveOCIContainers`，保留 `--buildah-container-remove` 语义为「是否执行 OCI 容器清理」。
4. 移除 buildah 相关代码与类型，统一命名与注释。
5. 视需要将 OCI 逻辑抽到 `oci_linux.go` 并加 build tag，保证非 Linux 可编译。
6. 补充单元/集成测试，更新设计文档。

---

## 七、风险与注意事项

- **state root 一致性**：要清理的「ar_ 容器」必须是由同一 OCI runtime root 管理的（即与 runc 默认或你配置的 root 一致）。若创建容器时用的是其他 root（如某容器引擎自定义路径），需通过 `--oci-runtime-root` 指向该路径。
- **root 与 rootless**：rootless 时 state root 常为 `$XDG_RUNTIME_DIR/runc`，需在文档或 flag 说明中写明。
- **权限**：进程需对 state root 有读权限，对要 Signal/Destroy 的容器目录有写权限，且能向容器 init 进程发信号。
- **平台**：libcontainer 仅支持 Linux；若后端需在 Windows/macOS 运行，必须用 build tag 隔离，避免编译或运行时错误。

完成以上步骤后，`backend/pkg/web/cmd.go` 即改为**直接依赖 OpenContainer 规范实现（libcontainer）**操作容器，不调用任何 runc/buildah 等外部命令。
