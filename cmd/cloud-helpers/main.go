package main

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
	"github.com/spf13/cobra"
	"go.uber.org/atomic"
)

func main() {
	rootParams := &root_cmd.Params{
		Verbose:    false,
		Silent:     false,
		IsCanceled: atomic.NewBool(false),
		CancelFunc: func() {},
	}

	ctx, cancel := context.WithCancel(context.Background())

	rootParams.CancelFunc = cancel

	chType := os.Getenv(api.ScCloudHelperTypeEnvVariable)

	handlerCmd := &cobra.Command{
		Use:     "cloud-helpers",
		Version: build.Version,
		Short:   "Simple Container is a handy tool for provisioning your cloud clusters. Cloud Helpers allow to run certain operations within clouds.",
		Long:    "A fast and flexible way of deploying your whole infrastructure with the underlying use of Pulumi.\nComplete documentation is available at https://docs.simple-container.com",
		RunE: func(cmd *cobra.Command, args []string) error {
			ch, err := api.NewCloudHelper(chType)
			if err != nil {
				return errors.Wrapf(err, "failed to init cloud helper, did you pass %q env variable?", api.ScCloudHelperTypeEnvVariable)
			}
			return ch.Run()
		},
	}

	handlerCmd.SetContext(ctx)
	handlerCmd.SetVersionTemplate("{{printf \"%s\\n\" .Version}}")

	err := handlerCmd.Execute()
	if err != nil {
		panic(err)
	}
}
