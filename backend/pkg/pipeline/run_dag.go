package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// LoadTemplate 从 pipelinesDir 读取 pipelineName.template.json，返回步骤列表。
func LoadTemplate(pipelinesDir, pipelineName string) ([]TemplateStep, error) {
	name := sanitizePipelineName(pipelineName)
	if name == "" {
		return nil, fmt.Errorf("流水线名称无效: %s", pipelineName)
	}
	path := filepath.Join(pipelinesDir, name+".template.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("流水线模板不存在: %s（请先 load 对应流水线镜像）", path)
		}
		return nil, fmt.Errorf("读取流水线模板失败 %s: %w", path, err)
	}

	var steps []TemplateStep
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, fmt.Errorf("解析流水线模板失败 %s: %w", path, err)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("流水线模板为空: %s", path)
	}
	return steps, nil
}

// TopoOrder 返回 DAG 的拓扑序（无依赖或依赖已列出的先执行）。steps 中 nodes 表示后继，即 name -> nodes 的边。
// 返回顺序为执行顺序：先执行无后继或依赖已满足的节点。
func TopoOrder(steps []TemplateStep) ([]TemplateStep, error) {
	nameToStep := make(map[string]TemplateStep)
	for _, s := range steps {
		nameToStep[s.Name] = s
	}
	// 计算入度：被谁依赖（谁指向我）
	inDegree := make(map[string]int)
	for _, s := range steps {
		if _, ok := inDegree[s.Name]; !ok {
			inDegree[s.Name] = 0
		}
		for _, next := range s.Nodes {
			inDegree[next]++
		}
	}
	var order []TemplateStep
	queue := make([]string, 0)
	for name, d := range inDegree {
		if d == 0 {
			queue = append(queue, name)
		}
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		order = append(order, nameToStep[name])
		for _, next := range nameToStep[name].Nodes {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(order) != len(steps) {
		return nil, fmt.Errorf("流水线模板存在环或未知节点引用，无法得到拓扑序")
	}
	return order, nil
}

// RenderStep 用 node 渲染 step 的 env（以及 args 若含模板）。占位符：node_ip, node_port, node_username, node_password, node_labels。
func RenderStep(step TemplateStep, node RunNode) TemplateStep {
	labelsStr := labelsString(node.Labels)
	m := map[string]string{
		"node_ip":       node.IP,
		"node_port":     node.Port,
		"node_username": node.Username,
		"node_password": node.Password,
		"node_labels":   labelsStr,
	}
	env := make([]string, 0, len(step.Env))
	for _, e := range step.Env {
		env = append(env, renderString(e, m))
	}
	args := make([]string, 0, len(step.Args))
	for _, a := range step.Args {
		args = append(args, renderString(a, m))
	}
	return TemplateStep{
		Name:       step.Name,
		Image:      step.Image,
		Entrypoint: renderString(step.Entrypoint, m),
		Args:       args,
		Env:        env,
		Nodes:      step.Nodes,
	}
}

func labelsString(labels []Label) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	for i, l := range labels {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(l.Key)
		b.WriteString("=")
		b.WriteString(l.Value)
	}
	return b.String()
}

// 设计文档使用 {{node_ip}}，Go text/template 需要 {{.node_ip}}，此处做兼容替换。
func renderString(s string, m map[string]string) string {
	for k := range m {
		s = strings.ReplaceAll(s, "{{"+k+"}}", "{{."+k+"}}")
	}
	t, err := template.New("").Parse(s)
	if err != nil {
		return s
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, m); err != nil {
		return s
	}
	return buf.String()
}

// BuildRunData 根据拓扑序与节点生成初始 PipelineRunData（所有步骤 pending）。
func BuildRunData(taskID, pipelineName string, orderedSteps []TemplateStep, node RunNode) *PipelineRunData {
	steps := make([]PipelineStepState, 0, len(orderedSteps))
	for _, s := range orderedSteps {
		rendered := RenderStep(s, node)
		steps = append(steps, PipelineStepState{
			Name:       rendered.Name,
			Image:      rendered.Image,
			Status:     StatusPending,
			Entrypoint: rendered.Entrypoint,
			Args:       rendered.Args,
			Env:        rendered.Env,
			Nodes:      rendered.Nodes,
		})
	}
	return &PipelineRunData{
		TaskID:       taskID,
		PipelineName: pipelineName,
		Steps:        steps,
	}
}

// RunDir 返回流水线运行目录：/var/lib/ar/pipelineName/taskID/
func RunDir(arRoot, pipelineName, taskID string) string {
	name := sanitizePipelineName(pipelineName)
	return filepath.Join(arRoot, name, taskID)
}

// NodeDir 返回某步骤的当前任务目录：runDir/node{index}，设计为挂载到容器 /current-task/
func NodeDir(runDir string, stepIndex int) string {
	return filepath.Join(runDir, fmt.Sprintf("node%d", stepIndex+1))
}

func sanitizePipelineName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "._-")
}

// WritePipelineJSON 将 runData 写入 runDir/pipeline.json。
func WritePipelineJSON(runDir string, runData *PipelineRunData) error {
	path := filepath.Join(runDir, "pipeline.json")
	data, err := json.MarshalIndent(runData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 pipeline.json 失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入 pipeline.json 失败 %s: %w", path, err)
	}
	logrus.Debugf("已写入 %s", path)
	return nil
}

// ReadPipelineJSON 从 runDir 读取 pipeline.json。
func ReadPipelineJSON(runDir string) (*PipelineRunData, error) {
	path := filepath.Join(runDir, "pipeline.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var runData PipelineRunData
	if err := json.Unmarshal(data, &runData); err != nil {
		return nil, fmt.Errorf("解析 pipeline.json 失败: %w", err)
	}
	return &runData, nil
}

// FindRunDirByTaskID 根据 taskID 在 arRoot 下扫描各流水线目录，找到包含 pipeline.json 的任务目录。
// 用于停止/恢复时仅知 taskId 的场景。返回 runDir 与 nil，未找到则返回错误。
func FindRunDirByTaskID(arRoot, taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("taskId 不能为空")
	}
	entries, err := os.ReadDir(arRoot)
	if err != nil {
		return "", fmt.Errorf("读取 ar 根目录失败 %s: %w", arRoot, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		runDir := filepath.Join(arRoot, e.Name(), taskID)
		path := filepath.Join(runDir, "pipeline.json")
		if _, err := os.Stat(path); err == nil {
			return runDir, nil
		}
	}
	return "", fmt.Errorf("未找到 taskId 对应的流水线任务目录: %s", taskID)
}
