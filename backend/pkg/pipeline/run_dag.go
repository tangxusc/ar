package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// LoadTemplate 从 pipelinesDir 读取 pipelineName.template.json（纯 JSON），返回步骤列表。
// 若模板内含 Go template 语法（如 {{range .nodes}}），请使用 LoadAndRenderTemplate。
func LoadTemplate(pipelinesDir, pipelineName string) ([]TemplateStep, error) {
	return loadTemplateWithContext(pipelinesDir, pipelineName, nil, nil)
}

// LoadAndRenderTemplate 读取模板文件，用 nodes 作为上下文渲染（支持 {{.nodes}}、{{range}} 等），再解析为步骤列表。
// args 为可选键值对参数（来自 --args 指定的 JSON 文件），在模板中通过 {{index .args "key"}} 或 {{arg .args "key"}} 读取。
// 用于模板中含 Go template 语法的 pipeline_name.template.json。
func LoadAndRenderTemplate(pipelinesDir, pipelineName string, nodes []RunNode, args map[string]interface{}) ([]TemplateStep, error) {
	return loadTemplateWithContext(pipelinesDir, pipelineName, nodes, args)
}

func loadTemplateWithContext(pipelinesDir, pipelineName string, nodes []RunNode, args map[string]interface{}) ([]TemplateStep, error) {
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

	var toParse []byte
	if nodes != nil {
		tpl, err := template.New("").Funcs(templateFuncs).Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("解析流水线模板语法失败 %s: %w", path, err)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, buildRenderContext(nodes, args)); err != nil {
			return nil, fmt.Errorf("渲染流水线模板失败 %s: %w", path, err)
		}
		toParse = buf.Bytes()
	} else {
		toParse = data
	}

	var steps []TemplateStep
	if err := json.Unmarshal(toParse, &steps); err != nil {
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

// StepsToLevels 根据 steps 的 DAG 依赖关系，将步骤按层级分组（BFS 按批次）。
// 同一层内的步骤互不依赖，可并行执行；不同层之间保持顺序。
// 返回 [][]PipelineStepState，每个子切片为一层。
func StepsToLevels(steps []PipelineStepState) ([][]PipelineStepState, error) {
	nameToStep := make(map[string]PipelineStepState)
	for _, s := range steps {
		nameToStep[s.Name] = s
	}
	inDegree := make(map[string]int)
	for _, s := range steps {
		if _, ok := inDegree[s.Name]; !ok {
			inDegree[s.Name] = 0
		}
		for _, next := range s.Nodes {
			inDegree[next]++
		}
	}
	// 初始队列：所有入度为 0 的节点
	queue := make([]string, 0)
	for name, d := range inDegree {
		if d == 0 {
			queue = append(queue, name)
		}
	}
	var levels [][]PipelineStepState
	total := 0
	for len(queue) > 0 {
		// 当前批次（层）就是 queue 中的全部节点
		level := make([]PipelineStepState, 0, len(queue))
		nextQueue := make([]string, 0)
		for _, name := range queue {
			level = append(level, nameToStep[name])
			for _, next := range nameToStep[name].Nodes {
				inDegree[next]--
				if inDegree[next] == 0 {
					nextQueue = append(nextQueue, next)
				}
			}
		}
		levels = append(levels, level)
		total += len(level)
		queue = nextQueue
	}
	if total != len(steps) {
		return nil, fmt.Errorf("pipeline.json 中的 DAG 存在环或未知节点引用，无法得到层级顺序")
	}
	return levels, nil
}

// OrderStepsFromRunData 根据 pipeline.json 中的 steps（含 Nodes 依赖）解析为 DAG 并做拓扑排序，返回执行顺序。
// 用于「先写入 pipeline.json，再解析为 DAG 再运行」的流程。
func OrderStepsFromRunData(steps []PipelineStepState) ([]PipelineStepState, error) {
	nameToStep := make(map[string]PipelineStepState)
	for _, s := range steps {
		nameToStep[s.Name] = s
	}
	inDegree := make(map[string]int)
	for _, s := range steps {
		if _, ok := inDegree[s.Name]; !ok {
			inDegree[s.Name] = 0
		}
		for _, next := range s.Nodes {
			inDegree[next]++
		}
	}
	var order []PipelineStepState
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
		return nil, fmt.Errorf("pipeline.json 中的 DAG 存在环或未知节点引用，无法得到拓扑序")
	}
	return order, nil
}

// NodeTemplateData 供模板渲染使用的单节点数据（与 RunNode 对应，字段首字母大写以便 template 访问）。
type NodeTemplateData struct {
	IP         string
	IntranetIP string
	Port       string
	Username   string
	Password   string
	LabelsStr  string
}

// buildRenderContext 根据节点列表构建模板上下文，包含 .nodes 数组和 .args 键值对。
// 模板中可用 {{.nodes}}、{{(index .nodes 0).IP}}、{{range .nodes}}、{{index .args "key"}}、{{arg .args "key"}} 等。
func buildRenderContext(nodes []RunNode, args map[string]interface{}) map[string]interface{} {
	list := make([]NodeTemplateData, 0, len(nodes))
	for _, n := range nodes {
		list = append(list, NodeTemplateData{
			IP:         n.IP,
			IntranetIP: n.IntranetIP,
			Port:       n.Port,
			Username:   n.Username,
			Password:   n.Password,
			LabelsStr:  labelsString(n.Labels),
		})
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	return map[string]interface{}{"nodes": list, "args": args}
}

// RenderStep 用 nodes 数组渲染 step 的 entrypoint/args/env。模板中使用 .nodes（如 {{(index .nodes 0).IP}}、{{range .nodes}}）。
func RenderStep(step TemplateStep, nodes []RunNode) TemplateStep {
	ctx := buildRenderContext(nodes, nil)
	env := make([]string, 0, len(step.Env))
	for _, e := range step.Env {
		env = append(env, renderString(e, ctx))
	}
	args := make([]string, 0, len(step.Args))
	for _, a := range step.Args {
		args = append(args, renderString(a, ctx))
	}
	return TemplateStep{
		Name:       step.Name,
		Image:      step.Image,
		Entrypoint: renderString(step.Entrypoint, ctx),
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

func labelHas(labelsStr, key, value string) bool {
	if labelsStr == "" || key == "" {
		return false
	}
	for _, label := range strings.Split(labelsStr, ",") {
		parts := strings.SplitN(strings.TrimSpace(label), "=", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] == key && parts[1] == value {
			return true
		}
	}
	return false
}

// getNodeField 根据字段名获取 NodeTemplateData 对应字段的值（大小写不敏感）。
func getNodeField(n NodeTemplateData, fieldName string) string {
	switch strings.ToLower(fieldName) {
	case "ip":
		return n.IP
	case "intranetip", "intranet_ip":
		return n.IntranetIP
	case "port":
		return n.Port
	case "username":
		return n.Username
	case "password":
		return n.Password
	case "labelsstr":
		return n.LabelsStr
	default:
		return ""
	}
}

// templateFuncs 供 renderString 使用的模板函数，如 join、len、arg 等。
var templateFuncs = template.FuncMap{
	"join":     func(sep string, s []string) string { return strings.Join(s, sep) },
	"labelHas": labelHas,
	// arg 从 .args 中读取指定 key 的值，key 不存在时返回空字符串。用法：{{arg .args "version"}}
	"arg": func(args map[string]interface{}, key string) interface{} {
		if args == nil {
			return ""
		}
		if v, ok := args[key]; ok {
			return v
		}
		return ""
	},
	"ipsByLabel": func(nodes []NodeTemplateData, key, value string) []string {
		ips := make([]string, 0, len(nodes))
		for _, n := range nodes {
			if labelHas(n.LabelsStr, key, value) {
				ips = append(ips, n.IP)
			}
		}
		return ips
	},
	"getNodeFieldValueByLabel": func(nodes []NodeTemplateData, labelKey, labelValue, fieldName string) []string {
		result := make([]string, 0, len(nodes))
		for _, n := range nodes {
			if labelHas(n.LabelsStr, labelKey, labelValue) {
				result = append(result, getNodeField(n, fieldName))
			}
		}
		return result
	},
	"indicesByLabel": func(nodes []NodeTemplateData, key, value string) []int {
		indices := make([]int, 0, len(nodes))
		for i, n := range nodes {
			if labelHas(n.LabelsStr, key, value) {
				indices = append(indices, i)
			}
		}
		return indices
	},
	"indicesByNotLabel": func(nodes []NodeTemplateData, key, value string) []int {
		indices := make([]int, 0, len(nodes))
		for i, n := range nodes {
			if !labelHas(n.LabelsStr, key, value) {
				indices = append(indices, i)
			}
		}
		return indices
	},
	"len": func(slice interface{}) int {
		switch v := slice.(type) {
		case []NodeTemplateData:
			return len(v)
		case []string:
			return len(v)
		case []int:
			return len(v)
		default:
			return 0
		}
	},
}

// renderString 使用 Go text/template 渲染字符串，上下文为 .nodes 等。
func renderString(s string, data interface{}) string {
	t, err := template.New("").Funcs(templateFuncs).Parse(s)
	if err != nil {
		return s
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return s
	}
	return buf.String()
}

// BuildRunData 根据拓扑序与节点列表生成初始 PipelineRunData（所有步骤 pending）。模板渲染时使用完整 nodes 数组。
func BuildRunData(taskID, pipelineName string, orderedSteps []TemplateStep, nodes []RunNode) *PipelineRunData {
	steps := make([]PipelineStepState, 0, len(orderedSteps))
	for _, s := range orderedSteps {
		rendered := RenderStep(s, nodes)
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

// RunDir 返回流水线运行目录：/var/lib/ar/tasks/pipelineName/taskID/
func RunDir(arRoot, pipelineName, taskID string) string {
	name := sanitizePipelineName(pipelineName)
	return filepath.Join(arRoot, "tasks", name, taskID)
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

// sanitizeStepNameForContainerID 将步骤名转为 OCI/runc 允许的容器 ID 片段（仅 [a-zA-Z0-9_.-]）。
// 若结果为空（如全中文步骤名），返回 "step" + index 作为回退，保证唯一且合法。
func sanitizeStepNameForContainerID(stepName string, stepIndex int) string {
	sanitized := sanitizePipelineName(stepName)
	if sanitized != "" {
		return sanitized
	}
	return fmt.Sprintf("step%d", stepIndex)
}

// WritePipelineJSON 将 runData 写入 runDir/pipeline.json，并 Sync 确保容器启动前落盘。
func WritePipelineJSON(runDir string, runData *PipelineRunData) error {
	if runData == nil {
		return fmt.Errorf("runData 不能为空")
	}
	path := filepath.Join(runDir, "pipeline.json")
	data, err := json.MarshalIndent(runData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 pipeline.json 失败: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("序列化 pipeline.json 结果为空")
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入 pipeline.json 失败 %s: %w", path, err)
	}
	// 确保落盘后再启动容器，避免容器内读到空或未刷新的文件
	f, err := os.Open(path)
	if err == nil {
		_ = f.Sync()
		_ = f.Close()
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

// FindRunDirByTaskID 根据 taskID 在 arRoot/tasks 下扫描各流水线目录，找到包含 pipeline.json 的任务目录。
// 用于停止/恢复时仅知 taskId 的场景。返回 runDir 与 nil，未找到则返回错误。
func FindRunDirByTaskID(arRoot, taskID string) (string, error) {
	if taskID == "" {
		return "", fmt.Errorf("taskId 不能为空")
	}
	root := filepath.Join(arRoot, "tasks")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("读取 ar 根目录失败 %s: %w", arRoot, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		runDir := filepath.Join(root, e.Name(), taskID)
		path := filepath.Join(runDir, "pipeline.json")
		if _, err := os.Stat(path); err == nil {
			return runDir, nil
		}
	}
	return "", fmt.Errorf("未找到 taskId 对应的流水线任务目录: %s", taskID)
}

// GenerateTaskID 生成唯一任务 ID：时间戳_随机数（设计文档约定）。放在本文件以便所有平台可编译。
func GenerateTaskID() string {
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), time.Now().UnixNano()%10000)
}
