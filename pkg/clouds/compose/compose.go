package compose

import (
	"context"
	"path"

	"github.com/compose-spec/compose-go/loader"
	"github.com/compose-spec/compose-go/types"
)

type Config struct {
	Project *types.Project
}

func ReadDockerCompose(ctx context.Context, workingDir, composeFilePath string) (Config, error) {
	var res Config
	project, err := loader.LoadWithContext(ctx, types.ConfigDetails{
		WorkingDir: workingDir,
		ConfigFiles: []types.ConfigFile{{
			Filename: path.Join(workingDir, composeFilePath),
		}},
	}, func(options *loader.Options) {
		// todo: figure out options
		options.SkipNormalization = true
		options.SkipConsistencyCheck = true
	})
	if err != nil {
		return res, err
	}
	res.Project = project

	return res, err
}
