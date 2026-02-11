package web

import (
	"context"

	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tangxusc/ar/backend/pkg/command"
	"github.com/tangxusc/ar/backend/pkg/config"
)

var webServerPort string = "8080"
var logFilePath string = "/var/lib/ar/server.log"

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
			if err := Start(ctx); err != nil {
				return err
			}
			<-ctx.Done()
			return nil
		},
	}
	startCmd.PersistentFlags().StringVar(&webServerPort, "web-server-port", "8080", "graphql web server port")
	startCmd.PersistentFlags().StringVar(&logFilePath, "log-file-path", "/var/lib/ar/server.log", "log file path")
	serverCmd.AddCommand(startCmd)

	command.RegisterCommand(func(ctx context.Context, cancelFunc func(), command *cobra.Command) {
		command.AddCommand(serverCmd)
	})
}
