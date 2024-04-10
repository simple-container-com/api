package aws

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type PrivateBucketInput struct {
	Name     string
	Provider sdk.ProviderResource
}

type PrivateBucketOutput struct {
	Bucket          *s3.Bucket
	AccessBlock     *s3.BucketPublicAccessBlock
	User            *iam.User
	AccessKey       *iam.AccessKey
	AccessKeySecret sdk.StringOutput
	BucketPolicy    *s3.BucketPolicy
}

func ProvisionBucket(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.ResourceTypeS3Bucket {
		return nil, errors.Errorf("unsupported bucket type %q", input.Descriptor.Type)
	}

	bucketCfg, ok := input.Descriptor.Config.Config.(*aws.S3Bucket)
	if !ok {
		return nil, errors.Errorf("failed to convert bucket config for %q", input.Descriptor.Type)
	}

	res, err := createPrivateBucket(ctx, PrivateBucketInput{
		Name:     bucketCfg.Name,
		Provider: params.Provider,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision private bucket")
	}

	return &api.ResourceOutput{Ref: res}, nil
}

func createPrivateBucket(ctx *sdk.Context, input PrivateBucketInput) (*PrivateBucketOutput, error) {
	provider := sdk.Provider(input.Provider)

	bucket, err := s3.NewBucket(ctx, input.Name, &s3.BucketArgs{
		Bucket: sdk.String(input.Name),
		Acl:    sdk.String("private"),
	}, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision bucket %q", input.Name)
	}
	ctx.Export(toBucketNameExport(input.Name), bucket.Bucket)
	ctx.Export(toBucketRegionExport(input.Name), bucket.Region)

	// Apply the public access block configuration to the bucket
	accessBlock, err := s3.NewBucketPublicAccessBlock(ctx, fmt.Sprintf("%s-access-block", input.Name), &s3.BucketPublicAccessBlockArgs{
		Bucket:                bucket.ID(),    // References the created S3 bucket
		BlockPublicAcls:       sdk.Bool(true), // Blocks new public ACLs and uploading public objects.
		IgnorePublicAcls:      sdk.Bool(true), // Ignores all public ACLs on this bucket and objects within it.
		BlockPublicPolicy:     sdk.Bool(true), // Blocks new public bucket policies.
		RestrictPublicBuckets: sdk.Bool(true), // Restricts access to this bucket to only AWS services and authorized users within the AWS account.
	}, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision access block for bucket %q", input.Name)
	}

	user, err := iam.NewUser(ctx, fmt.Sprintf("%s-user", input.Name), &iam.UserArgs{
		ForceDestroy: sdk.BoolPtr(true),
	}, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision user for bucket %q", input.Name)
	}
	ctx.Export(toBucketUserExport(input.Name), user.Name)

	accessKey, err := iam.NewAccessKey(ctx, fmt.Sprintf("%s-access-key", input.Name), &iam.AccessKeyArgs{
		User: user.ID(),
	},
		provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision access key for bucket %q", input.Name)
	}
	ctx.Export(toBucketAccessKeySecretExport(input.Name), accessKey.Secret)
	ctx.Export(toBucketAccessKeyOutputExport(input.Name), accessKey.ToAccessKeyOutput())
	ctx.Export(toBucketAccessKeyIdExport(input.Name), accessKey.ID())

	// Define the S3 Bucket Policy.
	bucketPolicy, err := s3.NewBucketPolicy(ctx, fmt.Sprintf("%s-policy", input.Name), &s3.BucketPolicyArgs{
		Bucket: bucket.ID(), // Reference to the bucket created above.
		Policy: sdk.All(user.Arn, bucket.Arn, bucket.ID()).ApplyT(func(args []interface{}) (sdk.StringOutput, error) {
			userArn := args[0].(string)
			bucketID := args[2].(sdk.ID)
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []map[string]interface{}{
					{
						"Effect": "Allow",
						"Principal": map[string]interface{}{
							"AWS": userArn,
						},
						"Action": "s3:*",
						"Resource": []string{
							fmt.Sprintf("arn:aws:s3:::%s", bucketID),
							fmt.Sprintf("arn:aws:s3:::%s/*", bucketID),
						},
					},
				},
			}
			policyJSON, err := json.Marshal(policy)
			if err != nil {
				return sdk.StringOutput{}, err
			}
			return sdk.String(policyJSON).ToStringOutput(), nil
		}).(sdk.StringOutput),
	}, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision bucket policy")
	}

	return &PrivateBucketOutput{
		Bucket:          bucket,
		AccessBlock:     accessBlock,
		AccessKey:       accessKey,
		User:            user,
		AccessKeySecret: accessKey.Secret,
		BucketPolicy:    bucketPolicy,
	}, nil
}

func toBucketAccessKeySecretExport(bucketName string) string {
	return fmt.Sprintf("%s-access-key-secret", bucketName)
}

func toBucketAccessKeyOutputExport(bucketName string) string {
	return fmt.Sprintf("%s-access-key-output", bucketName)
}

func toBucketAccessKeyIdExport(bucketName string) string {
	return fmt.Sprintf("%s-access-key-name", bucketName)
}

func toBucketRegionExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-region", bucketName)
}

func toBucketUserExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-region", bucketName)
}

func toBucketNameExport(bucketName string) string {
	return fmt.Sprintf("%s-bucket-name", bucketName)
}
