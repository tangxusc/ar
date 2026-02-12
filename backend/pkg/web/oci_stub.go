//go:build !linux

package web

// 非 Linux 平台上，OCI 容器清理实现为空，以保证编译通过。
func stopAndRemoveOCIContainers(root, prefix string) error {
	return nil
}

