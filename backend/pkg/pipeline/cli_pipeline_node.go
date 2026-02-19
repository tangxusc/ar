package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tangxusc/ar/backend/pkg/config"
)

const pipelineTemplateSuffixCLI = ".template.json"

// addPipelineCommand 在已有的 `pipeline` 命令下注册 list / rm 子命令。
func addPipelineCommand(pipelineCmd *cobra.Command) {

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "列出已存在的流水线模板",
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := listPipelineNames(config.PipelinesDir)
			if err != nil {
				return err
			}
			if len(names) == 0 {
				logrus.Info("当前无任何流水线模板")
				return nil
			}
			for _, name := range names {
				fmt.Println(name)
			}
			return nil
		},
	}
	pipelineCmd.AddCommand(listCmd)

	rmCmd := &cobra.Command{
		Use:     "rm [流水线名...]",
		Aliases: []string{"delete", "del"},
		Short:   "删除一个或多个流水线模板",
		Long:    "流水线名为模板前缀（不含 .template.json 后缀），可一次指定多个。例如: ar pipeline rm demo1 demo2",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请指定要删除的流水线名，例如: ar pipeline rm <name>")
			}
			for _, name := range args {
				if err := deletePipelineByName(config.PipelinesDir, name); err != nil {
					return err
				}
				logrus.Infof("已删除流水线模板: %s", name)
			}
			return nil
		},
	}
	pipelineCmd.AddCommand(rmCmd)
}

// listPipelineNames 返回 pipelinesDir 下所有有效流水线名称（去掉 .template.json 后缀，按字典序排序）。
func listPipelineNames(pipelinesDir string) ([]string, error) {
	if strings.TrimSpace(pipelinesDir) == "" {
		return nil, fmt.Errorf("pipelinesDir 不能为空")
	}
	if _, err := os.Stat(pipelinesDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(pipelinesDir)
	if err != nil {
		return nil, fmt.Errorf("读取流水线目录失败: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), pipelineTemplateSuffixCLI) {
			continue
		}
		name := strings.TrimSuffix(e.Name(), pipelineTemplateSuffixCLI)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func deletePipelineByName(pipelinesDir, name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("流水线名不能为空")
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return fmt.Errorf("非法的流水线名 %q", name)
	}

	fileName := trimmed
	if !strings.HasSuffix(fileName, pipelineTemplateSuffixCLI) {
		fileName += pipelineTemplateSuffixCLI
	}
	path := filepath.Join(pipelinesDir, fileName)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("流水线 %s 不存在", name)
		}
		return fmt.Errorf("访问流水线模板失败: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除流水线模板失败 %s: %w", path, err)
	}
	return nil
}

// ---- 节点相关 CLI ----

type cliNodeLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type cliNode struct {
	IP       string        `json:"ip"`
	Port     string        `json:"port"`
	Username string        `json:"username"`
	Password string        `json:"password"`
	Labels   []cliNodeLabel `json:"labels"`
}

// addNodeCommand 注册 `ar node` 相关子命令：list / rm。
func addNodeCommand(rootCommand *cobra.Command) {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "管理执行节点（列表、删除）",
	}
	rootCommand.AddCommand(nodeCmd)

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "列出已注册的执行节点",
		RunE: func(cmd *cobra.Command, args []string) error {
			nodes, err := loadAllCliNodes()
			if err != nil {
				return err
			}
			if len(nodes) == 0 {
				logrus.Info("当前无已注册节点")
				return nil
			}
			for _, n := range nodes {
				labelParts := make([]string, 0, len(n.Labels))
				for _, l := range n.Labels {
					labelParts = append(labelParts, fmt.Sprintf("%s=%s", l.Key, l.Value))
				}
				labels := strings.Join(labelParts, ",")
				fmt.Printf("%s\t%s\t%s\t%s\n", n.IP, n.Port, n.Username, labels)
			}
			return nil
		},
	}
	nodeCmd.AddCommand(listCmd)

	rmCmd := &cobra.Command{
		Use:     "rm [节点 IP...]",
		Aliases: []string{"delete", "del"},
		Short:   "删除一个或多个执行节点",
		Long:    "根据节点 IP 删除对应的节点文件 node_<ip>.json，可一次指定多个 IP。例如: ar node rm 10.0.0.1 10.0.0.2",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请指定要删除的节点 IP，例如: ar node rm <ip>")
			}
			for _, ip := range args {
				if err := deleteNodeByIP(ip); err != nil {
					return err
				}
				logrus.Infof("已删除节点: %s", ip)
			}
			return nil
		},
	}
	nodeCmd.AddCommand(rmCmd)
}

func loadAllCliNodes() ([]cliNode, error) {
	if _, err := os.Stat(config.NodesDir); os.IsNotExist(err) {
		return []cliNode{}, nil
	}

	entries, err := os.ReadDir(config.NodesDir)
	if err != nil {
		return nil, err
	}

	nodes := make([]cliNode, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "node_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(config.NodesDir, name))
		if err != nil {
			return nil, err
		}
		var n cliNode
		if err := json.Unmarshal(data, &n); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].IP < nodes[j].IP
	})
	return nodes, nil
}

func deleteNodeByIP(ip string) error {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return fmt.Errorf("节点 IP 不能为空")
	}
	filename := fmt.Sprintf("node_%s.json", trimmed)
	path := filepath.Join(config.NodesDir, filename)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("节点 %s 不存在", ip)
		}
		return fmt.Errorf("访问节点文件失败: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除节点文件失败 %s: %w", path, err)
	}
	return nil
}

