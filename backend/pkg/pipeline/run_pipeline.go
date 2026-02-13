package pipeline

import (
	"context"
	"fmt"
	"os"

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

// Run 执行流水线：加载模板、拓扑序、渲染、创建任务目录、按序执行各步并更新 pipeline.json。
// 若某步退出码非 0 则停止后续步骤并返回错误。
// 返回 taskID 与错误。
func (r *Runner) Run(ctx context.Context, pipelineName string, nodes []RunNode) (taskID string, err error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("节点列表不能为空（请通过 -n 指定节点 JSON 文件）")
	}
	node := nodes[0] // 设计：多节点时可按步分配，此处简化为首节点用于全部步骤

	steps, err := LoadTemplate(r.pipelinesDir, pipelineName)
	if err != nil {
		return "", err
	}
	ordered, err := TopoOrder(steps)
	if err != nil {
		return "", err
	}

	taskID = GenerateTaskID()
	runDir := RunDir(r.arRoot, pipelineName, taskID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return "", fmt.Errorf("创建运行目录失败 %s: %w", runDir, err)
	}
	runData := BuildRunData(taskID, pipelineName, ordered, node)
	if err := WritePipelineJSON(runDir, runData); err != nil {
		return "", err
	}

	logrus.Infof("开始执行流水线: pipeline=%s taskId=%s runDir=%s", pipelineName, taskID, runDir)

	for i := range runData.Steps {
		step := &runData.Steps[i]
		nodeDir := NodeDir(runDir, i)
		if err := os.MkdirAll(nodeDir, 0755); err != nil {
			return taskID, fmt.Errorf("创建节点目录失败 %s: %w", nodeDir, err)
		}

		step.Status = StatusRunning
		if err := WritePipelineJSON(runDir, runData); err != nil {
			return taskID, err
		}

		containerID := fmt.Sprintf("ar_%s_%s_%d", sanitizePipelineName(pipelineName), step.Name, i+1)
		result := RunStep(ctx, r.runtimeRoot, r.imagesStoreDir, runDir, nodeDir, containerID, step)

		if result.Err != nil {
			step.Status = StatusFailed
			_ = WritePipelineJSON(runDir, runData)
			return taskID, fmt.Errorf("步骤 %s 执行失败: %w", step.Name, result.Err)
		}
		if result.ExitCode != 0 {
			step.Status = StatusFailed
			_ = WritePipelineJSON(runDir, runData)
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

// RunDirFor 返回指定流水线任务的运行目录，便于 CLI 输出或恢复/停止逻辑使用。
func RunDirFor(arRoot, pipelineName, taskID string) string {
	return RunDir(arRoot, pipelineName, taskID)
}
