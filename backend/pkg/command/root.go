package command

import (
	"context"
	"os"
	"os/signal"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tangxusc/ar/backend/pkg/config"
)

func NewRootCommand() (context.Context, context.CancelFunc, *cobra.Command) {
	ctx, cancelFunc := context.WithCancel(context.TODO())
	rootCommand := &cobra.Command{
		Use:   `ar`,
		Short: `AR 命令行工具`,
		Long:  `AR 命令行工具，用于启动后端服务等子命令`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if config.Debug {
				logrus.SetLevel(logrus.DebugLevel)
				logrus.SetReportCaller(true)
				logrus.Debug("debug 模式已开启，日志级别: Debug")
			} else {
				logrus.SetLevel(logrus.InfoLevel)
				logrus.SetReportCaller(false)
			}
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Kill)
			go func() {
				<-c
				cancelFunc()
			}()
		},
	}
	return ctx, cancelFunc, rootCommand
}
