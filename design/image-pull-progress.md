## allrun image pull 并行拉层与进度展示设计

### 背景与目标

- **命令入口**: `allrun image pull IMAGE`，当前实现位于 `backend/pkg/pipeline/cmd.go` → `addImageCommand` → `PullImageToStore`（`backend/pkg/pipeline/image.go`）。
- **现状**:
  - 拉取镜像：调用 `remote.Image` 一次性从远程仓库取回 `v1.Image`。
  - 写入本地：调用 `writeImageToStore(img, refStr, storeDir)`，使用 `layout.Write` + `layout.AppendImage` 将镜像写为 OCI layout。
  - **没有进度展示**，用户只能等命令完成后看到一条“镜像已拉取到本地”的日志。
- **目标**:
  - 拉取阶段**并行下载镜像层（layer）**。
  - 在终端输出上**仿照 `docker pull` 风格展示每一层的进度**（包含“Pulling fs layer / Downloading / Download complete”等）。
  - **不改变镜像本地存储结构**（仍使用 OCI layout，保持与现有 `load` / `run` 等逻辑兼容）。

> 本文档仅为设计说明，阶段性目标是**先确定接口和输出形式**，后续再按此设计逐步实现。当前阶段不修改任何 Go 代码。

---

### 总体设计

- **入口保持不变**: 仍通过 `PullImageToStore(imageRef, storeDir, tlsVerify)` 完成整个拉取流程，`cmd.go` 无需改动函数签名和调用方式。
- **内部逻辑拆分为两大阶段**:
  - **阶段 A：带进度的远程拉取**
    - 使用 `remote.Get` 获取 manifest + config 元数据。
    - 解析 manifest，按 layer 维度构造任务列表。
    - 使用 goroutine + worker pool **并行拉取每个 layer**，在读取 layer 内容时统计字节数，周期性刷新终端进度输出。
  - **阶段 B：写入本地 OCI layout**
    - 阶段 A 完成后，再按现有流程构造 `v1.Image` 并调用 `writeImageToStore`，保证与现有 `load` / `run` 等逻辑兼容。
- **实现策略分两步走**:
  - **第一阶段（简单版本）**：只在“拉取阶段”增加进度显示，但为了降低改造复杂度，本次实现仍会额外再调用一次 `remote.Image` 来生成 `v1.Image` 写入本地（即：**网络上会拉两次镜像**，换取实现简单、风险小）。
  - **后续优化阶段（完整版本）**：复用阶段 A 下载到本地的 layer 与 config，自己写入 OCI layout，不再调用 `remote.Image`，从而真正做到“一次下载 + 并行 + 带进度”。

---

### 与 docker pull 的行为对齐点

#### 输出形式

仿照 `docker pull`，终端输出大致分为三类信息：

- **整体提示**:
  - `Using default tag: latest`（可选，暂不强制）
  - `<tag>: Pulling from <repository>`
- **每层进度**（每一行对应一个 layer，使用 digest 短 ID 标识）:
  - `<short-digest>: Pulling fs layer`
  - `<short-digest>: Downloading x.xMB/yy.yMB`
  - `<short-digest>: Download complete`
  - `<short-digest>: Already exists`（未来如有本地缓存逻辑时可使用）
- **结束摘要**:
  - `Digest: sha256:xxxx...`
  - `Status: Downloaded newer image for <imageRef>`

本次设计中，**重点是每层 Downloading/Download complete 的展示**；整体提示和结束摘要可先实现最简版：

- 开始时打印：
  - `<imageRef>: Pulling`
- 结束时打印：
  - `Digest: <manifest digest>`
  - `Status: Downloaded image for <imageRef>`

#### 进度刷新策略

- 为了避免过度刷新导致日志刷屏，采用**定时刷新**：
  - 启动一个单独 goroutine，每隔固定时间（例如 200ms–500ms）汇总所有 layer 的当前进度并刷新输出。
  - 终止条件：所有 layer 标记为 `Done`，或任意 layer 出错。
- 不做复杂的 TTY 光标控制（不依赖 ANSI 控制码），先用**多行追加打印**的方式实现简单版本：
  - 每次刷新打印一组最新行，行格式中包含当前字节数 / 总字节数、人类可读单位（MB），示例：
    - `a3ed95ca: Downloading  5.3MB / 10.2MB`
    - `a3ed95ca: Download complete (10.2MB)`

---

### 阶段 A：带进度的远程拉取设计

#### 元数据获取与任务构造

1. **解析镜像引用**
   - 继续使用现有逻辑：
     - `ref, err := name.ParseReference(refStr, nameOptions...)`
   - `nameOptions` 中根据 `--tls-verify` 决定是否使用 `name.Insecure`。

2. **remote 选项**
   - 继续沿用当前 `remoteOptions` 逻辑：
     - 如 `!tlsVerify` 时使用自定义 `http.Transport` 跳过证书校验。
     - 若存在 registry 登录信息，添加 `remote.WithAuth(&authn.Basic{...})`。

3. **获取 manifest**
   - 新增使用 `remote.Get(ref, remoteOptions...)`（不直接调用 `remote.Image`）：
     - 返回值包含：
       - `Descriptor` 信息（包含 `Digest`、`MediaType` 等）。
       - `Manifest` 字节（JSON）。
   - 将 `desc.Manifest` 反序列化为一个简化结构：

     ```go
     type remoteManifest struct {
         MediaType string `json:"mediaType"`
         Config struct {
             MediaType string `json:"mediaType"`
             Digest    string `json:"digest"`
             Size      int64  `json:"size"`
         } `json:"config"`
         Layers []struct {
             MediaType string `json:"mediaType"`
             Digest    string `json:"digest"`
             Size      int64  `json:"size"`
         } `json:"layers"`
     }
     ```

4. **进度结构体**

   为每个 layer 准备一份进度信息：

   ```go
   type layerProgress struct {
       Digest    string // 全量 digest，例如 "sha256:..."
       Total     int64  // manifest 中的 size 字段
       Completed int64  // 已下载字节数
       Done      bool   // 是否完成（包含成功与失败）
       Err       error  // 拉取该层时的错误（若有）
   }
   ```

   所有 layer 进度可用一个 `[]*layerProgress` 保存：

   ```go
   progs := make([]*layerProgress, len(m.Layers))
   ```

#### worker pool 并发下载

- 使用一个简易 worker pool 控制并发数，例如：

  ```go
  const defaultPullJobs = 4 // 或根据 CPU / 网络调整
  ```

- 构造 `jobs` channel，channel 元素为 layer 下标 `int`：
  - 主 goroutine 将所有 layer 下标依次写入 `jobs`，写完后关闭。
  - 每个 worker 从 `jobs` 中读一个下标，对应地下载该 layer。

- worker 下载流程：

  1. 通过 layer 的 digest 构造引用：

     ```go
     d, err := name.NewDigest(layer.Digest, nameOptions...)
     ```

  2. 拉取远程 layer 对象：

     ```go
     l, err := remote.Layer(d, remoteOptions...)
     ```

  3. 获取压缩层数据流：

     ```go
     rc, err := l.Compressed()
     defer rc.Close()
     ```

  4. 使用 `io.TeeReader` 包装读取流，统计实际读取的字节数：

     ```go
     tr := io.TeeReader(rc, countingWriter{onWrite: func(n int) {
         atomic.AddInt64(&prog.Completed, int64(n))
     }})
     ```

     其中 `countingWriter` 仅用来在写入时回调：

     ```go
     type countingWriter struct {
         onWrite func(int)
     }

     func (w countingWriter) Write(p []byte) (int, error) {
         if w.onWrite != nil {
             w.onWrite(len(p))
         }
         return len(p), nil
     }
     ```

  5. 为了触发实际下载，需要将 `tr` 完整读完：

     ```go
     _, err = io.Copy(io.Discard, tr) // 简化版本先丢弃内容
     ```

     > **注意**：在“简单版本”中，这一步只用于产生进度和验证拉取可用，真正用于写 layout 的数据仍由后续 `remote.Image` 获取。优化版本会在这里把数据写入本地 blob 文件。

  6. 成功时设置：

     ```go
     prog.Done = true
     ```

     失败时：

     ```go
     prog.Err = err
     prog.Done = true
     ```

#### 进度刷新与输出

1. **短 digest 展示**

   为了让行更短，可将 digest 转为 12 字符的短 ID：

   ```go
   func shortDigest(d string) string {
       if i := strings.Index(d, ":"); i >= 0 {
           d = d[i+1:]
       }
       if len(d) > 12 {
           return d[:12]
       }
       return d
   }
   ```

2. **刷新 goroutine**

   - 使用 `time.Ticker` 周期性从 `progs` 中读取当前 `Completed` / `Total`，计算百分比和已下载大小（MB/KB）。
   - 对于还未开始（`Total == 0` 且 `Completed == 0`）的层，可打印：
     - `<short-digest>: Waiting`
   - 对于进行中的层：
     - `<short-digest>: Downloading  x.xMB / y.yMB (p%)`
   - 对于完成的层：
     - 成功：`<short-digest>: Download complete (y.yMB)`
     - 失败：`<short-digest>: Download failed: <err>`

3. **结束条件**

   - 当 `progs` 中所有层都 `Done == true` 时：
     - 停止 ticker，退出刷新 goroutine。
   - 若某层 `Err != nil`：
     - 可以选择立即在主流程中返回错误，并让刷新 goroutine 在下一次 Tick 检测到后结束。

4. **与主流程的同步**

   - 主 goroutine：
     - 等待 `worker` 的 `WaitGroup` 结束。
     - 扫描 `progs` 是否存在 `Err`，若有则整体返回错误。
   - 刷新 goroutine：
     - 独立通过 `progs` 的状态判断是否退出，不直接对主流程做控制。

---

### 阶段 B：写入本地 OCI layout 的两阶段方案

#### 简单版本（先行实现）

在简单版本中，**阶段 A 的并行拉取仅用于进度展示与连接校验**，实际写入仍复用当前逻辑：

1. 阶段 A 完成且无错误后，再执行现有代码：

   ```go
   img, err := remote.Image(ref, remoteOptions...)
   if err != nil {
       return "", fmt.Errorf("拉取镜像失败: %w", err)
   }
   dest, err := writeImageToStore(img, refStr, storeDir)
   ```

2. 这样会产生两次网络拉取：
   - 第一次：`remote.Layer` 按层读取（用于进度）。
   - 第二次：`remote.Image` 按镜像整体读取（用于生成 `v1.Image` 并写 layout）。

3. **优点**:
   - 不需要改动 `writeImageToStore` 或 `load.go` 中的其它逻辑。
   - 失败路径仍然简单、清晰，出错时直接返回错误，不产生部分写入的 layout。

4. **缺点**:
   - 同一镜像会从网络上下载两次，增加带宽占用与等待时间。
   - 但在对用户体验影响不大的前提下，可以作为**第一阶段可交付版本**。

#### 完整版本（后续优化）

完整版本的目标是：**阶段 A 下载的 layer 与 config 直接用于生成 OCI layout，不再额外调用 `remote.Image`**。

大致步骤：

1. 将阶段 A 中下载 layer 的逻辑从 `io.Copy(io.Discard, tr)` 改为写入本地 `blobs` 目录：
   - 布局仿照 OCI layout 规范：
     - `blobs/sha256/<digest>`。
   - 下载完成后，校验 digest 是否与 manifest 中声明一致。

2. 同样方式下载并存储 config blob。

3. 生成符合 OCI layout 规范的 `index.json` 与 `oci-layout` 文件：
   - 可参考当前 `writeImageToStore` 调用 `layout.Write` + `layout.AppendImage` 的行为，抄其输出结构。
   - 或者在内存中构造 `v1.ImageIndex` / `v1.Image`，然后调用 `layout.Write` / `layout.AppendImage` 将其写入指定目录。

4. 写入完成后，可复用已有的 `OpenImageFromStore` / `firstImageFromIndex` 等逻辑验证结果。

> 由于完整版本工作量较大，且影响到 `load.go` 中的复用函数 `writeImageToStore`，建议在简单版本稳定后再单独设计一个更细的文档或章节，专门说明本地 layout 重构方案。

---

### 错误处理与用户体验

- **网络错误 / 身份认证失败**:
  - 阶段 A 中任一 layer 出错时：
    - 在对应层进度行中标出错误信息。
    - 主流程返回错误（例如 `拉取镜像层失败: <details>`）。
  - 保证不会在本地留下不完整的 OCI layout 目录（简单版本中 layout 写入只在阶段 A 全部成功后才执行）。

- **TLS 校验控制**:
  - 继续保留 `--tls-verify` 参数：
    - 默认为 `true`。
    - 当为 `false` 时，通过 `remote.WithTransport` 使用 `InsecureSkipVerify: true` 的 TLS 配置。

- **多次输出的控制**:
  - 考虑日志量，刷新频率控制在 200ms–500ms。
  - 对超过一定长度的镜像名 / digest，只输出短 ID 或截断前缀，避免行过长。

---

### 对现有接口和调用方的影响

- **命令行接口**:
  - `allrun image pull IMAGE` 的参数和 flag 保持不变，包括：
    - `--tls-verify`。
  - 仅增加/调整终端输出内容。

- **内部函数签名**:
  - `PullImageToStore(imageRef, storeDir string, tlsVerify bool) (string, error)` 签名保持不变。
  - `writeImageToStore` 签名保持不变（简单版本完全不修改）。

- **与其他功能的关系**:
  - `pipeline load` / `pipeline run` 均依赖于镜像被正确写入 `config.ImagesStoreDir` 的 OCI layout 目录。
  - 由于简单版本仍然复用当前 `writeImageToStore` 逻辑，因此对这些功能的行为无影响。

---

### 实施计划（仅设计阶段）

1. **设计确认阶段（当前）**
   - 将本文档写入 `design/image-pull-progress.md`，作为 `allrun image pull` 并行与进度改造的设计依据。
   - 不对任何 Go 源码做修改。

2. **第一期开发**
   - 在 `PullImageToStore` 中接入：
     - `remote.Get` + manifest 解析。
     - worker pool 并行 `remote.Layer` 下载。
     - 周期性进度输出。
   - 下载完成后仍调用 `remote.Image` + `writeImageToStore` 写入。

3. **第二期优化（可选）**
   - 将阶段 A 下载到本地的数据直接写入 OCI layout。
   - 去掉第二次网络拉取，优化性能。

