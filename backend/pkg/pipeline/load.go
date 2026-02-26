package pipeline

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runc/libcontainer/specconv"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const ociRefNameAnnotation = "org.opencontainers.image.ref.name"

type Loader struct {
	pipelinesDir   string
	imagesStoreDir string
	tmpRoot        string
	runtimeRoot    string
}

func NewLoader(pipelinesDir, imagesStoreDir, tmpRoot, runtimeRoot string) *Loader {
	return &Loader{
		pipelinesDir:   pipelinesDir,
		imagesStoreDir: imagesStoreDir,
		tmpRoot:        tmpRoot,
		runtimeRoot:    runtimeRoot,
	}
}

func (l *Loader) Load(ctx context.Context, archivePath string, cleanTmp bool) error {
	archiveAbs, err := filepath.Abs(archivePath)
	if err != nil {
		return fmt.Errorf("解析镜像路径失败: %w", err)
	}

	pipelineImage, imageRef, err := loadImageFromArchive(archiveAbs)
	if err != nil {
		return fmt.Errorf("加载流水线镜像失败: %w", err)
	}

	imageName := sanitizeImageName(imageRef)
	if imageName == "" {
		imageName = sanitizeImageName(stripArchiveExt(filepath.Base(archiveAbs)))
	}
	if imageName == "" {
		imageName = "pipeline"
	}

	workRoot := filepath.Join(l.tmpRoot, imageName)
	bundleDir := filepath.Join(workRoot, "bundle")
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	runtimeImagesDir := filepath.Join(workRoot, "images")

	if err := os.RemoveAll(workRoot); err != nil {
		return fmt.Errorf("清理旧的临时目录失败 %s: %w", workRoot, err)
	}
	if err := ensureDirs(l.pipelinesDir, l.imagesStoreDir, rootfsDir, runtimeImagesDir); err != nil {
		return err
	}

	logrus.Infof("开始解包流水线镜像 rootfs: %s", rootfsDir)
	if err := extractRootfsFromImage(pipelineImage, rootfsDir); err != nil {
		return fmt.Errorf("解包流水线镜像失败: %w", err)
	}
	if err := ensureEntrypointInRootfs(pipelineImage, rootfsDir); err != nil {
		return err
	}

	logrus.Infof("开始生成 OCI runtime spec: %s/config.json", bundleDir)
	if err := writeRuntimeSpec(bundleDir, pipelineImage, l.pipelinesDir, runtimeImagesDir); err != nil {
		return fmt.Errorf("生成 OCI 运行时配置失败: %w", err)
	}

	containerID := fmt.Sprintf("ar_load_%d", time.Now().UnixNano())
	logrus.Infof("开始运行一次性流水线容器: %s", containerID)
	// Loader 的一次性容器仍然将 stdout/stderr 聚合到内存，用于错误信息。
	var out bytes.Buffer
	if err := runOneShotContainer(ctx, l.runtimeRoot, bundleDir, containerID, &out, &out); err != nil {
		// 将容器输出附加到错误日志，便于排查加载失败原因。
		return fmt.Errorf("%w, output: %s", err, strings.TrimSpace(out.String()))
	}

	logrus.Infof("流水线容器执行完成，开始加载子镜像目录: %s", runtimeImagesDir)
	loadedCount, err := l.loadAllImagesFromDir(runtimeImagesDir)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(runtimeImagesDir); err != nil {
		return fmt.Errorf("子镜像加载完成后清理目录失败 %s: %w", runtimeImagesDir, err)
	}

	logrus.Infof("流水线加载完成: image=%s childImages=%d", imageName, loadedCount)
	if cleanTmp {
		if err := os.RemoveAll(workRoot); err != nil {
			return fmt.Errorf("清理临时目录失败 %s: %w", workRoot, err)
		}
		logrus.Infof("已清理临时目录: %s", workRoot)
	}
	return nil
}

// LoadFromStore 从 images-store-dir 中已存在的流水线镜像加载：解包 rootfs、运行一次性容器、
// 将模板与子镜像写入 pipelinesDir 并导入子镜像到 images-store-dir。
func (l *Loader) LoadFromStore(ctx context.Context, imageNameOrRef string, cleanTmp bool) error {
	pipelineImage, err := OpenImageFromStore(l.imagesStoreDir, imageNameOrRef)
	if err != nil {
		return fmt.Errorf("从本地镜像存储打开流水线镜像失败: %w", err)
	}

	imageName := sanitizeImageName(imageNameOrRef)
	if imageName == "" {
		imageName = "pipeline"
	}

	workRoot := filepath.Join(l.tmpRoot, imageName)
	bundleDir := filepath.Join(workRoot, "bundle")
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	runtimeImagesDir := filepath.Join(workRoot, "images")

	if err := os.RemoveAll(workRoot); err != nil {
		return fmt.Errorf("清理旧的临时目录失败 %s: %w", workRoot, err)
	}
	if err := ensureDirs(l.pipelinesDir, l.imagesStoreDir, rootfsDir, runtimeImagesDir); err != nil {
		return err
	}

	logrus.Infof("开始解包流水线镜像 rootfs: %s (from store)", rootfsDir)
	if err := extractRootfsFromImage(pipelineImage, rootfsDir); err != nil {
		return fmt.Errorf("解包流水线镜像失败: %w", err)
	}
	if err := ensureEntrypointInRootfs(pipelineImage, rootfsDir); err != nil {
		return err
	}

	logrus.Infof("开始生成 OCI runtime spec: %s/config.json", bundleDir)
	if err := writeRuntimeSpec(bundleDir, pipelineImage, l.pipelinesDir, runtimeImagesDir); err != nil {
		return fmt.Errorf("生成 OCI 运行时配置失败: %w", err)
	}

	containerID := fmt.Sprintf("ar_load_%d", time.Now().UnixNano())
	logrus.Infof("开始运行一次性流水线容器: %s", containerID)
	var out bytes.Buffer
	if err := runOneShotContainer(ctx, l.runtimeRoot, bundleDir, containerID, &out, &out); err != nil {
		return fmt.Errorf("%w, output: %s", err, strings.TrimSpace(out.String()))
	}

	logrus.Infof("流水线容器执行完成，开始加载子镜像目录: %s", runtimeImagesDir)
	loadedCount, err := l.loadAllImagesFromDir(runtimeImagesDir)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(runtimeImagesDir); err != nil {
		return fmt.Errorf("子镜像加载完成后清理目录失败 %s: %w", runtimeImagesDir, err)
	}

	logrus.Infof("流水线加载完成(来自本地存储): image=%s childImages=%d", imageName, loadedCount)
	if cleanTmp {
		if err := os.RemoveAll(workRoot); err != nil {
			return fmt.Errorf("清理临时目录失败 %s: %w", workRoot, err)
		}
		logrus.Infof("已清理临时目录: %s", workRoot)
	}
	return nil
}

func (l *Loader) loadAllImagesFromDir(imagesDir string) (int, error) {
	archives, err := collectArchiveFiles(imagesDir)
	if err != nil {
		return 0, fmt.Errorf("遍历子镜像目录失败: %w", err)
	}
	if len(archives) == 0 {
		logrus.Infof("子镜像目录为空，无需加载: %s", imagesDir)
		return 0, nil
	}

	loaded := 0
	for _, archive := range archives {
		img, imageRef, err := loadImageFromArchive(archive)
		if err != nil {
			return loaded, fmt.Errorf("加载子镜像失败 %s: %w", archive, err)
		}

		if strings.TrimSpace(imageRef) == "" {
			imageRef = stripArchiveExt(filepath.Base(archive))
		}
		dest, err := writeImageToStore(img, imageRef, l.imagesStoreDir)
		if err != nil {
			return loaded, fmt.Errorf("导入子镜像失败 %s: %w", archive, err)
		}
		logrus.Infof("子镜像导入成功: %s -> %s", archive, dest)
		loaded++
	}

	return loaded, nil
}

func runOneShotContainer(ctx context.Context, runtimeRoot, bundleDir, containerID string, stdout, stderr io.Writer) error {
	if strings.TrimSpace(runtimeRoot) == "" {
		return fmt.Errorf("OCI runtime state root 不能为空")
	}

	spec, err := readRuntimeSpec(bundleDir)
	if err != nil {
		return err
	}
	// 仅在非 root 时启用 rootless（UserNamespace）
	if os.Geteuid() != 0 {
		if err := ensureRootlessRuntimeSpec(spec); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(runtimeRoot, 0700); err != nil {
		return fmt.Errorf("创建 OCI runtime state root 失败 %s: %w", runtimeRoot, err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前工作目录失败: %w", err)
	}
	if err := os.Chdir(bundleDir); err != nil {
		return fmt.Errorf("切换到 bundle 目录失败 %s: %w", bundleDir, err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	rootless := os.Geteuid() != 0
	containerCfg, err := specconv.CreateLibcontainerConfig(&specconv.CreateOpts{
		CgroupName:      containerID,
		Spec:            spec,
		RootlessEUID:    rootless,
		RootlessCgroups: rootless,
	})
	if err != nil {
		return fmt.Errorf("根据 OCI spec 构建容器配置失败: %w", err)
	}

	container, err := libcontainer.Create(runtimeRoot, containerID, containerCfg)
	if err != nil {
		return fmt.Errorf("创建 OCI 容器失败: %w", err)
	}
	defer func() {
		_ = container.Destroy()
	}()

	var out bytes.Buffer
	stdoutWriter := io.Writer(&out)
	stderrWriter := io.Writer(&out)
	if stdout != nil {
		stdoutWriter = io.MultiWriter(stdout, &out)
	}
	if stderr != nil {
		stderrWriter = io.MultiWriter(stderr, &out)
	}

	process, err := toLibcontainerProcess(spec.Process, stdoutWriter, stderrWriter)
	if err != nil {
		return err
	}
	process.Init = true

	if err := container.Run(process); err != nil {
		return fmt.Errorf("一次性容器运行失败: %w, 输出: %s", err, strings.TrimSpace(out.String()))
	}

	type waitResult struct {
		state *os.ProcessState
		err   error
	}
	waitCh := make(chan waitResult, 1)
	go func() {
		state, waitErr := process.Wait()
		waitCh <- waitResult{state: state, err: waitErr}
	}()

	select {
	case r := <-waitCh:
		if r.err != nil {
			return fmt.Errorf("等待一次性容器退出失败: %w, 输出: %s", r.err, strings.TrimSpace(out.String()))
		}
		if code := r.state.ExitCode(); code != 0 {
			return fmt.Errorf("一次性容器退出码非 0: %d, 输出: %s", code, strings.TrimSpace(out.String()))
		}
		return nil
	case <-ctx.Done():
		_ = container.Signal(syscall.SIGKILL)
		<-waitCh
		return fmt.Errorf("一次性容器运行被取消: %w, 输出: %s", ctx.Err(), strings.TrimSpace(out.String()))
	}
}

func readRuntimeSpec(bundleDir string) (*specs.Spec, error) {
	specPath := filepath.Join(bundleDir, "config.json")
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("读取 OCI spec 失败 %s: %w", specPath, err)
	}

	var spec specs.Spec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return nil, fmt.Errorf("解析 OCI spec 失败 %s: %w", specPath, err)
	}
	if spec.Process == nil {
		return nil, fmt.Errorf("OCI spec 缺少 process 配置")
	}
	if len(spec.Process.Args) == 0 {
		return nil, fmt.Errorf("OCI spec 缺少 process.args，无法运行一次性容器")
	}
	if strings.TrimSpace(spec.Process.Cwd) == "" {
		spec.Process.Cwd = "/"
	}
	return &spec, nil
}

func ensureRootlessRuntimeSpec(spec *specs.Spec) error {
	if spec == nil {
		return fmt.Errorf("OCI spec 不能为空")
	}
	if spec.Process == nil {
		return fmt.Errorf("OCI spec 缺少 process 配置")
	}
	if spec.Linux == nil {
		spec.Linux = &specs.Linux{}
	}

	if !hasLinuxNamespace(spec.Linux.Namespaces, specs.UserNamespace) {
		spec.Linux.Namespaces = append(spec.Linux.Namespaces, specs.LinuxNamespace{Type: specs.UserNamespace})
	}

	processUID := spec.Process.User.UID
	processGID := spec.Process.User.GID
	spec.Linux.UIDMappings = []specs.LinuxIDMapping{
		{
			ContainerID: processUID,
			HostID:      uint32(os.Geteuid()),
			Size:        1,
		},
	}
	spec.Linux.GIDMappings = []specs.LinuxIDMapping{
		{
			ContainerID: processGID,
			HostID:      uint32(os.Getegid()),
			Size:        1,
		},
	}
	return nil
}

func hasLinuxNamespace(namespaces []specs.LinuxNamespace, namespaceType specs.LinuxNamespaceType) bool {
	for _, ns := range namespaces {
		if ns.Type == namespaceType {
			return true
		}
	}
	return false
}

func toLibcontainerProcess(processSpec *specs.Process, stdout, stderr io.Writer) (*libcontainer.Process, error) {
	if processSpec.Terminal {
		return nil, fmt.Errorf("一次性容器暂不支持 terminal=true")
	}

	p := &libcontainer.Process{
		Args:            append([]string{}, processSpec.Args...),
		Env:             append([]string{}, processSpec.Env...),
		UID:             int(processSpec.User.UID),
		GID:             int(processSpec.User.GID),
		Cwd:             processSpec.Cwd,
		Stdout:          stdout,
		Stderr:          stderr,
		Label:           processSpec.SelinuxLabel,
		AppArmorProfile: processSpec.ApparmorProfile,
	}
	noNewPrivileges := processSpec.NoNewPrivileges
	p.NoNewPrivileges = &noNewPrivileges

	if len(processSpec.User.AdditionalGids) > 0 {
		p.AdditionalGroups = make([]int, 0, len(processSpec.User.AdditionalGids))
		for _, gid := range processSpec.User.AdditionalGids {
			p.AdditionalGroups = append(p.AdditionalGroups, int(gid))
		}
	}

	if caps := processSpec.Capabilities; caps != nil {
		p.Capabilities = &configs.Capabilities{
			Bounding:    append([]string{}, caps.Bounding...),
			Effective:   append([]string{}, caps.Effective...),
			Inheritable: append([]string{}, caps.Inheritable...),
			Permitted:   append([]string{}, caps.Permitted...),
			Ambient:     append([]string{}, caps.Ambient...),
		}
	}

	return p, nil
}

func writeRuntimeSpec(bundleDir string, image v1.Image, pipelinesDir, runtimeImagesDir string) error {
	if err := os.MkdirAll(bundleDir, 0755); err != nil {
		return fmt.Errorf("创建 bundle 目录失败: %w", err)
	}

	cfg, err := image.ConfigFile()
	if err != nil {
		return fmt.Errorf("读取镜像配置失败: %w", err)
	}

	args := append([]string{}, cfg.Config.Entrypoint...)
	args = append(args, cfg.Config.Cmd...)
	if len(args) == 0 {
		return fmt.Errorf("镜像缺少 entrypoint/cmd，无法运行一次性容器")
	}
	// 使用 /bin/sh 显式执行脚本类 entrypoint，避免内核解析 shebang 时因解释器路径或 CRLF 等问题报 "no such file or directory"
	if len(args) == 1 && (args[0] == "/entrypoint.sh" || strings.HasSuffix(args[0], ".sh")) {
		args = append([]string{"/bin/sh"}, args...)
		// 确保 rootfs 中存在 /bin/sh（来自 base 层），否则 runc 会报 "stat /bin/sh: no such file or directory"
		shPath := filepath.Join(bundleDir, "rootfs", "bin", "sh")
		if _, err := os.Stat(shPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("rootfs 中缺少 /bin/sh，无法执行 entrypoint 脚本；请确认流水线镜像是通过 ar pipeline build 正确构建的（含 base 层）且解包了全部镜像层")
			}
			return fmt.Errorf("检查 rootfs/bin/sh 失败: %w", err)
		}
	}

	env := append([]string{}, cfg.Config.Env...)
	if !containsEnvKey(env, "PATH") {
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}

	cwd := cfg.Config.WorkingDir
	if strings.TrimSpace(cwd) == "" {
		cwd = "/"
	}

	spec := specs.Spec{
		Version: specs.Version,
		Process: &specs.Process{
			Terminal:        false,
			Args:            args,
			Env:             env,
			Cwd:             cwd,
			NoNewPrivileges: true,
		},
		Root: &specs.Root{
			Path:     "rootfs",
			Readonly: false,
		},
		Hostname: "ar-load",
		Mounts: []specs.Mount{
			{
				Destination: "/proc",
				Type:        "proc",
				Source:      "proc",
			},
			{
				Destination: "/dev",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/dev/pts",
				Type:        "devpts",
				Source:      "devpts",
				Options:     []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
			},
			{
				Destination: "/dev/shm",
				Type:        "tmpfs",
				Source:      "shm",
				Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
			},
			{
				Destination: "/dev/mqueue",
				Type:        "mqueue",
				Source:      "mqueue",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			},
			{
				Destination: "/pipelines",
				Type:        "bind",
				Source:      pipelinesDir,
				Options:     []string{"rbind", "rw"},
			},
			{
				Destination: "/images",
				Type:        "bind",
				Source:      runtimeImagesDir,
				Options:     []string{"rbind", "rw"},
			},
		},
		Linux: &specs.Linux{
			Namespaces: []specs.LinuxNamespace{
				{Type: specs.PIDNamespace},
				{Type: specs.IPCNamespace},
				{Type: specs.UTSNamespace},
				{Type: specs.MountNamespace},
				{Type: specs.NetworkNamespace},
			},
			MaskedPaths: []string{
				"/proc/acpi",
				"/proc/asound",
				"/proc/kcore",
				"/proc/keys",
				"/proc/latency_stats",
				"/proc/timer_list",
				"/proc/timer_stats",
				"/proc/sched_debug",
				"/sys/firmware",
				"/proc/scsi",
			},
			ReadonlyPaths: []string{
				"/proc/bus",
				"/proc/fs",
				"/proc/irq",
				"/proc/sys",
				"/proc/sysrq-trigger",
			},
		},
	}

	specBytes, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 OCI spec 失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "config.json"), specBytes, 0644); err != nil {
		return fmt.Errorf("写入 config.json 失败: %w", err)
	}
	return nil
}

// ensureEntrypointInRootfs 检查镜像配置的 entrypoint 在 rootfs 中是否存在，避免运行时出现 "no such file or directory"。
func ensureEntrypointInRootfs(image v1.Image, rootfsDir string) error {
	cfg, err := image.ConfigFile()
	if err != nil {
		return fmt.Errorf("读取镜像配置失败: %w", err)
	}
	args := append([]string{}, cfg.Config.Entrypoint...)
	args = append(args, cfg.Config.Cmd...)
	if len(args) == 0 {
		return nil
	}
	entryPath := strings.TrimPrefix(args[0], "/")
	if entryPath == "" {
		return nil
	}
	absPath := filepath.Join(rootfsDir, entryPath)
	st, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("镜像不是有效的流水线镜像：rootfs 中缺少 entrypoint %s（请使用 ar pipeline build 构建流水线镜像后再加载，或检查本地镜像是否为流水线镜像）", args[0])
		}
		return fmt.Errorf("检查 entrypoint 失败 %s: %w", absPath, err)
	}
	if st.Mode().IsDir() {
		return fmt.Errorf("镜像 entrypoint %s 是目录而非可执行文件", args[0])
	}
	return nil
}

func extractRootfsFromImage(image v1.Image, rootfsDir string) error {
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return fmt.Errorf("创建 rootfs 目录失败: %w", err)
	}

	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("获取镜像层列表失败: %w", err)
	}
	// 按层顺序解压（base 先，再逐层叠加），确保 rootfs 包含完整文件系统（含 /bin/sh 等 base 层内容）。
	// 不依赖 mutate.Extract，避免从 OCI layout 打开的 image 在合并层时出现只解出顶层的问题。
	for i, layer := range layers {
		stream, err := layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("读取第 %d 层失败: %w", i+1, err)
		}
		if err := extractTarStream(stream, rootfsDir); err != nil {
			_ = stream.Close()
			return fmt.Errorf("解压第 %d 层失败: %w", i+1, err)
		}
		if err := stream.Close(); err != nil {
			return fmt.Errorf("关闭第 %d 层流失败: %w", i+1, err)
		}
	}
	return nil
}

func writeImageToStore(img v1.Image, imageRef, storeRoot string) (string, error) {
	name := sanitizeImageName(imageRef)
	if name == "" {
		name = "image"
	}
	dest := filepath.Join(storeRoot, name)

	if err := os.RemoveAll(dest); err != nil {
		return "", fmt.Errorf("清理镜像目录失败: %w", err)
	}

	ociPath, err := layout.Write(dest, empty.Index)
	if err != nil {
		return "", fmt.Errorf("创建 OCI layout 失败: %w", err)
	}

	options := make([]layout.Option, 0, 1)
	if strings.TrimSpace(imageRef) != "" {
		options = append(options, layout.WithAnnotations(map[string]string{
			ociRefNameAnnotation: imageRef,
		}))
	}

	if err := ociPath.AppendImage(img, options...); err != nil {
		return "", fmt.Errorf("写入 OCI layout 失败: %w", err)
	}

	return dest, nil
}

func loadImageFromArchive(archivePath string) (v1.Image, string, error) {
	tarPath, cleanup, err := normalizeArchiveToTar(archivePath)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()

	if img, ref, err := loadDockerArchiveImage(tarPath); err == nil {
		return img, ref, nil
	}

	if img, ref, err := loadOCIArchiveImage(tarPath); err == nil {
		return img, ref, nil
	}

	return nil, "", fmt.Errorf("不支持的镜像归档格式: %s", archivePath)
}

func loadDockerArchiveImage(tarPath string) (v1.Image, string, error) {
	var imageRef string
	manifest, err := tarball.LoadManifest(fileOpener(tarPath))
	if err == nil && len(manifest) > 0 && len(manifest[0].RepoTags) > 0 {
		imageRef = manifest[0].RepoTags[0]
	}

	if strings.TrimSpace(imageRef) != "" {
		tag, err := name.NewTag(imageRef, name.WeakValidation)
		if err == nil {
			img, err := tarball.ImageFromPath(tarPath, &tag)
			if err == nil {
				return img, imageRef, nil
			}
		}
	}

	img, err := tarball.ImageFromPath(tarPath, nil)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(imageRef) == "" {
		imageRef = stripArchiveExt(filepath.Base(tarPath))
	}
	return img, imageRef, nil
}

func loadOCIArchiveImage(tarPath string) (v1.Image, string, error) {
	extractDir, err := os.MkdirTemp("", "ar-oci-archive-*")
	if err != nil {
		return nil, "", fmt.Errorf("创建 OCI 归档临时目录失败: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := untarFileToDir(tarPath, extractDir); err != nil {
		return nil, "", err
	}

	layoutRoot, err := detectOCILayoutRoot(extractDir)
	if err != nil {
		return nil, "", err
	}

	ociPath, err := layout.FromPath(layoutRoot)
	if err != nil {
		return nil, "", err
	}

	index, err := ociPath.ImageIndex()
	if err != nil {
		return nil, "", err
	}

	return firstImageFromIndex(index)
}

func firstImageFromIndex(index v1.ImageIndex) (v1.Image, string, error) {
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, "", err
	}
	if len(manifest.Manifests) == 0 {
		return nil, "", fmt.Errorf("OCI index 为空")
	}

	for _, desc := range manifest.Manifests {
		imageRef := ""
		if desc.Annotations != nil {
			imageRef = desc.Annotations[ociRefNameAnnotation]
		}

		img, err := index.Image(desc.Digest)
		if err == nil {
			return img, imageRef, nil
		}

		childIndex, indexErr := index.ImageIndex(desc.Digest)
		if indexErr != nil {
			continue
		}
		img, nestedRef, nestedErr := firstImageFromIndex(childIndex)
		if nestedErr != nil {
			continue
		}
		if nestedRef == "" {
			nestedRef = imageRef
		}
		return img, nestedRef, nil
	}

	return nil, "", fmt.Errorf("OCI index 中没有可用镜像")
}

func normalizeArchiveToTar(archivePath string) (string, func(), error) {
	cleanup := func() {}
	lower := strings.ToLower(archivePath)

	if strings.HasSuffix(lower, ".tar") {
		return archivePath, cleanup, nil
	}
	if !strings.HasSuffix(lower, ".tar.gz") && !strings.HasSuffix(lower, ".tgz") {
		return archivePath, cleanup, nil
	}

	in, err := os.Open(archivePath)
	if err != nil {
		return "", cleanup, fmt.Errorf("打开归档失败: %w", err)
	}
	defer in.Close()

	header := make([]byte, 2)
	n, err := io.ReadFull(in, header)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return "", cleanup, fmt.Errorf("归档文件过小，无法解析: %s", archivePath)
		}
		return "", cleanup, fmt.Errorf("读取归档头失败: %w", err)
	}
	if !isGzipHeader(header[:n]) {
		// 兼容后缀为 .tar.gz/.tgz 但内容实际是 tar 的归档。
		return archivePath, cleanup, nil
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return "", cleanup, fmt.Errorf("重置归档读取位置失败: %w", err)
	}

	gzReader, err := gzip.NewReader(in)
	if err != nil {
		return "", cleanup, fmt.Errorf("解压归档失败: %w", err)
	}
	defer gzReader.Close()

	tmpFile, err := os.CreateTemp("", "ar-load-*.tar")
	if err != nil {
		return "", cleanup, fmt.Errorf("创建临时 tar 失败: %w", err)
	}

	if _, err := io.Copy(tmpFile, gzReader); err != nil {
		tmpName := tmpFile.Name()
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return "", cleanup, fmt.Errorf("写入临时 tar 失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		tmpName := tmpFile.Name()
		_ = os.Remove(tmpName)
		return "", cleanup, fmt.Errorf("关闭临时 tar 失败: %w", err)
	}

	tmpName := tmpFile.Name()
	cleanup = func() {
		_ = os.Remove(tmpName)
	}
	return tmpName, cleanup, nil
}

func untarFileToDir(tarPath, destDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("打开 tar 文件失败: %w", err)
	}
	defer file.Close()

	if err := extractTarStream(file, destDir); err != nil {
		return fmt.Errorf("解压 tar 文件失败: %w", err)
	}
	return nil
}

func extractTarStream(reader io.Reader, destDir string) error {
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		targetPath, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, fs.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, tr); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			if err := os.RemoveAll(targetPath); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
				return err
			}
		case tar.TypeLink:
			linkTarget, err := safeJoin(destDir, filepath.Join(filepath.Dir(hdr.Name), hdr.Linkname))
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			if err := os.RemoveAll(targetPath); err != nil {
				return err
			}
			if err := os.Link(linkTarget, targetPath); err != nil {
				return err
			}
		case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			// 设备节点和 fifo 在当前场景并非必需，跳过以避免非特权环境报错。
			continue
		default:
			continue
		}

		modTime := hdr.ModTime
		if !modTime.IsZero() {
			_ = os.Chtimes(targetPath, modTime, modTime)
		}
	}
}

func safeJoin(root, tarEntry string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(tarEntry, "/"))
	if clean == "." {
		return root, nil
	}
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("非法 tar 路径: %s", tarEntry)
	}
	return target, nil
}

func detectOCILayoutRoot(extractDir string) (string, error) {
	rootIndex := filepath.Join(extractDir, "index.json")
	if _, err := os.Stat(rootIndex); err == nil {
		return extractDir, nil
	}

	var layoutRoot string
	err := filepath.WalkDir(extractDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "index.json" {
			layoutRoot = filepath.Dir(path)
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return "", err
	}
	if strings.TrimSpace(layoutRoot) == "" {
		return "", fmt.Errorf("归档中未找到 OCI layout(index.json)")
	}
	return layoutRoot, nil
}

func collectArchiveFiles(dir string) ([]string, error) {
	archives := make([]string, 0)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isArchiveFile(d.Name()) {
			archives = append(archives, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(archives)
	return archives, nil
}

func isArchiveFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tar") ||
		strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz")
}

func stripArchiveExt(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return name[:len(name)-len(".tar.gz")]
	case strings.HasSuffix(lower, ".tgz"):
		return name[:len(name)-len(".tgz")]
	case strings.HasSuffix(lower, ".tar"):
		return name[:len(name)-len(".tar")]
	default:
		return name
	}
}

func sanitizeImageName(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	if at := strings.Index(value, "@"); at > 0 {
		value = value[:at]
	}
	if colon := strings.LastIndex(value, ":"); colon > strings.LastIndex(value, "/") {
		value = value[:colon]
	}

	replacer := strings.NewReplacer(
		"/", "_",
		":", "_",
		"@", "_",
		" ", "_",
	)
	value = replacer.Replace(value)
	value = strings.Trim(value, "._-")
	if value == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	return strings.Trim(builder.String(), "._-")
}

func containsEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func isGzipHeader(header []byte) bool {
	return len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b
}

func fileOpener(path string) tarball.Opener {
	return func() (io.ReadCloser, error) {
		return os.Open(path)
	}
}

func ensureDirs(paths ...string) error {
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("目录路径不能为空")
		}
		if err := os.MkdirAll(p, 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", p, err)
		}
	}
	return nil
}
