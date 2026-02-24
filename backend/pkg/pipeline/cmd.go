package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tangxusc/ar/backend/pkg/config"
	"github.com/tangxusc/ar/backend/pkg/container"
)

func AddCommand(ctx context.Context, rootCommand *cobra.Command) {
	pipelineCmd := &cobra.Command{
		Use:   "pipeline",
		Short: "管理流水线（加载、执行、模板管理）",
	}
	rootCommand.AddCommand(pipelineCmd)

	var inputArchive string
	var loadNoCleanTmp bool

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
			return loader.Load(ctx, inputArchive, !loadNoCleanTmp)
		},
	}
	loadCmd.Flags().StringVarP(&inputArchive, "input", "i", "", "流水线镜像归档路径（.tar 或 .tar.gz）")
	loadCmd.Flags().BoolVar(&loadNoCleanTmp, "no-clean-tmp", false, "加载完成后保留临时目录（默认会清理 /tmp 下该流水线临时文件）")
	_ = loadCmd.MarkFlagRequired("input")

	pipelineCmd.AddCommand(loadCmd)

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
			taskID, err := runner.Run(ctx, runPipelineName, nodes, "")
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
	pipelineCmd.AddCommand(runCmd)

	// ar pipeline task：任务相关操作（list / stop / resume 等）
	var listPipelineName string
	var stopTaskID string
	var resumeTaskID string
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "管理流水线任务（列出、停止、恢复等）",
	}
	pipelineCmd.AddCommand(taskCmd)

	// ar pipeline task list
	taskListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "列出正在运行的流水线及其正在运行的容器",
		RunE: func(cmd *cobra.Command, args []string) error {
			arRoot := filepath.Dir(config.PipelinesDir)
			return listRunningTasks(arRoot, listPipelineName)
		},
	}
	taskListCmd.Flags().StringVarP(&listPipelineName, "pipeline", "p", "", "按流水线名称过滤，仅展示指定流水线的运行任务")
	taskCmd.AddCommand(taskListCmd)

	// ar pipeline task stop -t <taskId>
	taskStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "停止指定流水线任务（按 taskId）",
		Long:  "根据 taskId 查找对应流水线运行目录，停止正在运行的容器并将 pending/running 步骤状态标记为 cancelled，写回 pipeline.json（参照 design/停止流水线流程.md）。",
		RunE: func(cmd *cobra.Command, args []string) error {
			if stopTaskID == "" {
				return fmt.Errorf("请通过 -t 指定要停止的流水线任务 ID（taskId）")
			}
			arRoot := filepath.Dir(config.PipelinesDir)
			return stopPipelineTask(arRoot, config.OciRuntimeRoot, stopTaskID)
		},
	}
	taskStopCmd.Flags().StringVarP(&stopTaskID, "task", "t", "", "要停止的流水线任务 ID（必填）")
	_ = taskStopCmd.MarkFlagRequired("task")
	taskCmd.AddCommand(taskStopCmd)

	// ar pipeline task resume -t <taskId>
	taskResumeCmd := &cobra.Command{
		Use:   "resume",
		Short: "恢复被取消的流水线任务（按 taskId）",
		Long:  "根据 taskId 查找对应流水线运行目录，读取 pipeline.json，确定上次执行到的步骤并从该步骤开始继续执行（参照 design/恢复流水线执行.md）。",
		RunE: func(cmd *cobra.Command, args []string) error {
			if resumeTaskID == "" {
				return fmt.Errorf("请通过 -t 指定要恢复的流水线任务 ID（taskId）")
			}
			arRoot := filepath.Dir(config.PipelinesDir)
			runner := NewRunner(arRoot, config.PipelinesDir, config.ImagesStoreDir, config.OciRuntimeRoot)
			if err := runner.Resume(ctx, resumeTaskID); err != nil {
				return err
			}
			logrus.Infof("流水线任务已恢复执行: taskId=%s", resumeTaskID)
			return nil
		},
	}
	taskResumeCmd.Flags().StringVarP(&resumeTaskID, "task", "t", "", "要恢复的流水线任务 ID（必填）")
	_ = taskResumeCmd.MarkFlagRequired("task")
	taskCmd.AddCommand(taskResumeCmd)

	addImageCommand(rootCommand)
	addPipelineCommand(pipelineCmd)
	addNodeCommand(rootCommand)
}

// listRunningTasks 扫描 /var/lib/ar/tasks 目录，列出所有包含 running 步骤的流水线任务。
// arRoot 一般为 /var/lib/ar。
// filterPipelineName 若非空，则仅展示该流水线（按 sanitizePipelineName 处理后的名称匹配目录）。
func listRunningTasks(arRoot, filterPipelineName string) error {
	root := filepath.Join(arRoot, "tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.Info("当前无任何流水线任务")
			return nil
		}
		return fmt.Errorf("读取任务根目录失败 %s: %w", root, err)
	}

	filterNameSanitized := ""
	if filterPipelineName != "" {
		filterNameSanitized = sanitizePipelineName(filterPipelineName)
		if filterNameSanitized == "" {
			return fmt.Errorf("流水线名称无效: %s", filterPipelineName)
		}
	}

	type runningRow struct {
		pipelineName string
		taskID       string
		stepName     string
		containerID  string
	}
	var rows []runningRow

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pipelineDirName := e.Name() // 已经是 sanitize 后的目录名
		if filterNameSanitized != "" && pipelineDirName != filterNameSanitized {
			continue
		}
		pipelineTasksDir := filepath.Join(root, pipelineDirName)
		taskEntries, err := os.ReadDir(pipelineTasksDir)
		if err != nil {
			logrus.WithError(err).Warnf("读取流水线任务目录失败: %s", pipelineTasksDir)
			continue
		}
		for _, te := range taskEntries {
			if !te.IsDir() {
				continue
			}
			taskID := te.Name()
			runDir := filepath.Join(pipelineTasksDir, taskID)
			runData, err := ReadPipelineJSON(runDir)
			if err != nil {
				logrus.WithError(err).Warnf("读取任务状态失败: %s", filepath.Join(runDir, "pipeline.json"))
				continue
			}
			pipelineName := runData.PipelineName
			for i, step := range runData.Steps {
				if step.Status != StatusRunning {
					continue
				}
				containerID := fmt.Sprintf("ar_%s_%s_%d", pipelineDirName, step.Name, i+1)
				rows = append(rows, runningRow{
					pipelineName: pipelineName,
					taskID:       taskID,
					stepName:     step.Name,
					containerID:  containerID,
				})
			}
		}
	}

	if len(rows) == 0 {
		if filterPipelineName != "" {
			logrus.Infof("未找到正在运行的流水线任务（pipeline=%s）", filterPipelineName)
		} else {
			logrus.Info("当前无正在运行的流水线任务")
		}
		return nil
	}

	fmt.Println("PIPELINE\tTASK_ID\tCONTAINER_ID\tSTEP")
	for _, r := range rows {
		fmt.Printf("%s\t%s\t%s\t%s\n", r.pipelineName, r.taskID, r.containerID, r.stepName)
	}
	return nil
}

// stopPipelineTask 参照 design/停止流水线流程.md，实现按 taskId 停止流水线任务：
// 1. 根据 taskId 找到运行目录（包含 pipeline.json）。
// 2. 标记 pending/running 步骤为 cancelled。
// 3. 对于 running 步骤，调用 OCI runtime 停止并删除对应容器。
// 4. 写回 pipeline.json。
func stopPipelineTask(arRoot, runtimeRoot, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("taskId 不能为空")
	}
	runDir, err := FindRunDirByTaskID(arRoot, taskID)
	if err != nil {
		return err
	}

	runData, err := ReadPipelineJSON(runDir)
	if err != nil {
		return fmt.Errorf("读取 pipeline.json 失败: %w", err)
	}

	// 根据 runDir 反推出流水线目录名（已是 sanitize 之后的名字）。
	pipelineDirName := filepath.Base(filepath.Dir(runDir))

	// 按设计：正在运行的节点需要停止容器，未运行的节点标记为取消。
	for i := range runData.Steps {
		step := &runData.Steps[i]
		switch step.Status {
		case StatusRunning:
			// 计算容器 ID，与 Run()/Resume() 时保持一致。
			containerID := fmt.Sprintf("ar_%s_%s_%d", pipelineDirName, step.Name, i+1)
			if err := container.StopAndRemoveOCIContainers(runtimeRoot, containerID); err != nil {
				logrus.WithError(err).Warnf("停止流水线任务容器失败: %s", containerID)
			}
			step.Status = StatusCancelled
		case StatusPending:
			step.Status = StatusCancelled
		default:
			// success / failed / cancelled 等状态保持不变
		}
	}

	if err := WritePipelineJSON(runDir, runData); err != nil {
		return fmt.Errorf("写回 pipeline.json 失败: %w", err)
	}

	logrus.Infof("流水线任务已停止: taskId=%s runDir=%s", taskID, runDir)
	return nil
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
