package config

import "github.com/spf13/cobra"

var Debug bool

// OCI runtime 的 state 根目录（libcontainer 使用的 root，与 runc --root 含义一致）。
// 默认使用 /run/runc，rootless 场景可通过 flag 指定为 $XDG_RUNTIME_DIR/runc 等路径。
var OciRuntimeRoot string = "/var/lib/ar/runc"
var PipelinesDir string = "/var/lib/ar/pipelines"
var ImagesStoreDir string = "/var/lib/ar/images"
var LoadTmpRoot string = "/tmp"
var NodesDir string = "/var/lib/ar/nodes"

func InitGlobalFlags(command *cobra.Command) {
	command.PersistentFlags().BoolVar(&Debug, "debug", true, "enable debug")
	command.PersistentFlags().StringVar(&OciRuntimeRoot, "oci-runtime-root", "/var/lib/ar/runc", "OCI runtime state root directory")
	command.PersistentFlags().StringVar(&PipelinesDir, "pipelines-dir", "/var/lib/ar/pipelines", "directory used to store pipeline templates")
	command.PersistentFlags().StringVar(&ImagesStoreDir, "images-store-dir", "/var/lib/ar/images", "directory used to store loaded OCI images")
	command.PersistentFlags().StringVar(&LoadTmpRoot, "load-tmp-root", "/tmp", "temporary root directory used by ar load")
	command.PersistentFlags().StringVar(&NodesDir, "nodes-dir", "/var/lib/ar/nodes", "nodes directory")
}
