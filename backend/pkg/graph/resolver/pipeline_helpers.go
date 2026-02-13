package resolver

import (
	"sync"

	"github.com/tangxusc/ar/backend/pkg/graph/model"
	"github.com/tangxusc/ar/backend/pkg/pipeline"
)

// runCancelRegistry 登记正在运行的流水线 taskID -> context.CancelFunc，供 StopPipeline 取消执行。
// 放在单独文件中，避免 gqlgen generate 覆盖 pipeline.resolvers.go 时被移除。
var runCancelRegistry sync.Map

// runPipelineNodesFromInput 将 GraphQL 入参转为 pipeline.RunNode 列表。
// 放在单独文件中，避免 gqlgen generate 覆盖 pipeline.resolvers.go 时被移除。
func runPipelineNodesFromInput(nodes []*model.RunPipelineNodeInput) []pipeline.RunNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]pipeline.RunNode, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		port := ""
		if n.Port != nil {
			port = *n.Port
		}
		labels := make([]pipeline.Label, 0, len(n.Labels))
		for _, l := range n.Labels {
			if l != nil {
				labels = append(labels, pipeline.Label{Key: l.Key, Value: l.Value})
			}
		}
		out = append(out, pipeline.RunNode{
			IP:       n.IP,
			Port:     port,
			Username: n.Username,
			Password: n.Password,
			Labels:   labels,
		})
	}
	return out
}
