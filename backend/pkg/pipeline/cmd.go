package pipeline

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tangxusc/ar/backend/pkg/config"
)

func AddCommand(ctx context.Context, rootCommand *cobra.Command) {
	var inputArchive string

	loadCmd := &cobra.Command{
		Use:   "load",
		Short: "加载流水线 OCI 镜像并导入子镜像",
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputArchive == "" {
				return fmt.Errorf("请通过 -i 指定流水线镜像归档路径")
			}

			loader := NewLoader(
				config.PipelinesDir,
				config.ImagesStoreDir,
				config.LoadTmpRoot,
				config.OciRuntimeBinary,
			)
			return loader.Load(ctx, inputArchive)
		},
	}
	loadCmd.Flags().StringVarP(&inputArchive, "input", "i", "", "流水线镜像归档路径（.tar 或 .tar.gz）")
	_ = loadCmd.MarkFlagRequired("input")

	rootCommand.AddCommand(loadCmd)
}
