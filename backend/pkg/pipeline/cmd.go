package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
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
				config.OciRuntimeRoot,
			)
			return loader.Load(ctx, inputArchive)
		},
	}
	loadCmd.Flags().StringVarP(&inputArchive, "input", "i", "", "流水线镜像归档路径（.tar 或 .tar.gz）")
	_ = loadCmd.MarkFlagRequired("input")

	rootCommand.AddCommand(loadCmd)

	// ar run：按 design/执行流水线流程.md 使用 OCI 规范执行流水线（不依赖 podman）
	var runPipelineName, runNodesPath string
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "执行流水线（按 DAG 顺序运行 OCI 容器）",
		Long:  "读取 pipeline_name.template.json，根据节点列表渲染并按拓扑序执行各步骤；使用 OCI Runtime（libcontainer）运行容器，挂载 /tasks 与 /current-task。",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runPipelineName == "" {
				return fmt.Errorf("请通过 -p 指定流水线名称（与 .template.json 前缀一致）")
			}
			if runNodesPath == "" {
				return fmt.Errorf("请通过 -n 指定节点列表 JSON 文件路径")
			}
			data, err := os.ReadFile(runNodesPath)
			if err != nil {
				return fmt.Errorf("读取节点文件失败 %s: %w", runNodesPath, err)
			}
			nodes, err := ParseNodesFile(data)
			if err != nil {
				return fmt.Errorf("解析节点 JSON 失败: %w", err)
			}
			arRoot := filepath.Dir(config.PipelinesDir)
			runner := NewRunner(arRoot, config.PipelinesDir, config.ImagesStoreDir, config.OciRuntimeRoot)
			taskID, err := runner.Run(ctx, runPipelineName, nodes)
			if err != nil {
				return err
			}
			fmt.Println("taskId:", taskID)
			return nil
		},
	}
	runCmd.Flags().StringVarP(&runPipelineName, "pipeline", "p", "", "流水线名称（对应 pipelines-dir 下的 <name>.template.json）")
	runCmd.Flags().StringVarP(&runNodesPath, "nodes", "n", "", "节点列表 JSON 文件路径（格式见 design/节点管理.md）")
	_ = runCmd.MarkFlagRequired("pipeline")
	_ = runCmd.MarkFlagRequired("nodes")
	rootCommand.AddCommand(runCmd)

	addImageCommand(rootCommand)
}

func addImageCommand(rootCommand *cobra.Command) {
	imageCmd := &cobra.Command{
		Use:   "image",
		Short: "管理 OCI 镜像（列表、删除、清理）",
		Long:  "对 load -i 导入的镜像进行列表、删除或 prune 清理。镜像存储在 --images-store-dir 目录下。",
	}
	rootCommand.AddCommand(imageCmd)

	// image list / image ls
	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "列出已导入的镜像",
		RunE: func(cmd *cobra.Command, args []string) error {
			list, err := ListImages(config.ImagesStoreDir)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				logrus.Info("当前无已导入镜像")
				return nil
			}
			for _, e := range list {
				fmt.Printf("%s\t%s\n", e.Name, e.Ref)
			}
			return nil
		},
	}
	imageCmd.AddCommand(listCmd)

	// image rm
	rmCmd := &cobra.Command{
		Use:   "rm [镜像名...]",
		Short: "删除一个或多个已导入的镜像",
		Long:  "镜像名为存储目录名（与 list 输出第一列一致），可指定多个。",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请指定要删除的镜像名，例如: ar image rm <name>")
			}
			for _, name := range args {
				if err := DeleteImage(config.ImagesStoreDir, name); err != nil {
					return err
				}
				logrus.Infof("已删除镜像: %s", name)
			}
			return nil
		},
	}
	imageCmd.AddCommand(rmCmd)

	// image prune
	var pruneAll bool
	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "清理未使用的镜像",
		Long:  "删除未被任何流水线模板（*.template.json）引用的镜像。使用 --all 时删除全部已导入镜像。",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pruneAll {
				pruned, err := PruneAllImages(config.ImagesStoreDir)
				if err != nil {
					return err
				}
				for _, name := range pruned {
					logrus.Infof("已删除镜像: %s", name)
				}
				logrus.Infof("共删除 %d 个镜像", len(pruned))
				return nil
			}
			pruned, err := PruneImages(config.ImagesStoreDir, config.PipelinesDir)
			if err != nil {
				return err
			}
			for _, name := range pruned {
				logrus.Infof("已删除未引用镜像: %s", name)
			}
			logrus.Infof("共删除 %d 个未引用镜像", len(pruned))
			return nil
		},
	}
	pruneCmd.Flags().BoolVar(&pruneAll, "all", false, "删除所有已导入镜像（慎用）")
	imageCmd.AddCommand(pruneCmd)
}
