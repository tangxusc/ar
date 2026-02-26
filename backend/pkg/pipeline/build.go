package pipeline

import (
	"archive/tar"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/sirupsen/logrus"
)

// parseFromInDockerfile 从 Dockerfile 中解析第一条 FROM 指令的镜像引用，行为参照 docker build。
// 支持 FROM image、FROM image:tag、FROM image AS stage 等形式；忽略空行与 # 注释。
func parseFromInDockerfile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开 Dockerfile 失败: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 忽略空行与注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 处理行续行：Dockerfile 中 \ 结尾表示下一行接续，此处仅解析首行 FROM，不拼接续行
		line = strings.TrimSuffix(line, "\\")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			continue
		}
		rest := strings.TrimSpace(line[5:]) // 去掉 "FROM "
		if rest == "" {
			return "", fmt.Errorf("Dockerfile 中 FROM 指令缺少镜像名: %s", path)
		}
		// 第一个 token 为镜像引用（含 :tag），后续可能为 AS stage 或 --platform=...
		parts := strings.Fields(rest)
		image := strings.TrimSpace(parts[0])
		if image == "" {
			return "", fmt.Errorf("Dockerfile 中 FROM 指令缺少镜像名: %s", path)
		}
		return image, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取 Dockerfile 失败: %w", err)
	}
	return "", fmt.Errorf("Dockerfile 中未找到 FROM 指令: %s", path)
}

// BuildPipelineImage 根据设计文档构建流水线镜像（不依赖 Docker，使用 go-containerregistry）：
// 1. 校验 templatePath、imageListPath 存在
// 2. 在 tmpRoot 下创建临时目录 <流水线镜像名>_<时间戳>，结束后按 cleanBuildDir 决定是否删除
// 3. 拉取镜像列表中的镜像到临时目录 images-store
// 4. 使用 base 镜像（FROM 指令：优先从 dockerfilePath 解析，否则用 fromImage），追加层并写入 imagesStoreDir
func BuildPipelineImage(templatePath, imageListPath, pipelineImageTag, fromImage, dockerfilePath, tmpRoot, imagesStoreDir string, tlsVerify bool, cleanBuildDir bool) error {
	templatePath = strings.TrimSpace(templatePath)
	imageListPath = strings.TrimSpace(imageListPath)
	pipelineImageTag = strings.TrimSpace(pipelineImageTag)
	dockerfilePath = strings.TrimSpace(dockerfilePath)
	if templatePath == "" {
		return fmt.Errorf("流水线模板路径不能为空")
	}
	if imageListPath == "" {
		return fmt.Errorf("镜像列表文件路径不能为空")
	}
	if pipelineImageTag == "" {
		return fmt.Errorf("流水线镜像名不能为空")
	}
	if tmpRoot == "" {
		return fmt.Errorf("临时目录根路径不能为空")
	}
	if imagesStoreDir == "" {
		return fmt.Errorf("镜像存储目录不能为空")
	}
	// FROM 指令：未指定 -f 时使用 --from；指定 -f 时从该路径解析首条 FROM（路径可为构建目录内的 Dockerfile）
	fromImage = strings.TrimSpace(fromImage)
	if fromImage == "" {
		fromImage = "alpine:latest"
	}

	// 1. 校验模板文件存在
	templateAbs, err := filepath.Abs(templatePath)
	if err != nil {
		return fmt.Errorf("解析模板路径失败: %w", err)
	}
	if _, err := os.Stat(templateAbs); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("流水线模板文件不存在: %s", templateAbs)
		}
		return fmt.Errorf("校验模板文件失败: %w", err)
	}

	// 2. 校验镜像列表文件存在
	imageListAbs, err := filepath.Abs(imageListPath)
	if err != nil {
		return fmt.Errorf("解析镜像列表路径失败: %w", err)
	}
	if _, err := os.Stat(imageListAbs); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("镜像列表文件不存在: %s", imageListAbs)
		}
		return fmt.Errorf("校验镜像列表文件失败: %w", err)
	}

	templateBase := filepath.Base(templateAbs)
	if !strings.HasSuffix(templateBase, ".template.json") {
		return fmt.Errorf("模板文件名须以 .template.json 结尾: %s", templateBase)
	}
	pipelineName := strings.TrimSuffix(templateBase, ".template.json")
	if pipelineName == "" {
		return fmt.Errorf("无效的模板文件名: %s", templateBase)
	}

	// 3. 创建临时目录 <流水线镜像名>_<时间戳>
	sanitizedTag := sanitizeImageName(pipelineImageTag)
	if sanitizedTag == "" {
		sanitizedTag = "pipeline"
	}
	ts := time.Now().Format("20060102150405")
	tmpDir := filepath.Join(tmpRoot, sanitizedTag+"_"+ts)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败 %s: %w", tmpDir, err)
	}
	defer func() {
		if !cleanBuildDir {
			logrus.Infof("保留构建目录: %s", tmpDir)
			return
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			logrus.Warnf("清理构建目录失败 %s: %v", tmpDir, err)
		}
	}()

	imagesStoreInBuild := filepath.Join(tmpDir, "images-store")
	if err := os.MkdirAll(imagesStoreInBuild, 0755); err != nil {
		return fmt.Errorf("创建 images-store 目录失败: %w", err)
	}

	// 4. 读取镜像列表并拉取到临时 images-store
	f, err := os.Open(imageListAbs)
	if err != nil {
		return fmt.Errorf("打开镜像列表文件失败: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var imageLines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		imageLines = append(imageLines, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取镜像列表失败: %w", err)
	}

	for _, imageRef := range imageLines {
		logrus.Infof("拉取镜像: %s", imageRef)
		if _, err := PullImageToStore(imageRef, imagesStoreInBuild, tlsVerify); err != nil {
			return fmt.Errorf("拉取镜像 %s 失败: %w", imageRef, err)
		}
	}

	// 5. 将模板文件复制到临时目录（设计文档步骤 5）
	dstTemplate := filepath.Join(tmpDir, templateBase)
	if err := copyFile(templateAbs, dstTemplate); err != nil {
		return fmt.Errorf("复制模板文件到构建目录失败: %w", err)
	}

	// 6. 写入 Dockerfile 到构建目录（设计文档步骤 6：<流水线镜像名>_<时间戳>/Dockerfile）
	dockerfilePathInBuild := filepath.Join(tmpDir, "Dockerfile")
	dockerfileContent := fmt.Sprintf(`FROM %s

COPY entrypoint.sh /entrypoint.sh
COPY %s /%s
COPY ./images-store/* /images-store/

ENTRYPOINT ["/entrypoint.sh"]
`, fromImage, templateBase, templateBase)
	if err := os.WriteFile(dockerfilePathInBuild, []byte(dockerfileContent), 0644); err != nil {
		return fmt.Errorf("写入 Dockerfile 失败: %w", err)
	}
	logrus.Infof("Dockerfile 已写入: %s", dockerfilePathInBuild)

	// 按设计文档步骤 8：构建时读取 Dockerfile 解析 FROM，行为参照 docker build。
	// -f 未指定：使用刚写入的 tmpDir/Dockerfile；-f 指定：绝对路径直接用，相对路径优先按当前工作目录解析（外部 Dockerfile），否则按构建目录解析。
	dfPath := dockerfilePathInBuild
	if dockerfilePath != "" {
		if filepath.IsAbs(dockerfilePath) {
			dfPath = filepath.Clean(dockerfilePath)
		} else {
			cleaned := filepath.Clean(dockerfilePath)
			if cwd, err := os.Getwd(); err == nil {
				externalPath := filepath.Join(cwd, cleaned)
				if _, statErr := os.Stat(externalPath); statErr == nil {
					dfPath = externalPath
					logrus.Infof("使用外部 Dockerfile 解析 FROM: %s", dfPath)
				} else {
					dfPath = filepath.Join(tmpDir, cleaned)
				}
			} else {
				dfPath = filepath.Join(tmpDir, cleaned)
			}
		}
	}
	if _, err := os.Stat(dfPath); err == nil {
		parsed, err := parseFromInDockerfile(dfPath)
		if err != nil {
			return err
		}
		fromImage = parsed
		logrus.Infof("从 Dockerfile 解析 FROM: %s", fromImage)
	}

	// 7. 写入 entrypoint.sh（设计文档步骤 7）
	entrypointBody := `#!/bin/sh
cp /*.json /pipelines/
cp -r ./images-store/* /images-store/ 2>/dev/null || true
`
	if err := os.WriteFile(filepath.Join(tmpDir, "entrypoint.sh"), []byte(entrypointBody), 0755); err != nil {
		return fmt.Errorf("写入 entrypoint.sh 失败: %w", err)
	}

	templateData, err := os.ReadFile(templateAbs)
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}

	// 8. 获取 base 镜像（实现 FROM 指令）：先在 --images-store-dir 中查找，无则从远程拉取
	baseImg, err := OpenImageFromStore(imagesStoreDir, fromImage)
	if err != nil {
		logrus.Infof("本地未找到 base 镜像 %s，从远程拉取", fromImage)
		baseImg, err = PullImage(fromImage, tlsVerify)
		if err != nil {
			return fmt.Errorf("拉取 base 镜像 %s 失败: %w", fromImage, err)
		}
	} else {
		logrus.Infof("使用本地 base 镜像: %s", fromImage)
	}

	// 9. 构建包含 entrypoint.sh、模板、images-store 的 tar 层（OCI 层路径无前导 /）
	layer, err := buildPipelineLayer(entrypointBody, templateBase, templateData, imagesStoreInBuild)
	if err != nil {
		return fmt.Errorf("构建镜像层失败: %w", err)
	}

	// 10. 追加层并设置 Entrypoint
	img, err := mutate.AppendLayers(baseImg, layer)
	if err != nil {
		return fmt.Errorf("追加层失败: %w", err)
	}

	cfg, err := img.ConfigFile()
	if err != nil {
		return fmt.Errorf("读取镜像配置失败: %w", err)
	}
	cfg.Config.Entrypoint = []string{"/entrypoint.sh"}
	cfg.Config.Cmd = nil
	img, err = mutate.Config(img, cfg.Config)
	if err != nil {
		return fmt.Errorf("设置 Entrypoint 失败: %w", err)
	}

	// 11. 写入 imagesStoreDir（OCI layout）
	dest, err := writeImageToStore(img, pipelineImageTag, imagesStoreDir)
	if err != nil {
		return fmt.Errorf("写入镜像存储失败: %w", err)
	}

	logrus.Infof("流水线镜像已构建并保存: %s", dest)
	return nil
}

// buildPipelineLayer 生成包含 entrypoint.sh、模板文件、images-store 目录的 OCI 层（tar 流）。
// 路径为根相对且无前导 /，符合 OCI 层规范。
func buildPipelineLayer(entrypointBody, templateBase string, templateData []byte, imagesStoreDir string) (v1.Layer, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		tw := tar.NewWriter(pw)
		defer tw.Close()
		now := time.Now()

		// entrypoint.sh
		if err := writeTarFile(tw, "entrypoint.sh", []byte(entrypointBody), 0755, now); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		// 模板 json（放在根目录，如 /alpine.template.json）
		if err := writeTarFile(tw, templateBase, templateData, 0644, now); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		// images-store 目录下所有内容，路径前缀为 images-store/
		if err := writeTarDir(tw, imagesStoreDir, "images-store", now); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
	}()

	layer, err := tarball.LayerFromReader(pr)
	if err != nil {
		return nil, err
	}
	return layer, nil
}

// copyFile 将 src 文件复制到 dst，保留权限（可执行位等）。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	mode := int(0644)
	if info, err := os.Stat(src); err == nil {
		mode = int(info.Mode() & 0777)
	}
	return os.WriteFile(dst, data, os.FileMode(mode))
}

func writeTarFile(tw *tar.Writer, name string, data []byte, mode int64, modTime time.Time) error {
	h := &tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(data)),
		ModTime:  modTime,
		Typeflag:  tar.TypeReg,
		Uid:      0,
		Gid:      0,
		Uname:    "root",
		Gname:    "root",
	}
	if err := tw.WriteHeader(h); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func writeTarDir(tw *tar.Writer, srcDir, tarPrefix string, modTime time.Time) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		var name string
		if rel == "." {
			name = tarPrefix + "/"
		} else {
			name = filepath.Join(tarPrefix, filepath.ToSlash(rel))
		}
		if info.IsDir() {
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			h := &tar.Header{
				Name:     name,
				Mode:     int64(info.Mode()),
				ModTime:  modTime,
				Typeflag: tar.TypeDir,
				Uid:      0,
				Gid:      0,
				Uname:    "root",
				Gname:    "root",
			}
			return tw.WriteHeader(h)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil {
			return err
		}
		h := &tar.Header{
			Name:     name,
			Mode:     int64(stat.Mode()),
			Size:     stat.Size(),
			ModTime:  modTime,
			Typeflag: tar.TypeReg,
			Uid:      0,
			Gid:      0,
			Uname:    "root",
			Gname:    "root",
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	})
}
