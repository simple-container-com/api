package aws

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws/s3"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/api/logger"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type S3BucketInput struct {
	Name       string
	Log        logger.Logger
	Provider   sdk.ProviderResource
	Registrar  pApi.Registrar
	StaticSite *api.StaticSiteConfig
	Stack      api.Stack
}

type PrivateBucketOutput struct {
	Bucket          *s3.Bucket
	AccessBlock     *s3.BucketPublicAccessBlock
	User            *iam.User
	AccessKey       *iam.AccessKey
	AccessKeySecret sdk.StringOutput
	BucketPolicy    *s3.BucketPolicy
	DomainRecord    sdk.AnyOutput
}

func S3Bucket(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.ResourceTypeS3Bucket {
		return nil, errors.Errorf("unsupported bucket type %q", input.Descriptor.Type)
	}

	bucketCfg, ok := input.Descriptor.Config.Config.(*aws.S3Bucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	bucketName := input.ToResName(lo.If(bucketCfg.Name == "", input.Descriptor.Name).Else(bucketCfg.Name))
	params.Log.Info(ctx.Context(), "configure private s3 bucket %q for %q in %q",
		bucketName, input.StackParams.StackName, input.StackParams.Environment)

	res, err := createS3Bucket(ctx, S3BucketInput{
		Name:       bucketName,
		Provider:   params.Provider,
		Registrar:  params.Registrar,
		Stack:      stack,
		Log:        params.Log,
		StaticSite: bucketCfg.StaticSiteConfig,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision private bucket")
	}

	return &api.ResourceOutput{Ref: res}, nil
}

func createS3Bucket(ctx *sdk.Context, input S3BucketInput) (*PrivateBucketOutput, error) {
	opts := []sdk.ResourceOption{
		sdk.Provider(input.Provider),
	}

	bucketArgs := &s3.BucketArgs{
		Bucket: sdk.String(input.Name),
	}

	staticSite := lo.FromPtr(input.StaticSite)

	if staticSite.Domain == "" {
		bucketArgs.Acl = sdk.String("private")
	} else if staticSite.Domain != "" {
		bucketArgs.Website = &s3.BucketWebsiteArgs{
			IndexDocument: sdk.String(lo.If(staticSite.IndexDocument != "", staticSite.IndexDocument).Else("index.html")),
			ErrorDocument: sdk.String(lo.If(staticSite.ErrorDocument != "", staticSite.ErrorDocument).Else("error.html")),
		}
	}

	bucket, err := s3.NewBucket(ctx, input.Name, bucketArgs, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision bucket %q", input.Name)
	}
	ctx.Export(toBucketNameExport(input.Name), bucket.Bucket)
	ctx.Export(toBucketRegionExport(input.Name), bucket.Region)

	// Apply the public access block configuration to the bucket
	pabArgs := &s3.BucketPublicAccessBlockArgs{
		Bucket:                bucket.ID(),    // References the created S3 bucket
		BlockPublicAcls:       sdk.Bool(true), // Blocks new public ACLs and uploading public objects.
		IgnorePublicAcls:      sdk.Bool(true), // Ignores all public ACLs on this bucket and objects within it.
		BlockPublicPolicy:     sdk.Bool(true), // Blocks new public bucket policies.
		RestrictPublicBuckets: sdk.Bool(true), // Restricts access to this bucket to only AWS services and authorized users within the AWS account.
	}
	if staticSite.Domain != "" {
		pabArgs.BlockPublicAcls = sdk.Bool(false)
		pabArgs.BlockPublicPolicy = sdk.Bool(false)
		pabArgs.RestrictPublicBuckets = sdk.Bool(false)
	}
	input.Log.Info(ctx.Context(), "configure public access block for s3 bucket...")
	accessBlock, err := s3.NewBucketPublicAccessBlock(ctx, fmt.Sprintf("%s-access-block", input.Name), pabArgs, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision access block for bucket %q", input.Name)
	}

	// Set ownership controls for the new S3 bucket
	input.Log.Info(ctx.Context(), "configure bucket ownership controls for s3 bucket...")
	ownershipControls, err := s3.NewBucketOwnershipControls(ctx, fmt.Sprintf("%s-ownership-controls", input.Name), &s3.BucketOwnershipControlsArgs{
		Bucket: bucket.ID(),
		Rule: &s3.BucketOwnershipControlsRuleArgs{
			ObjectOwnership: sdk.String("ObjectWriter"),
		},
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision ownership controls")
	}
	opts = append(opts, sdk.DependsOn([]sdk.Resource{ownershipControls}))

	input.Log.Info(ctx.Context(), "configure user having write access to s3 bucket...")
	user, err := iam.NewUser(ctx, fmt.Sprintf("%s-user", input.Name), &iam.UserArgs{
		ForceDestroy: sdk.BoolPtr(true),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision user for bucket %q", input.Name)
	}
	ctx.Export(toBucketUserExport(input.Name), user.Name)

	input.Log.Info(ctx.Context(), "configure access key for user having access to s3 bucket...")
	accessKey, err := iam.NewAccessKey(ctx, fmt.Sprintf("%s-access-key", input.Name), &iam.AccessKeyArgs{
		User: user.ID(),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision access key for bucket %q", input.Name)
	}
	ctx.Export(toBucketAccessKeySecretExport(input.Name), accessKey.Secret)
	ctx.Export(toBucketAccessKeyIdExport(input.Name), accessKey.ID())

	input.Log.Info(ctx.Context(), "configure s3 bucket policy...")
	bucketPolicy, err := s3.NewBucketPolicy(ctx, fmt.Sprintf("%s-policy", input.Name), &s3.BucketPolicyArgs{
		Bucket: bucket.ID(), // Reference to the bucket created above.
		Policy: sdk.All(user.Arn, bucket.Arn, bucket.ID()).ApplyT(func(args []interface{}) (sdk.StringOutput, error) {
			userArn := args[0].(string)
			bucketArn := args[1].(string)
			bucketID := args[2].(sdk.ID)
			var statement []map[string]any
			statement = append(statement, map[string]any{
				"Effect": "Allow",
				"Principal": map[string]any{
					"AWS": userArn,
				},
				"Action": "s3:*",
				"Resource": []string{
					fmt.Sprintf("arn:aws:s3:::%s", bucketID),
					fmt.Sprintf("arn:aws:s3:::%s/*", bucketID),
				},
			})
			if staticSite.Domain != "" {
				statement = append(statement, map[string]any{
					"Effect":    "Allow",
					"Principal": "*",
					"Action":    []string{"s3:GetObject"},
					"Resource":  []string{bucketArn + "/*"},
				})
			}
			policy := map[string]interface{}{
				"Version":   "2012-10-17",
				"Statement": statement,
			}
			policyJSON, err := json.Marshal(policy)
			if err != nil {
				return sdk.StringOutput{}, err
			}
			return sdk.String(policyJSON).ToStringOutput(), nil
		}).(sdk.StringOutput),
	}, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision bucket policy")
	}

	var domainRecord sdk.AnyOutput
	if staticSite.Domain != "" {
		domainRecord = bucket.BucketDomainName.ApplyT(func(endpoint string) (*api.ResourceOutput, error) {
			_, err = input.Registrar.NewOverrideHeaderRule(ctx, input.Stack, pApi.OverrideHeaderRule{
				Name:     input.Name,
				FromHost: staticSite.Domain,
				ToHost:   bucket.BucketDomainName,
			})
			if err != nil {
				return nil, errors.Wrapf(err, "failed to create override host rule from %q to %q", staticSite.Domain, endpoint)
			}
			return input.Registrar.NewRecord(ctx, api.DnsRecord{
				Name:     staticSite.Domain,
				Type:     "CNAME",
				ValueOut: bucket.BucketDomainName,
				Proxied:  true,
			})
		}).(sdk.AnyOutput)
	}

	return &PrivateBucketOutput{
		Bucket:          bucket,
		AccessBlock:     accessBlock,
		AccessKey:       accessKey,
		User:            user,
		AccessKeySecret: accessKey.Secret,
		BucketPolicy:    bucketPolicy,
		DomainRecord:    domainRecord,
	}, nil
}

func toBucketAccessKeySecretExport(bucketName string) string {
	return fmt.Sprintf("%s-access-key-secret", bucketName)
}

func toBucketAccessKeyIdExport(bucketName string) string {
	return fmt.Sprintf("%s-access-key-name", bucketName)
}

func toBucketRegionExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-region", bucketName)
}

func toBucketUserExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-user", bucketName)
}

func toBucketNameExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-name", bucketName)
}
