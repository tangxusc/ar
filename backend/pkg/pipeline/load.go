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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
)

const ociRefNameAnnotation = "org.opencontainers.image.ref.name"

type Loader struct {
	pipelinesDir   string
	imagesStoreDir string
	tmpRoot        string
	runtimeBinary  string
}

func NewLoader(pipelinesDir, imagesStoreDir, tmpRoot, runtimeBinary string) *Loader {
	return &Loader{
		pipelinesDir:   pipelinesDir,
		imagesStoreDir: imagesStoreDir,
		tmpRoot:        tmpRoot,
		runtimeBinary:  runtimeBinary,
	}
}

func (l *Loader) Load(ctx context.Context, archivePath string) error {
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

	logrus.Infof("开始生成 OCI runtime spec: %s/config.json", bundleDir)
	if err := writeRuntimeSpec(bundleDir, pipelineImage, l.pipelinesDir, runtimeImagesDir); err != nil {
		return fmt.Errorf("生成 OCI 运行时配置失败: %w", err)
	}

	containerID := fmt.Sprintf("ar_load_%d", time.Now().UnixNano())
	logrus.Infof("开始运行一次性流水线容器: %s", containerID)
	if err := runOneShotContainer(ctx, l.runtimeBinary, bundleDir, containerID); err != nil {
		return err
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

func runOneShotContainer(ctx context.Context, runtimeBinary, bundleDir, containerID string) error {
	if _, err := exec.LookPath(runtimeBinary); err != nil {
		return fmt.Errorf("未找到 OCI runtime 二进制 %q: %w", runtimeBinary, err)
	}

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, runtimeBinary, "run", "--bundle", bundleDir, containerID)
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("一次性容器运行失败: %w, 输出: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
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

func extractRootfsFromImage(image v1.Image, rootfsDir string) error {
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return fmt.Errorf("创建 rootfs 目录失败: %w", err)
	}

	stream := mutate.Extract(image)
	defer stream.Close()

	if err := extractTarStream(stream, rootfsDir); err != nil {
		return fmt.Errorf("解压 rootfs 失败: %w", err)
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
