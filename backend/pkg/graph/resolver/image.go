package resolver

import (
	"github.com/tangxusc/ar/backend/pkg/config"
	"github.com/tangxusc/ar/backend/pkg/graph/model"
	"github.com/tangxusc/ar/backend/pkg/pipeline"
)

func listImages() ([]*model.ImageEntry, error) {
	list, err := pipeline.ListImages(config.ImagesStoreDir)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ImageEntry, 0, len(list))
	for _, e := range list {
		out = append(out, &model.ImageEntry{
			Name: e.Name,
			Ref:  e.Ref,
			Path: e.Path,
		})
	}
	return out, nil
}
