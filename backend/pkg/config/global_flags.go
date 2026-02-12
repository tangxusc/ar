package config

import "github.com/spf13/cobra"

var Debug bool

// OCI runtime 的 state 根目录（libcontainer 使用的 root，与 runc --root 含义一致）。
// 默认使用 /run/runc，rootless 场景可通过 flag 指定为 $XDG_RUNTIME_DIR/runc 等路径。
var OciRuntimeRoot string = "/var/lib/ar/runc"

func InitGlobalFlags(command *cobra.Command) {
	command.PersistentFlags().BoolVar(&Debug, "debug", true, "enable debug")
	command.PersistentFlags().StringVar(&OciRuntimeRoot, "oci-runtime-root", "/var/lib/ar/runc", "OCI runtime state root directory")
}
