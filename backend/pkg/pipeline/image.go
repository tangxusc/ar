package pipeline

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ImageEntry 表示镜像仓库中的一条镜像记录。
type ImageEntry struct {
	Name string // 存储目录名（用于 rm/prune 指定）
	Ref  string // 镜像引用名（来自 annotation 或 Name）
	Path string // 完整路径
}

// registryAuthEntry 表示单个镜像仓库的认证信息。
type registryAuthEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// authFile 是登录信息在磁盘上的整体结构。
type authFile struct {
	Registries map[string]registryAuthEntry `json:"registries"`
}

const authFilePath = "/var/lib/ar/auth.json"

// SaveRegistryAuth 保存指定 registry 的用户名和密码，供后续镜像拉取使用。
func SaveRegistryAuth(server, username, password string) error {
	server = strings.TrimSpace(server)
	if server == "" {
		return fmt.Errorf("registry 服务器地址不能为空")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("用户名不能为空")
	}

	data, err := loadAuthFile()
	if err != nil {
		return err
	}
	if data.Registries == nil {
		data.Registries = make(map[string]registryAuthEntry)
	}
	data.Registries[server] = registryAuthEntry{
		Username: username,
		Password: password,
	}

	if err := saveAuthFile(data); err != nil {
		return err
	}
	return nil
}

// getAuthForRegistry 读取指定 registry 的认证信息。
func getAuthForRegistry(server string) (registryAuthEntry, bool, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return registryAuthEntry{}, false, nil
	}
	data, err := loadAuthFile()
	if err != nil {
		return registryAuthEntry{}, false, err
	}
	if data.Registries == nil {
		return registryAuthEntry{}, false, nil
	}
	entry, ok := data.Registries[server]
	return entry, ok, nil
}

func loadAuthFile() (*authFile, error) {
	content, err := os.ReadFile(authFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &authFile{Registries: make(map[string]registryAuthEntry)}, nil
		}
		return nil, fmt.Errorf("读取登录配置失败: %w", err)
	}
	var data authFile
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("解析登录配置失败: %w", err)
	}
	if data.Registries == nil {
		data.Registries = make(map[string]registryAuthEntry)
	}
	return &data, nil
}

func saveAuthFile(data *authFile) error {
	if data == nil {
		return fmt.Errorf("登录配置不能为空")
	}
	dir := filepath.Dir(authFilePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建登录配置目录失败 %s: %w", dir, err)
	}
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化登录配置失败: %w", err)
	}
	if err := os.WriteFile(authFilePath, content, 0600); err != nil {
		return fmt.Errorf("写入登录配置失败 %s: %w", authFilePath, err)
	}
	return nil
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

// pullRemoteImage 从远程拉取镜像到内存，供 PullImage / PullImageToStore 复用。
func pullRemoteImage(imageRef string, tlsVerify bool) (v1.Image, error) {
	refStr := strings.TrimSpace(imageRef)
	if refStr == "" {
		return nil, fmt.Errorf("镜像引用不能为空")
	}
	nameOptions := []name.Option{}
	if !tlsVerify {
		nameOptions = append(nameOptions, name.Insecure)
	}
	ref, err := name.ParseReference(refStr, nameOptions...)
	if err != nil {
		return nil, fmt.Errorf("解析镜像引用失败: %w", err)
	}
	remoteOptions := []remote.Option{}
	if !tlsVerify {
		remoteOptions = append(remoteOptions, remote.WithTransport(&http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // 由 --tls-verify 控制
			},
		}))
	}
	if entry, ok, _ := getAuthForRegistry(ref.Context().RegistryStr()); ok {
		remoteOptions = append(remoteOptions, remote.WithAuth(&authn.Basic{
			Username: entry.Username,
			Password: entry.Password,
		}))
	}
	img, err := remote.Image(ref, remoteOptions...)
	if err != nil {
		return nil, fmt.Errorf("拉取镜像失败: %w", err)
	}
	return img, nil
}

// PullImage 从远程拉取镜像到内存，不写入存储。供构建流水线镜像时拉取 base 等使用。
func PullImage(imageRef string, tlsVerify bool) (v1.Image, error) {
	return pullRemoteImage(imageRef, tlsVerify)
}

// PullImageToStore 从远程镜像仓库拉取镜像，并以 OCI layout 形式写入本地镜像存储目录。
// imageRef 例如: registry.cn-shanghai.aliyuncs.com/tangxusc/alpine:3.18.0
// storeDir 使用全局 flags 中的 --images-store-dir。
func PullImageToStore(imageRef, storeDir string, tlsVerify bool) (string, error) {
	if strings.TrimSpace(storeDir) == "" {
		return "", fmt.Errorf("镜像存储目录不能为空")
	}
	img, err := pullRemoteImage(imageRef, tlsVerify)
	if err != nil {
		return "", err
	}
	return writeImageToStore(img, strings.TrimSpace(imageRef), storeDir)
}

// PushImageFromStore 将本地镜像存储目录中的镜像推送到远程镜像仓库。
// imageNameOrRef 可以是存储目录名（ListImages 第一列）或原始镜像引用名（ListImages 第二列）。
// targetRef 若为空，则优先使用本地镜像记录的原始引用名；否则推送为 targetRef。
func PushImageFromStore(imageNameOrRef, storeDir, targetRef string, tlsVerify bool) (string, error) {
	imageNameOrRef = strings.TrimSpace(imageNameOrRef)
	if imageNameOrRef == "" {
		return "", fmt.Errorf("镜像名称或引用不能为空")
	}
	if strings.TrimSpace(storeDir) == "" {
		return "", fmt.Errorf("镜像存储目录不能为空")
	}

	list, err := ListImages(storeDir)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", fmt.Errorf("当前本地镜像存储目录下无任何镜像（%s）", storeDir)
	}

	safe := sanitizeImageName(imageNameOrRef)
	var matched *ImageEntry
	for i := range list {
		e := &list[i]
		if e.Name == imageNameOrRef || e.Ref == imageNameOrRef || (safe != "" && e.Name == safe) {
			matched = e
			break
		}
	}
	if matched == nil {
		return "", fmt.Errorf("在本地镜像存储目录中未找到指定镜像: %s", imageNameOrRef)
	}

	pushRefStr := strings.TrimSpace(targetRef)
	if pushRefStr == "" {
		if strings.TrimSpace(matched.Ref) == "" {
			return "", fmt.Errorf("本地镜像未记录原始引用名，请通过 --target 指定要推送到的镜像名")
		}
		pushRefStr = matched.Ref
	}

	nameOptions := []name.Option{}
	if !tlsVerify {
		// 允许使用 http 以及跳过证书校验（与部分私有仓库兼容）。
		nameOptions = append(nameOptions, name.Insecure)
	}
	ref, err := name.ParseReference(pushRefStr, nameOptions...)
	if err != nil {
		return "", fmt.Errorf("解析目标镜像引用失败: %w", err)
	}

	remoteOptions := []remote.Option{}
	if !tlsVerify {
		remoteOptions = append(remoteOptions, remote.WithTransport(&http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // 由 --tls-verify 控制，允许跳过证书校验
			},
		}))
	}

	// 若存在针对该 registry 的登录信息，则使用 basic auth（与 PullImageToStore 行为保持一致）。
	if entry, ok, _ := getAuthForRegistry(ref.Context().RegistryStr()); ok {
		remoteOptions = append(remoteOptions, remote.WithAuth(&authn.Basic{
			Username: entry.Username,
			Password: entry.Password,
		}))
	}

	// 从本地 store 打开镜像。
	img, err := OpenImageFromStore(storeDir, matched.Name)
	if err != nil {
		return "", fmt.Errorf("打开本地镜像失败: %w", err)
	}

	if err := remote.Write(ref, img, remoteOptions...); err != nil {
		return "", fmt.Errorf("推送镜像失败: %w", err)
	}
	return ref.Name(), nil
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
