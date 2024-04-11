package compose

import (
	"context"
	"path"
	"path/filepath"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

type Config struct {
	Project *types.Project
}

func ReadDockerCompose(ctx context.Context, workingDir, composeFilePath string) (Config, error) {
	var res Config
	if !filepath.IsAbs(composeFilePath) {
		composeFilePath = path.Join(workingDir, composeFilePath)
	} else {
		workingDir = path.Dir(composeFilePath)
	}
	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir: workingDir,
		ConfigFiles: []types.ConfigFile{{
			Filename: composeFilePath,
		}},
	}, func(options *loader.Options) {
		// todo: figure out options
		options.SkipNormalization = true
		options.SkipConsistencyCheck = true
		options.Interpolate.LookupValue = func(key string) (string, bool) {
			return "", false
		}
	})
	if err != nil {
		return res, err
	}
	res.Project = project

	return res, err
}
