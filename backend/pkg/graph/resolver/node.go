package resolver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangxusc/ar/backend/pkg/graph/model"
)

const (
	nodesDir = "/var/lib/ar/nodes"
)

func nodeFilePath(ip string) string {
	return filepath.Join(nodesDir, fmt.Sprintf("node_%s.json", ip))
}
func loadAllNodes() (*model.NodeList, error) {
	if _, err := os.Stat(nodesDir); os.IsNotExist(err) {
		// 目录不存在时返回空列表
		return &model.NodeList{Nodes: []*model.Node{}}, nil
	}

	entries, err := os.ReadDir(nodesDir)
	if err != nil {
		return nil, err
	}

	nodes := make([]*model.Node, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "node_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(nodesDir, name))
		if err != nil {
			return nil, err
		}
		var n model.Node
		if err := json.Unmarshal(data, &n); err != nil {
			return nil, err
		}
		// 需要指针类型
		nCopy := n
		nodes = append(nodes, &nCopy)
	}
	return &model.NodeList{Nodes: nodes}, nil
}
func saveNode(n *model.Node) error {
	if err := os.MkdirAll(nodesDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(nodeFilePath(n.IP), data, 0o600)
}
