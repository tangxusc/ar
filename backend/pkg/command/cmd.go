package command

import (
	"context"

	"github.com/spf13/cobra"
)

type AddCommand func(ctx context.Context, cancelFunc func(), command *cobra.Command)

var commandSlice = make([]AddCommand, 0)

func RegisterCommand(command AddCommand) {
	commandSlice = append(commandSlice, command)
}

func BuildCommands(ctx context.Context, cancelFunc func(), command *cobra.Command) {
	for _, f := range commandSlice {
		f(ctx, cancelFunc, command)
	}
}
