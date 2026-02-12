package web

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tangxusc/ar/backend/pkg/command"
	"github.com/tangxusc/ar/backend/pkg/config"
)

var webServerPort string = "8080"
var logFilePath string = "/var/lib/ar/server.log"
var pidFilePath string = "/var/lib/ar/server.pid"

// 是否在 stop 时尝试按前缀清理 OCI 容器（基于 libcontainer）。
var buildahContainerRemove bool = false

// 要清理的容器 ID 前缀，默认与原设计保持一致：ar_。
var containerNamePrefix string = "ar_"

// OCI runtime 的 state 根目录（libcontainer 使用的 root，与 runc --root 含义一致）。
// 默认使用 /run/runc，rootless 场景可通过 flag 指定为 $XDG_RUNTIME_DIR/runc 等路径。
var ociRuntimeRoot string = "/run/runc"

func initLog() (io.Writer, error) {
	if dir := filepath.Dir(logFilePath); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return file, err
	}
	writer := io.MultiWriter(file, os.Stdout)
	logrus.SetOutput(writer)
	if config.Debug {
		logrus.SetReportCaller(true)
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}
	return writer, nil
}

func AddCommand(ctx context.Context, cancelFunc func(), rootCommand *cobra.Command) {
	serverCmd := &cobra.Command{
		Use:   `server`,
		Short: `web server 相关命令`,
	}

	startCmd := &cobra.Command{
		Use:   `start`,
		Short: `启动web server,默认监听8080端口`,
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() {
				cancelFunc()
			}()
			_, err := initLog()
			if err != nil {
				return err
			}
			if err := writePIDFile(pidFilePath); err != nil {
				return err
			}
			if err := Start(ctx); err != nil {
				return err
			}
			<-ctx.Done()
			return nil
		},
	}
	startCmd.PersistentFlags().StringVar(&webServerPort, "web-server-port", "8080", "graphql web server port")
	startCmd.PersistentFlags().StringVar(&logFilePath, "log-file-path", "/var/lib/ar/server.log", "log file path")
	startCmd.PersistentFlags().StringVar(&pidFilePath, "pid-file-path", "/var/lib/ar/server.pid", "pid file path for stop command")
	serverCmd.AddCommand(startCmd)

	stopCmd := &cobra.Command{
		Use:   `stop`,
		Short: `停止后端：按前缀清理 OCI 容器，并停止本地 gin server`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if buildahContainerRemove {
				if err := stopAndRemoveOCIContainers(ociRuntimeRoot, containerNamePrefix); err != nil {
					logrus.Warnf("按前缀清理 OCI 容器时出错（可忽略）: %v", err)
				}
			}
			return stopBackendProcess(pidFilePath)
		},
	}
	stopCmd.PersistentFlags().StringVar(&containerNamePrefix, "container-prefix", "ar_", "要移除的 OCI 容器 ID 前缀")
	stopCmd.PersistentFlags().StringVar(&pidFilePath, "pid-file-path", "/var/lib/ar/server.pid", "pid 文件路径，用于停止本地 server 进程")
	stopCmd.PersistentFlags().BoolVar(&buildahContainerRemove, "buildah-container-remove", false, "是否按前缀移除 OCI 容器,默认不移除（保留原参数名以兼容）")
	stopCmd.PersistentFlags().StringVar(&ociRuntimeRoot, "oci-runtime-root", "/run/runc", "OCI 容器 state 根目录（libcontainer root），例如 /run/runc 或 rootless 的 $XDG_RUNTIME_DIR/runc")
	serverCmd.AddCommand(stopCmd)

	command.RegisterCommand(func(ctx context.Context, cancelFunc func(), command *cobra.Command) {
		command.AddCommand(serverCmd)
	})
}

func writePIDFile(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
}

func readPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func stopBackendProcess(pidPath string) error {
	pid, err := readPIDFile(pidPath)
	if err != nil {
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		logrus.Warnf("向进程 %d 发送 SIGTERM 失败: %v", pid, err)
		return nil
	}
	logrus.Infof("已向后端进程 %d 发送停止信号", pid)
	_ = os.Remove(pidPath)
	return nil
}
