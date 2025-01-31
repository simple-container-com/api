package aws

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
	"github.com/simple-container-com/api/pkg/util"
)

type DbUserOutput struct {
	Username string `json:"username" yaml:"username"`
	Database string `json:"database" yaml:"database"`
	Password string `json:"password" yaml:"password"`
	DbUri    string `json:"dbUri" yaml:"dbUri"`
}

func (o DbUserOutput) ToJson() string {
	res, _ := json.Marshal(o)
	return string(res)
}

func RdsPostgresComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	parentStackName := params.ParentStack.StackName

	postgresCfg, ok := input.Descriptor.Config.Config.(*aws.PostgresConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert postgres config for %q", input.Descriptor.Type)
	}
	accountConfig := &aws.AccountConfig{}
	err := api.ConvertAuth(&postgresCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}

	postgresCfg.AccountConfig = *accountConfig
	postgresResName := lo.If(postgresCfg.Name == "", input.Descriptor.Name).Else(postgresCfg.Name)
	postgresName := toRdsPostgresName(postgresResName, input.StackParams.Environment)

	// Create a StackReference to the parent stack
	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for rds postgres %q (%q)", params.ParentStack.FullReference, postgresName, suffix)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s%s--pg-ref", stack.Name, params.ParentStack.StackName, input.Descriptor.Name, suffix),
		&sdk.StackReferenceArgs{
			Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
		})
	if err != nil {
		return nil, err
	}
	postgresEndpointExport := toPostgresInstanceEndpointExport(postgresName)
	resPgEndpoint, err := pApi.GetParentOutput(parentRef, postgresEndpointExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres endpoint from parent stack for %q (%q)", stack.Name, postgresEndpointExport)
	} else if resPgEndpoint == "" {
		return nil, errors.Errorf("postgres endpoint is empty for %q (%q)", stack.Name, postgresName)
	}
	postgresUsernameExport := toPostgresInstanceUsernameExport(postgresName)
	rootPgUsername, err := pApi.GetParentOutput(parentRef, postgresUsernameExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres username from parent stack for %q (%q)", stack.Name, postgresEndpointExport)
	} else if rootPgUsername == "" {
		return nil, errors.Errorf("postgres username is empty for %q (%q)", stack.Name, postgresName)
	}
	postgresPasswordExport := toPostgresInstancePasswordExport(postgresName)
	rootPgPassword, err := pApi.GetParentOutput(parentRef, postgresPasswordExport, params.ParentStack.FullReference, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres password from parent stack for %q (%q)", stack.Name, postgresEndpointExport)
	} else if rootPgPassword == "" {
		return nil, errors.Errorf("postgres password is empty for %q (%q)", stack.Name, postgresName)
	}

	if !params.ParentStack.UsesResource {
		params.Log.Warn(ctx.Context(), "rds postgres %q only supports `uses`, but it wasn't explicitly declared as being used", postgresName)
		return nil, errors.Errorf("rds postgres %q only supports `uses`, but it wasn't explicitly declared as being used", postgresName)
	}

	collector.AddOutput(parentRef.Name.ApplyT(func(refName any) any {
		pgEpSplit := strings.SplitN(resPgEndpoint, ":", 2)
		dbHost, dbPort := pgEpSplit[0], pgEpSplit[1]
		dbUsername := stack.Name
		dbName := stack.Name
		password, err := random.NewRandomPassword(ctx, fmt.Sprintf("%s%s-pg-password", dbUsername, suffix), &random.RandomPasswordArgs{
			Length:  sdk.Int(20),
			Special: sdk.Bool(false),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to generate random password for postgres for stack %q", stack.Name)
		}

		return sdk.All(password.Result).ApplyT(func(args []any) (any, error) {
			dbPassword := args[0].(string)

			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("PGHOST_%s", postgresResName)), dbHost,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("PGUSER_%s", postgresResName)), dbUsername,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("PGPORT_%s", postgresResName)), dbPort,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("PGDATABASE_%s", postgresResName)), dbName,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("PGPASSWORD_%s", postgresResName)), dbPassword,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGUSER"), dbUsername,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGHOST"), dbHost,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGPORT"), dbPort,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("PGDATABASE"), dbName,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("PGPASSWORD"), dbPassword,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
				"url":      resPgEndpoint,
				"host":     dbHost,
				"port":     dbPort,
				"user":     dbUsername,
				"database": dbName,
				"password": dbPassword,
			})

			dbUserOutputJSON := DbUserOutput{
				Username: dbUsername,
				Database: dbName,
				Password: dbPassword,
				DbUri:    resPgEndpoint,
			}.ToJson()

			command := []string{
				// TODO: replace with db.PSQL_DB_INIT_SH
				"sh", "-c", `apk add --update postgresql && 
psql -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1 || psql -U postgres -c "CREATE DATABASE \"$DB_NAME\"" &&
psql -c "DO
\$\$
BEGIN
  IF NOT EXISTS (SELECT * FROM pg_user WHERE usename = '$DB_USER') THEN
	CREATE ROLE \"$DB_USER\" WITH LOGIN PASSWORD '$DB_PASSWORD'; 
    GRANT ALL PRIVILEGES ON DATABASE \"$DB_NAME\" TO \"$DB_USER\";
	ALTER DATABASE \"$DB_NAME\" OWNER TO \"$DB_USER\";
  END IF;
END
\$\$
;
"`,
			}

			if err := execEcsTask(ctx, ecsTaskConfig{
				name:    fmt.Sprintf("%s-pg-init", stack.Name),
				account: postgresCfg.AccountConfig,
				params:  params,
				image:   "alpine:latest",
				command: command,
				env: map[string]string{
					"DB_NAME":     dbName,
					"DB_USER":     dbUsername,
					"DB_PASSWORD": dbPassword,
					"PGHOST":      dbHost,
					"PGPORT":      dbPort,
					"PGUSER":      rootPgUsername,
					"PGDATABASE":  "postgres",
					"PGPASSWORD":  rootPgPassword,
				},
			}); err != nil {
				return nil, errors.Wrapf(err, "failed to run init task for rds postgres")
			}

			ctx.Export(fmt.Sprintf("%s-%s%s", dbUsername, postgresResName, suffix), sdk.ToSecret(dbUserOutputJSON))
			return dbUserOutputJSON, nil
		})
	}))

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}

func RdsMysqlComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	parentStackName := params.ParentStack.StackName

	mysqlCfg, ok := input.Descriptor.Config.Config.(*aws.MysqlConfig)
	if !ok {
		return nil, errors.Errorf("failed to convert mysql config for %q", input.Descriptor.Type)
	}
	accountConfig := &aws.AccountConfig{}
	err := api.ConvertAuth(&mysqlCfg.AccountConfig, accountConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert aws account config")
	}

	mysqlCfg.AccountConfig = *accountConfig
	dbCfg := mysqlCfg
	mysqlResName := lo.If(dbCfg.Name == "", input.Descriptor.Name).Else(dbCfg.Name)
	mysqlName := toRdsMysqlName(mysqlResName, input.StackParams.Environment)

	// Create a StackReference to the parent stack
	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for rds mysql %q (%q)", params.ParentStack.FullReference, mysqlName, suffix)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s%s--pg-ref", stack.Name, params.ParentStack.StackName, input.Descriptor.Name, suffix),
		&sdk.StackReferenceArgs{
			Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
		})
	if err != nil {
		return nil, err
	}
	mysqlEndpointExport := toMysqlInstanceEndpointExport(mysqlName)
	resMysqlEndpoint, err := pApi.GetParentOutput(parentRef, mysqlEndpointExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mysql endpoint from parent stack for %q (%q)", stack.Name, mysqlEndpointExport)
	} else if resMysqlEndpoint == "" {
		return nil, errors.Errorf("mysql endpoint is empty for %q (%q)", stack.Name, mysqlName)
	}
	mysqlUsernameExport := toPostgresInstanceUsernameExport(mysqlName)
	rootMysqlUsername, err := pApi.GetParentOutput(parentRef, mysqlUsernameExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mysql username from parent stack for %q (%q)", stack.Name, mysqlEndpointExport)
	} else if rootMysqlUsername == "" {
		return nil, errors.Errorf("mysql username is empty for %q (%q)", stack.Name, mysqlName)
	}
	mysqlPasswordExport := toMysqlInstancePasswordExport(mysqlName)
	rootMysqlPassword, err := pApi.GetParentOutput(parentRef, mysqlPasswordExport, params.ParentStack.FullReference, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mysql password from parent stack for %q (%q)", stack.Name, mysqlEndpointExport)
	} else if rootMysqlPassword == "" {
		return nil, errors.Errorf("mysql password is empty for %q (%q)", stack.Name, mysqlName)
	}

	if !params.ParentStack.UsesResource {
		params.Log.Warn(ctx.Context(), "rds mysql %q only supports `uses`, but it wasn't explicitly declared as being used", mysqlName)
		return nil, errors.Errorf("rds mysql %q only supports `uses`, but it wasn't explicitly declared as being used", mysqlName)
	}

	collector.AddOutput(parentRef.Name.ApplyT(func(refName any) any {
		mysqlEpSplit := strings.SplitN(resMysqlEndpoint, ":", 2)
		dbHost, dbPort := mysqlEpSplit[0], mysqlEpSplit[1]
		dbUsername := stack.Name
		dbName := stack.Name
		password, err := random.NewRandomPassword(ctx, fmt.Sprintf("%s%s-mysql-password", dbUsername, suffix), &random.RandomPasswordArgs{
			Length:  sdk.Int(20),
			Special: sdk.Bool(false),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to generate random password for mysql for stack %q", stack.Name)
		}

		return sdk.All(password.Result).ApplyT(func(args []any) (any, error) {
			dbPassword := args[0].(string)

			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MYSQL_HOST_%s", mysqlResName)), dbHost,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MYSQL_USER_%s", mysqlResName)), dbUsername,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MYSQL_PORT_%s", mysqlResName)), dbPort,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MYSQL_DB_%s", mysqlResName)), dbName,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("MYSQL_PASSWORD_%s", mysqlResName)), dbPassword,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MYSQL_USER"), dbUsername,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MYSQL_HOST"), dbHost,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MYSQL_PORT"), dbPort,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("MYSQL_DB"), dbName,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("MYSQL_PASSWORD"), dbPassword,
				input.Descriptor.Type, input.Descriptor.Name, parentStackName)
			collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
				"url":      resMysqlEndpoint,
				"host":     dbHost,
				"port":     dbPort,
				"user":     dbUsername,
				"database": dbName,
				"password": dbPassword,
			})

			dbUserOutputJSON := DbUserOutput{
				Username: dbUsername,
				Database: dbName,
				Password: dbPassword,
				DbUri:    resMysqlEndpoint,
			}.ToJson()

			command := []string{
				"sh", "-c", "apk add --update mysql-client ; " +
					"echo \"CREATE DATABASE IF NOT EXISTS \\`${DB_NAME}\\`; \" > /tmp/init.sql ; " +
					"echo \"CREATE USER IF NOT EXISTS \\`${DB_USER}\\`@'%' IDENTIFIED BY '${DB_PASSWORD}'; \" >> /tmp/init.sql ; " +
					"echo \"GRANT ALL ON \\`${DB_NAME}\\`.* TO \\`${DB_USER}\\`@'%'; \" >> /tmp/init.sql ; " +
					"mysql -u \"${MYSQL_USER}\" -h \"${MYSQL_HOST}\" -P \"${MYSQL_PORT}\" --password=\"${MYSQL_PASSWORD}\" < /tmp/init.sql",
			}

			if err := execEcsTask(ctx, ecsTaskConfig{
				name:    fmt.Sprintf("%s-mysql-init", stack.Name),
				account: dbCfg.AccountConfig,
				params:  params,
				image:   "alpine:latest",
				command: command,
				env: map[string]string{
					"DB_NAME":        dbName,
					"DB_USER":        dbUsername,
					"DB_PASSWORD":    dbPassword, // TODO: to secrets
					"MYSQL_HOST":     dbHost,
					"MYSQL_PORT":     dbPort,
					"MYSQL_USER":     rootMysqlUsername,
					"MYSQL_PASSWORD": rootMysqlPassword, // TODO: to secrets
				},
			}); err != nil {
				return nil, errors.Wrapf(err, "failed to run init task for rds mysql")
			}

			ctx.Export(fmt.Sprintf("%s-%s%s", dbUsername, mysqlResName, suffix), sdk.ToSecret(dbUserOutputJSON))
			return dbUserOutputJSON, nil
		})
	}))

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}

func S3BucketComputeProcessor(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, collector pApi.ComputeContextCollector, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if params.ParentStack == nil {
		return nil, errors.Errorf("parent stack must not be nil for compute processor for %q", stack.Name)
	}
	parentStackName := params.ParentStack.StackName

	bucketCfg, ok := input.Descriptor.Config.Config.(*aws.S3Bucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	bucketName := input.ToResName(lo.If(bucketCfg.Name == "", input.Descriptor.Name).Else(bucketCfg.Name))

	// Create a StackReference to the parent stack
	suffix := lo.If(params.ParentStack.DependsOnResource != nil, "--"+lo.FromPtr(params.ParentStack.DependsOnResource).Name).Else("")
	params.Log.Info(ctx.Context(), "getting parent's (%q) outputs for s3 bucket %q (%q)", params.ParentStack.FullReference, bucketName, suffix)
	parentRef, err := sdk.NewStackReference(ctx, fmt.Sprintf("%s--%s--%s%s--s3-bucket-ref", stack.Name, params.ParentStack.StackName, input.Descriptor.Name, suffix), &sdk.StackReferenceArgs{
		Name: sdk.String(params.ParentStack.FullReference).ToStringOutput(),
	})
	if err != nil {
		return nil, err
	}

	bucketNameExport := toBucketNameExport(bucketName)
	resBucketName, err := pApi.GetParentOutput(parentRef, bucketNameExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket name from parent stack for %q (%q)", stack.Name, bucketNameExport)
	} else if resBucketName == "" {
		return nil, errors.Errorf("bucket name is empty for %q (%q)", stack.Name, bucketNameExport)
	}
	secretKeyExport := toBucketAccessKeySecretExport(bucketName)
	resAccessKeySecret, err := pApi.GetParentOutput(parentRef, secretKeyExport, params.ParentStack.FullReference, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket access key secret from parent stack for %q (%q)", stack.Name, secretKeyExport)
	} else if resAccessKeySecret == "" {
		return nil, errors.Errorf("bucket access key secret is empty for %q (%q)", stack.Name, secretKeyExport)
	}
	keyIdExport := toBucketAccessKeyIdExport(bucketName)
	resAccessKeyId, err := pApi.GetParentOutput(parentRef, keyIdExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket access key secret from parent stack for %q (%q)", stack.Name, keyIdExport)
	} else if resAccessKeyId == "" {
		return nil, errors.Errorf("bucket access key id is empty for %q (%q)", stack.Name, keyIdExport)
	}
	regionExport := toBucketRegionExport(bucketName)
	resBucketRegion, err := pApi.GetParentOutput(parentRef, regionExport, params.ParentStack.FullReference, false)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get bucket region from parent stack for %q (%q)", stack.Name, regionExport)
	} else if resBucketRegion == "" {
		return nil, errors.Errorf("bucket region is empty for %q (%q)", stack.Name, regionExport)
	}

	collector.AddOutput(parentRef.Name.ApplyT(func(refName any) any {
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_REGION", bucketName)), resBucketRegion,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_BUCKET", bucketName)), resBucketName,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_ACCESS_KEY", bucketName)), resAccessKeyId,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName(fmt.Sprintf("S3_%s_SECRET_KEY", bucketName)), resAccessKeySecret,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("S3_REGION"), resBucketRegion,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddEnvVariableIfNotExist(util.ToEnvVariableName("S3_BUCKET"), resBucketName,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("S3_ACCESS_KEY"), resAccessKeyId,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)
		collector.AddSecretEnvVariableIfNotExist(util.ToEnvVariableName("S3_SECRET_KEY"), resAccessKeySecret,
			input.Descriptor.Type, input.Descriptor.Name, parentStackName)

		collector.AddResourceTplExtension(input.Descriptor.Name, map[string]string{
			"bucket":     resBucketName,
			"region":     resBucketRegion,
			"access-key": resAccessKeyId,
			"secret-key": resAccessKeySecret,
		})

		return nil
	}))

	return &api.ResourceOutput{
		Ref: parentStackName,
	}, nil
}
