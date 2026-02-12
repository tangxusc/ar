//go:build linux

package container

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/sirupsen/logrus"
)

// StopAndRemoveOCIContainers 使用 libcontainer 按 OCI Runtime 规范停止并删除指定前缀的容器。
// 实现思路参考 runc list：读取 root 目录下的子目录作为容器 ID，Load 后按前缀过滤，再根据状态 Signal + Destroy。
func StopAndRemoveOCIContainers(root, prefix string) error {
	// 允许关闭：若前缀为空，直接返回，避免误操作所有容器。
	if strings.TrimSpace(prefix) == "" {
		logrus.Warn("未指定容器前缀，跳过 OCI 容器清理")
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logrus.Warnf("OCI runtime root %s 不存在，跳过容器清理", root)
			return nil
		}
		logrus.Warnf("读取 OCI runtime root %s 失败: %v", root, err)
		return nil
	}

	var matched []*libcontainer.Container
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		c, err := libcontainer.Load(root, id)
		if err != nil {
			// 可能存在并发删除，记录日志后忽略
			logrus.Warnf("加载 OCI 容器 %s 失败: %v", id, err)
			continue
		}
		matched = append(matched, c)
	}

	if len(matched) == 0 {
		logrus.Infof("没有需要移除的 %s 前缀 OCI 容器（root=%s）", prefix, root)
		return nil
	}

	logrus.Infof("准备停止并删除 %d 个前缀为 %q 的 OCI 容器（root=%s）", len(matched), prefix, root)

	for _, c := range matched {
		id := c.ID()

		status, err := c.Status()
		if err != nil {
			logrus.Warnf("获取容器 %s 状态失败: %v", id, err)
			// 继续尝试 Destroy，以便清理残留 state
		}

		// 对 Running/Created 状态的容器先发送 SIGTERM，再必要时 SIGKILL。
		if status == libcontainer.Running || status == libcontainer.Created {
			if err := c.Signal(syscall.SIGTERM); err != nil {
				logrus.Warnf("向容器 %s 发送 SIGTERM 失败: %v", id, err)
			} else {
				logrus.Infof("已向容器 %s 发送 SIGTERM", id)
				// 简单等待一小段时间，让容器有机会优雅退出。
				time.Sleep(2 * time.Second)
			}

			statusAfter, err := c.Status()
			if err != nil {
				logrus.Debugf("再次获取容器 %s 状态失败（可能已退出）: %v", id, err)
			} else if statusAfter == libcontainer.Running {
				if err := c.Signal(syscall.SIGKILL); err != nil {
					logrus.Warnf("向容器 %s 发送 SIGKILL 失败: %v", id, err)
				} else {
					logrus.Infof("已向容器 %s 发送 SIGKILL", id)
				}
			}
		}

		// Destroy 对于已停止的容器是幂等的；即使容器仍在运行也会返回错误，我们只记录日志不终止整体流程。
		if err := c.Destroy(); err != nil {
			// 若 state 目录已不存在，视为已被其他进程删除。
			if IsNotExistErr(err) {
				logrus.Debugf("容器 %s 的 state 已不存在，视为已删除: %v", id, err)
				continue
			}
			logrus.Warnf("删除 OCI 容器 %s 失败: %v", id, err)
		} else {
			logrus.Infof("已删除 OCI 容器 %s", id)
		}
	}

	return nil
}

// IsNotExistErr 尝试判断 Destroy 过程中是否是「state 目录不存在」类错误。
func IsNotExistErr(err error) bool {
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	// 一些错误信息可能只包含路径字符串，做一个简单的降级检查。
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, os.ErrNotExist)
	}
	// 再粗略判断下常见路径关键字。
	msg := err.Error()
	return strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "not exist") ||
		strings.Contains(msg, string(filepath.Separator)+"state")
}
