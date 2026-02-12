//go:build !linux

package container

import "errors"

// 非 Linux 平台上，OCI 容器清理实现为空，以保证编译通过。
func StopAndRemoveOCIContainers(root, prefix string) error {
	return errors.New("not implemented on non-linux platform")
}
