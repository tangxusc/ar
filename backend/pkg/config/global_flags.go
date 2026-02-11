package config

import "github.com/spf13/cobra"

var Debug bool

func InitGlobalFlags(command *cobra.Command) {
	command.PersistentFlags().BoolVar(&Debug, "debug", true, "enable debug")
}
