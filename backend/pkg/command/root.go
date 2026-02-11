package command

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
)

func NewRootCommand() (context.Context, context.CancelFunc, *cobra.Command) {
	ctx, cancelFunc := context.WithCancel(context.TODO())
	rootCommand := &cobra.Command{
		Use:   `ar`,
		Short: `AR 命令行工具`,
		Long:  `AR 命令行工具，用于启动后端服务等子命令`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
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
