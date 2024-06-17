package main

import (
	"context"
	"os"
	"time"

	"go.uber.org/atomic"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/cmd/root_cmd"
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

	chType := os.Getenv(api.ComputeEnv.CloudHelperType)

	handlerCmd := &cobra.Command{
		Use:     "cloud-helpers",
		Version: build.Version,
		Short:   "Simple Container is a handy tool for provisioning your cloud clusters. Cloud Helpers allow to run certain operations within clouds.",
		Long:    "A fast and flexible way of deploying your whole infrastructure with the underlying use of Pulumi.\nComplete documentation is available at https://docs.simple-container.com",
		RunE: func(cmd *cobra.Command, args []string) error {
			ch, err := api.NewCloudHelper(chType, api.WithLogger(logger.New()))
			if err != nil {
				return errors.Wrapf(err, "failed to init cloud helper, did you pass %q env variable?", api.ComputeEnv.CloudHelperType)
			}
			if os.Getenv("SIMPLE_CONTAINER_STARTUP_DELAY") != "" {
				delay, err := time.ParseDuration(os.Getenv("SIMPLE_CONTAINER_STARTUP_DELAY"))
				if err != nil {
					return errors.Wrapf(err, "failed to parse provided duration")
				}
				time.Sleep(delay)
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
