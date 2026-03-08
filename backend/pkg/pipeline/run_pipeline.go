package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// Runner 流水线执行器（使用 OCI/opencontainer，不依赖 podman）。
type Runner struct {
	arRoot         string // /var/lib/ar
	pipelinesDir   string
	imagesStoreDir string
	runtimeRoot    string
}

// NewRunner 构造 Runner。arRoot 为流水线运行根目录，通常为 filepath.Dir(PipelinesDir)。
func NewRunner(arRoot, pipelinesDir, imagesStoreDir, runtimeRoot string) *Runner {
	return &Runner{
		arRoot:         arRoot,
		pipelinesDir:   pipelinesDir,
		imagesStoreDir: imagesStoreDir,
		runtimeRoot:    runtimeRoot,
	}
}

// Run 执行流水线：加载模板、用节点渲染生成 pipeline.json、解析为 DAG、按拓扑序执行并更新 pipeline.json。
// 若某步退出码非 0 则停止后续步骤并返回错误。
// taskID 若为空则自动生成；调用方可传入预生成的 taskID 以便与停止/恢复时注册的 cancel 对应。
// 返回 taskID 与错误。
func (r *Runner) Run(ctx context.Context, pipelineName string, nodes []RunNode, taskID string) (string, error) {
	logrus.Debugf("Runner.Run: pipelineName=%s nodes=%d", pipelineName, len(nodes))
	if len(nodes) == 0 {
		logrus.Error("Runner.Run: 节点列表为空")
		return "", fmt.Errorf("节点列表不能为空（请通过 -n 指定节点 JSON 文件）")
	}

	// 1. 加载模板并用节点渲染（支持 .template.json 内 Go template 语法），得到带 DAG 的步骤列表（不在此处拓扑排序）
	renderedSteps, err := LoadAndRenderTemplate(r.pipelinesDir, pipelineName, nodes)
	if err != nil {
		logrus.Errorf("Runner.Run: 加载模板失败 pipeline=%s: %v", pipelineName, err)
		return "", err
	}

	if taskID == "" {
		taskID = GenerateTaskID()
	}
	runDir := RunDir(r.arRoot, pipelineName, taskID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return "", fmt.Errorf("创建运行目录失败 %s: %w", runDir, err)
	}

	// 2. 生成 pipeline.json（执行计划 DAG）并写入任务目录
	runData := BuildRunData(taskID, pipelineName, renderedSteps, nodes)
	if err := WritePipelineJSON(runDir, runData); err != nil {
		return "", err
	}

	// 3. 从 pipeline.json 解析为 DAG，得到拓扑序执行顺序
	orderedSteps, err := OrderStepsFromRunData(runData.Steps)
	if err != nil {
		logrus.Errorf("Runner.Run: 解析 DAG 失败: %v", err)
		return "", err
	}

	// 步骤名 -> runData.Steps 下标，用于按执行顺序更新状态
	nameToIndex := make(map[string]int)
	for i := range runData.Steps {
		nameToIndex[runData.Steps[i].Name] = i
	}

	logrus.Infof("开始执行流水线: pipeline=%s taskId=%s runDir=%s", pipelineName, taskID, runDir)
	hostDataDir := filepath.Join(r.arRoot, "data")

	// 4. 按 DAG 拓扑序执行各步
	for _, orderedStep := range orderedSteps {
		stepIndex := nameToIndex[orderedStep.Name]
		step := &runData.Steps[stepIndex]
		nodeDir := NodeDir(runDir, stepIndex)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			return taskID, fmt.Errorf("创建节点目录失败 %s: %w", nodeDir, err)
		}

		step.Status = StatusRunning
		if err := WritePipelineJSON(runDir, runData); err != nil {
			return taskID, err
		}

		containerID := fmt.Sprintf("ar_%s_%s_%d", sanitizePipelineName(pipelineName), sanitizeStepNameForContainerID(step.Name, stepIndex+1), stepIndex+1)
		result := RunStep(ctx, r.runtimeRoot, r.imagesStoreDir, runDir, nodeDir, hostDataDir, containerID, step)

		if result.Err != nil {
			step.Status = StatusFailed
			_ = WritePipelineJSON(runDir, runData)
			logrus.Errorf("Runner.Run: 步骤 %s 执行失败: %v", step.Name, result.Err)
			return taskID, fmt.Errorf("步骤 %s 执行失败: %w", step.Name, result.Err)
		}
		if result.ExitCode != 0 {
			step.Status = StatusFailed
			_ = WritePipelineJSON(runDir, runData)
			logrus.Errorf("Runner.Run: 步骤 %s 退出码非 0: %d", step.Name, result.ExitCode)
			return taskID, fmt.Errorf("步骤 %s 退出码非 0: %d（按设计停止后续步骤）", step.Name, result.ExitCode)
		}

		step.Status = StatusSuccess
		if err := WritePipelineJSON(runDir, runData); err != nil {
			return taskID, err
		}
		logrus.Infof("步骤完成: %s", step.Name)
	}

	logrus.Infof("流水线执行完成: pipeline=%s taskId=%s", pipelineName, taskID)
	return taskID, nil
}

// Resume 从 pipeline.json 恢复流水线：读取任务目录、解析 DAG，从第一个未成功步骤开始按拓扑序继续执行。
func (r *Runner) Resume(ctx context.Context, taskID string) error {
	runDir, err := FindRunDirByTaskID(r.arRoot, taskID)
	if err != nil {
		return err
	}
	runData, err := ReadPipelineJSON(runDir)
	if err != nil {
		return fmt.Errorf("读取 pipeline.json 失败: %w", err)
	}
	pipelineName := runData.PipelineName

	orderedSteps, err := OrderStepsFromRunData(runData.Steps)
	if err != nil {
		return fmt.Errorf("解析 pipeline.json DAG 失败: %w", err)
	}
	nameToIndex := make(map[string]int)
	for i := range runData.Steps {
		nameToIndex[runData.Steps[i].Name] = i
	}

	startExecIndex := -1
	for i, s := range orderedSteps {
		if runData.Steps[nameToIndex[s.Name]].Status != StatusSuccess {
			startExecIndex = i
			break
		}
	}
	if startExecIndex < 0 {
		logrus.Infof("流水线已全部完成，无需恢复: pipeline=%s taskId=%s", pipelineName, taskID)
		return nil
	}

	logrus.Infof("恢复流水线: pipeline=%s taskId=%s runDir=%s 从步骤 %d 开始", pipelineName, taskID, runDir, startExecIndex+1)
	hostDataDir := filepath.Join(r.arRoot, "data")

	for i := startExecIndex; i < len(orderedSteps); i++ {
		orderedStep := orderedSteps[i]
		stepIndex := nameToIndex[orderedStep.Name]
		step := &runData.Steps[stepIndex]
		nodeDir := NodeDir(runDir, stepIndex)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			return fmt.Errorf("创建节点目录失败 %s: %w", nodeDir, err)
		}

		step.Status = StatusRunning
		if err := WritePipelineJSON(runDir, runData); err != nil {
			return err
		}

		containerID := fmt.Sprintf("ar_%s_%s_%d", sanitizePipelineName(pipelineName), sanitizeStepNameForContainerID(step.Name, stepIndex+1), stepIndex+1)
		result := RunStep(ctx, r.runtimeRoot, r.imagesStoreDir, runDir, nodeDir, hostDataDir, containerID, step)

		if result.Err != nil {
			step.Status = StatusFailed
			_ = WritePipelineJSON(runDir, runData)
			logrus.Errorf("Runner.Resume: 步骤 %s 执行失败: %v", step.Name, result.Err)
			return fmt.Errorf("步骤 %s 执行失败: %w", step.Name, result.Err)
		}
		if result.ExitCode != 0 {
			step.Status = StatusFailed
			_ = WritePipelineJSON(runDir, runData)
			logrus.Errorf("Runner.Resume: 步骤 %s 退出码非 0: %d", step.Name, result.ExitCode)
			return fmt.Errorf("步骤 %s 退出码非 0: %d（按设计停止后续步骤）", step.Name, result.ExitCode)
		}

		step.Status = StatusSuccess
		if err := WritePipelineJSON(runDir, runData); err != nil {
			return err
		}
		logrus.Infof("步骤完成: %s", step.Name)
	}

	logrus.Infof("流水线恢复执行完成: pipeline=%s taskId=%s", pipelineName, taskID)
	return nil
}

// RunDirFor 返回指定流水线任务的运行目录，便于 CLI 输出或恢复/停止逻辑使用。
func RunDirFor(arRoot, pipelineName, taskID string) string {
	return RunDir(arRoot, pipelineName, taskID)
}
