package command

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var Version string = "0.1.0"
var Commit string = "unknown"
var Date string = "unknown"
var BuiltBy string = "unknown"

func AddVersionCommand(command *cobra.Command) {
	command.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "显示版本信息",
		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("version: 显示版本信息")
			logrus.Debugf("version: Version=%s Commit=%s Date=%s BuiltBy=%s", Version, Commit, Date, BuiltBy)
			fmt.Println("ar version", Version)
			fmt.Println("commit", Commit)
			fmt.Println("date", Date)
			fmt.Println("built by", BuiltBy)
		},
	})
}
