package main

import (
	"context"
	"os"

	//nolint:revive // Enable cgroup manager to manage devices.
	_ "github.com/opencontainers/cgroups/devices"
	"github.com/opencontainers/runc/libcontainer"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"
	"github.com/tangxusc/ar/backend/pkg/command"
	"github.com/tangxusc/ar/backend/pkg/pipeline"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/tangxusc/ar/backend/pkg/config"
	"github.com/tangxusc/ar/backend/pkg/web"
)

func init() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		libcontainer.Init()
	}
}

func NewCommand() (*cobra.Command, context.Context, context.CancelFunc) {
	ctx, cancelFunc, rootCommand := command.NewRootCommand()
	config.InitGlobalFlags(rootCommand)

	command.AddVersionCommand(rootCommand)
	web.AddCommand(ctx, cancelFunc, rootCommand)
	pipeline.AddCommand(ctx, rootCommand)
	command.BuildCommands(ctx, cancelFunc, rootCommand)

	return rootCommand, ctx, cancelFunc
}

//go:generate gqlgen generate
func main() {
	command, _, _ := NewCommand()
	if err := command.Execute(); err != nil {
		logrus.Fatalln(err)
	}

}
