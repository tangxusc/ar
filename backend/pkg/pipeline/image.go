package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
)

// ImageEntry 表示镜像仓库中的一条镜像记录。
type ImageEntry struct {
	Name string // 存储目录名（用于 rm/prune 指定）
	Ref  string // 镜像引用名（来自 annotation 或 Name）
	Path string // 完整路径
}

// ListImages 列出 storeDir 下所有 OCI layout 镜像目录。
func ListImages(storeDir string) ([]ImageEntry, error) {
	if storeDir == "" {
		return nil, fmt.Errorf("镜像存储目录不能为空")
	}
	entries, err := os.ReadDir(storeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取镜像存储目录失败: %w", err)
	}

	var list []ImageEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(storeDir, name)
		ref, err := readImageRefFromLayout(path)
		if err != nil {
			// 非 OCI layout 或损坏则跳过，不视为错误
			continue
		}
		if ref == "" {
			ref = name
		}
		list = append(list, ImageEntry{Name: name, Ref: ref, Path: path})
	}
	return list, nil
}

// OpenImageFromStore 根据镜像名或引用从 storeDir 中打开 v1.Image，供 run 使用。
func OpenImageFromStore(storeDir, imageNameOrRef string) (v1.Image, error) {
	list, err := ListImages(storeDir)
	if err != nil {
		return nil, err
	}
	safe := sanitizeImageName(imageNameOrRef)
	var layoutPath string
	for _, e := range list {
		if e.Name == imageNameOrRef || e.Ref == imageNameOrRef || (safe != "" && e.Name == safe) {
			layoutPath = e.Path
			break
		}
	}
	if layoutPath == "" {
		return nil, fmt.Errorf("镜像未找到: %s（请先 ar load 导入）", imageNameOrRef)
	}
	ociPath, err := layout.FromPath(layoutPath)
	if err != nil {
		return nil, fmt.Errorf("打开 OCI layout 失败 %s: %w", layoutPath, err)
	}
	index, err := ociPath.ImageIndex()
	if err != nil {
		return nil, err
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}
	for _, desc := range manifest.Manifests {
		img, err := index.Image(desc.Digest)
		if err == nil {
			return img, nil
		}
	}
	return nil, fmt.Errorf("镜像 index 中无可用镜像: %s", layoutPath)
}

// readImageRefFromLayout 从 OCI layout 目录读取 org.opencontainers.image.ref.name 注解。
func readImageRefFromLayout(layoutPath string) (string, error) {
	indexPath := filepath.Join(layoutPath, "index.json")
	if _, err := os.Stat(indexPath); err != nil {
		return "", err
	}
	ociPath, err := layout.FromPath(layoutPath)
	if err != nil {
		return "", err
	}
	index, err := ociPath.ImageIndex()
	if err != nil {
		return "", err
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return "", err
	}
	if len(manifest.Manifests) == 0 {
		return "", nil
	}
	if manifest.Manifests[0].Annotations != nil {
		if ref := manifest.Manifests[0].Annotations[ociRefNameAnnotation]; ref != "" {
			return ref, nil
		}
	}
	return "", nil
}

// DeleteImage 从 storeDir 中删除指定名称的镜像目录。
func DeleteImage(storeDir, name string) error {
	if storeDir == "" || name == "" {
		return fmt.Errorf("镜像存储目录和镜像名不能为空")
	}
	safe := sanitizeImageName(name)
	if safe == "" {
		safe = name
	}
	path := filepath.Join(storeDir, safe)
	// 允许按“存储名”或“原 ref”删除：若 name 已是安全名则用 safe；否则先尝试 name 再尝试 safe
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(storeDir, name)
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("镜像不存在: %s", name)
		}
		return fmt.Errorf("访问镜像目录失败: %w", err)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("删除镜像失败 %s: %w", path, err)
	}
	return nil
}

// ReferencedImageNames 从 pipelinesDir 下所有 *.template.json 中收集引用的镜像名（存储目录名形式）。
func ReferencedImageNames(pipelinesDir string) (map[string]struct{}, error) {
	refs := make(map[string]struct{})
	if pipelinesDir == "" {
		return refs, nil
	}
	entries, err := os.ReadDir(pipelinesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return refs, nil
		}
		return nil, fmt.Errorf("读取流水线目录失败: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".template.json") {
			continue
		}
		path := filepath.Join(pipelinesDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var steps []struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal(data, &steps); err != nil {
			continue
		}
		for _, s := range steps {
			img := strings.TrimSpace(s.Image)
			if img == "" {
				continue
			}
			safe := sanitizeImageName(img)
			if safe != "" {
				refs[safe] = struct{}{}
			}
			// 同时保留未 sanitize 的 key，便于按“目录名”匹配
			refs[img] = struct{}{}
		}
	}
	return refs, nil
}

// PruneImages 删除 storeDir 中未被流水线引用的镜像；返回被删除的镜像名列表。
func PruneImages(storeDir, pipelinesDir string) ([]string, error) {
	referenced, err := ReferencedImageNames(pipelinesDir)
	if err != nil {
		return nil, err
	}
	list, err := ListImages(storeDir)
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, entry := range list {
		if _, ok := referenced[entry.Name]; ok {
			continue
		}
		if _, ok := referenced[entry.Ref]; ok {
			continue
		}
		if err := os.RemoveAll(entry.Path); err != nil {
			return pruned, fmt.Errorf("删除未引用镜像 %s 失败: %w", entry.Name, err)
		}
		pruned = append(pruned, entry.Name)
	}
	return pruned, nil
}

// PruneAllImages 删除 storeDir 下所有镜像目录（用于 image prune --all）。
func PruneAllImages(storeDir string) ([]string, error) {
	list, err := ListImages(storeDir)
	if err != nil {
		return nil, err
	}
	var pruned []string
	for _, entry := range list {
		if err := os.RemoveAll(entry.Path); err != nil {
			return pruned, fmt.Errorf("删除镜像 %s 失败: %w", entry.Name, err)
		}
		pruned = append(pruned, entry.Name)
	}
	return pruned, nil
}
