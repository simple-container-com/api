package mongodb

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
)

// DropDatabaseHook is a pre-destroy hook that drops MongoDB databases before stack destruction.
// It reads the mongoDbDestroyDatabase flag from cloudExtras and, if enabled, drops all databases
// associated with service users found in the stack outputs.
func DropDatabaseHook(ctx context.Context, stack api.Stack, params api.DestroyParams, stackSource auto.Stack, log logger.Logger) {
	clientDesc := stack.Client.Stacks[params.Environment]
	if clientDesc.Config.Config == nil {
		return
	}

	type mongoDbDestroyExtras struct {
		MongoDBDestroyDatabase *bool `json:"mongoDbDestroyDatabase" yaml:"mongoDbDestroyDatabase"`
	}

	type stackConfigWithCloudExtras struct {
		CloudExtras *any `json:"cloudExtras" yaml:"cloudExtras"`
	}
	cfgWithExtras := &stackConfigWithCloudExtras{}
	if _, err := api.ConvertDescriptor(clientDesc.Config.Config, cfgWithExtras); err != nil || cfgWithExtras.CloudExtras == nil {
		return
	}

	extras := &mongoDbDestroyExtras{}
	converted, err := api.ConvertDescriptor(*cfgWithExtras.CloudExtras, extras)
	if err != nil || converted == nil || converted.MongoDBDestroyDatabase == nil || !*converted.MongoDBDestroyDatabase {
		return
	}

	outputs, err := stackSource.Outputs(ctx)
	if err != nil {
		log.Warn(ctx, "mongoDbDestroyDatabase: failed to get stack outputs: %v", err)
		return
	}

	for key, output := range outputs {
		if !strings.HasSuffix(key, "-service-user") {
			continue
		}
		dbUserJson, ok := output.Value.(string)
		if !ok {
			continue
		}
		var dbUser DbUserOutput
		if err := json.Unmarshal([]byte(dbUserJson), &dbUser); err != nil {
			log.Warn(ctx, "mongoDbDestroyDatabase: failed to parse service user output %q: %v", key, err)
			continue
		}
		// dbName == userName: both are set to stack.Name in appendUsesResourceContext
		dbName := dbUser.UserName
		fullUri := AppendUserPasswordAndDBToMongoUri(dbUser.DbUri, dbUser.UserName, dbUser.Password, dbName)
		log.Info(ctx, "mongoDbDestroyDatabase: dropping MongoDB database %q...", dbName)
		if err := DropDatabase(ctx, fullUri, dbName); err != nil {
			log.Warn(ctx, "mongoDbDestroyDatabase: failed to drop database %q: %v", dbName, err)
		} else {
			log.Info(ctx, "mongoDbDestroyDatabase: successfully dropped MongoDB database %q", dbName)
		}
	}
}
