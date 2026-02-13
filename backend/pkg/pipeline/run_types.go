package pipeline

import "encoding/json"

// RunNode 表示执行流水线时的一台节点（与 design/节点管理.md 一致）。
type RunNode struct {
	IP       string  `json:"ip"`
	Port     string  `json:"port,omitempty"`
	Username string  `json:"username"`
	Password string  `json:"password"`
	Labels   []Label `json:"labels,omitempty"`
}

// Label 标签键值。
type Label struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TemplateStep 对应 pipeline_name.template.json 中的单条步骤（与 design/pipeline.template.json 一致）。
type TemplateStep struct {
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	Entrypoint string   `json:"entrypoint,omitempty"`
	Args       []string `json:"args,omitempty"`
	Env        []string `json:"env,omitempty"`
	Nodes      []string `json:"nodes,omitempty"` // 后继节点名，用于 DAG 边
}

// PipelineRunData 写入 /var/lib/ar/pipeline_name/taskID/pipeline.json 的运行时状态（执行计划 DAG + 各节点状态）。
type PipelineRunData struct {
	TaskID       string              `json:"taskId"`
	PipelineName string              `json:"pipelineName"`
	Steps        []PipelineStepState `json:"steps"`
}

// PipelineStepState 单个步骤的执行状态。
type PipelineStepState struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"` // pending | running | success | failed | cancelled
	// 以下为渲染后的运行时参数（便于恢复/日志）
	Entrypoint string   `json:"entrypoint,omitempty"`
	Args       []string `json:"args,omitempty"`
	Env        []string `json:"env,omitempty"`
	Nodes      []string `json:"nodes,omitempty"`
}

// NodesFile 从 -n nodes.json 读取的节点列表（与 GraphQL RunPipelineInput 对应）。
type NodesFile struct {
	Nodes []RunNode `json:"nodes"`
}

// ParseNodesFile 解析节点列表 JSON，支持 { "nodes": [ ... ] } 或直接 [ ... ]。
func ParseNodesFile(data []byte) ([]RunNode, error) {
	var nodes []RunNode
	// 尝试包装格式
	var wrapped NodesFile
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Nodes) > 0 {
		return wrapped.Nodes, nil
	}
	// 尝试直接数组
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}
