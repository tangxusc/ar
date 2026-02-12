package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tangxusc/ar/backend/pkg/config"
	"github.com/tangxusc/ar/backend/pkg/graph/model"
)

const pipelineTemplateSuffix = ".template.json"

func loadAllPipelines() ([]*model.Pipeline, error) {
	if _, err := os.Stat(config.PipelinesDir); os.IsNotExist(err) {
		// 与 node 列表行为保持一致：目录不存在时返回空列表。
		return []*model.Pipeline{}, nil
	}

	entries, err := os.ReadDir(config.PipelinesDir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isPipelineTemplateFile(entry.Name()) {
			continue
		}

		pipelineName := pipelineNameFromTemplate(entry.Name())
		if pipelineName == "" {
			continue
		}
		names = append(names, pipelineName)
	}
	sort.Strings(names)

	pipelines := make([]*model.Pipeline, 0, len(names))
	for _, name := range names {
		pipeline, err := loadPipelineByName(name)
		if err != nil {
			return nil, err
		}
		pipelines = append(pipelines, pipeline)
	}
	return pipelines, nil
}

func loadPipelineByName(name string) (*model.Pipeline, error) {
	templatePath, err := pipelineTemplatePath(name)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	pipelineName := pipelineNameFromTemplate(filepath.Base(templatePath))
	if pipelineName == "" {
		return nil, fmt.Errorf("invalid pipeline template file: %s", templatePath)
	}

	return &model.Pipeline{
		Name: pipelineName,
		Dag:  string(data),
	}, nil
}

func pipelineTemplatePath(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("pipeline name is required")
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return "", fmt.Errorf("invalid pipeline name %q", name)
	}

	fileName := trimmed
	if !isPipelineTemplateFile(fileName) {
		fileName += pipelineTemplateSuffix
	}
	templatePath := filepath.Join(config.PipelinesDir, fileName)

	if _, err := os.Stat(templatePath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("pipeline %s not found", name)
		}
		return "", err
	}

	return templatePath, nil
}

func isPipelineTemplateFile(fileName string) bool {
	return strings.HasSuffix(fileName, pipelineTemplateSuffix)
}

func pipelineNameFromTemplate(fileName string) string {
	if !isPipelineTemplateFile(fileName) {
		return ""
	}
	return strings.TrimSuffix(fileName, pipelineTemplateSuffix)
}
