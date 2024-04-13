package aws

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"

	"github.com/MShekow/directory-checksum/directory_checksum"
	"github.com/pkg/errors"
	awsImpl "github.com/pulumi/pulumi-aws/sdk/v6/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v6/go/aws/s3"
	"github.com/pulumi/pulumi-command/sdk/go/command/local"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/spf13/afero"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/aws"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

func StaticWebsite(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != aws.TemplateTypeStaticWebsite {
		return nil, errors.Errorf("unsupported bucket type %q", input.Descriptor.Type)
	}

	staticSiteCfg, ok := input.Descriptor.Config.Config.(*aws.StaticSiteInput)
	if !ok {
		return nil, errors.Errorf("failed to convert static site input to *aws.StaticSiteInput for %q", stack.Name)
	}

	ref, err := provisionStaticSite(&StaticSiteInput{
		ServiceName:        stack.Name,
		Provider:           params.Provider,
		Registrar:          params.Registrar,
		Ctx:                ctx,
		BundleDir:          path.Join(staticSiteCfg.StackDir, staticSiteCfg.BundleDir),
		IndexDocument:      staticSiteCfg.IndexDocument,
		ErrorDocument:      staticSiteCfg.ErrorDocument,
		Domain:             staticSiteCfg.Domain,
		ProvisionWwwDomain: staticSiteCfg.ProvisionWwwDomain,
		Account:            staticSiteCfg.AccountConfig,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision static website for stack %q", stack.Name)
	}

	return &api.ResourceOutput{Ref: ref}, nil
}

type StaticSiteInput struct {
	ServiceName        string
	Provider           sdk.ProviderResource
	Registrar          pApi.Registrar
	Ctx                *sdk.Context
	BundleDir          string
	IndexDocument      string
	ErrorDocument      string
	Domain             string
	ProvisionWwwDomain bool
	Account            aws.AccountConfig
}

type StaticSiteOutput struct {
	MainBucket                  *s3.Bucket
	MainBucketPublicAccessBlock *s3.BucketPublicAccessBlock
	MainBucketOwnershipControls *s3.BucketOwnershipControls
	MainBucketPolicy            *s3.BucketPolicy
	MainRecord                  sdk.AnyOutput
	WwwBucket                   *s3.Bucket
	WwwRecord                   sdk.AnyOutput
}

func provisionStaticSite(input *StaticSiteInput) (*StaticSiteOutput, error) {
	ctx := input.Ctx

	provider := sdk.Provider(input.Provider)

	// Create an S3 bucket and configure it as a website.
	bucketName := input.Domain
	mainBucket, err := s3.NewBucket(ctx, bucketName, &s3.BucketArgs{
		Bucket:       sdk.String(bucketName),
		ForceDestroy: sdk.Bool(true),
		Website: &s3.BucketWebsiteArgs{
			IndexDocument: sdk.String(input.IndexDocument),
			ErrorDocument: sdk.String(input.ErrorDocument),
		},
	}, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision s3 bucket")
	}

	// Configure public access block for the new S3 bucket
	publicAccessBlock, err := s3.NewBucketPublicAccessBlock(ctx, fmt.Sprintf("%s-public-access-block", input.ServiceName), &s3.BucketPublicAccessBlockArgs{
		Bucket:                mainBucket.Bucket,
		BlockPublicAcls:       sdk.Bool(false),
		IgnorePublicAcls:      sdk.Bool(true),
		BlockPublicPolicy:     sdk.Bool(false),
		RestrictPublicBuckets: sdk.Bool(false),
	}, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision bucket public access block")
	}

	// Set ownership controls for the new S3 bucket
	ownershipControls, err := s3.NewBucketOwnershipControls(ctx, fmt.Sprintf("%s-ownership-controls", input.ServiceName), &s3.BucketOwnershipControlsArgs{
		Bucket: mainBucket.Bucket,
		Rule: &s3.BucketOwnershipControlsRuleArgs{
			ObjectOwnership: sdk.String("ObjectWriter"),
		},
	}, provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision ownership controls")
	}

	awsIdentity, err := awsImpl.GetCallerIdentity(ctx, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get caller identity")
	}
	// Define the S3 Bucket Policy.
	mainBucketPolicy, err := s3.NewBucketPolicy(ctx, fmt.Sprintf("%s-policy", input.ServiceName), &s3.BucketPolicyArgs{
		Bucket: mainBucket.ID(), // Reference to the bucket created above.
		Policy: sdk.All(mainBucket.Arn, mainBucket.ID()).ApplyT(func(args []interface{}) (sdk.StringOutput, error) {
			arn := args[0].(string)
			bucketID := args[1].(sdk.ID)
			policy := map[string]interface{}{
				"Version": "2012-10-17",
				"Statement": []map[string]interface{}{
					{
						"Effect":    "Allow",
						"Principal": "*",
						"Action":    []string{"s3:GetObject"},
						"Resource":  []string{arn + "/*"},
					},
					{
						"Effect": "Allow",
						"Principal": map[string]interface{}{
							"AWS": awsIdentity.Arn,
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

	if dir, err := directory_checksum.ScanDirectory(input.BundleDir, afero.NewOsFs()); err != nil {
		return nil, errors.Wrapf(err, "failed to scan directory %q", input.BundleDir)
	} else if checksums, err := dir.ComputeDirectoryChecksums(); err != nil {
		return nil, errors.Wrapf(err, "failed to calculate directory checksums")
	} else {
		sum := md5.Sum([]byte(checksums))
		checksum := hex.EncodeToString(sum[:])
		sdk.All(mainBucket.Bucket).ApplyT(func(a []interface{}) error {
			bucketName := a[0].(string)
			_, err = local.NewCommand(ctx, fmt.Sprintf("%s-sync", input.ServiceName), &local.CommandArgs{
				Create:   sdk.String(fmt.Sprintf("aws s3 sync %s s3://%s", input.BundleDir, bucketName)),
				Update:   sdk.String(fmt.Sprintf("aws s3 sync %s s3://%s", input.BundleDir, bucketName)),
				Triggers: sdk.ArrayInput(sdk.Array{sdk.String(checksum)}),
				Environment: sdk.ToStringMap(map[string]string{
					"AWS_ACCESS_KEY_ID":     input.Account.AccessKey,
					"AWS_SECRET_ACCESS_KEY": input.Account.SecretAccessKey,
					"AWS_DEFAULT_REGION":    input.Account.Region,
				}),
			}, sdk.DependsOn([]sdk.Resource{mainBucket, publicAccessBlock, ownershipControls, mainBucketPolicy}))
			if err != nil {
				return err
			}
			return nil
		})
	}

	mainRecord := mainBucket.WebsiteEndpoint.ApplyT(func(endpoint string) (*api.ResourceOutput, error) {
		return input.Registrar.NewRecord(ctx, api.DnsRecord{
			Name:    input.Domain,
			Type:    "CNAME",
			Value:   endpoint,
			Proxied: true,
		})
	}).(sdk.AnyOutput)

	var wwwRecord sdk.AnyOutput
	var wwwBucket *s3.Bucket
	if input.ProvisionWwwDomain {
		// Configure S3 bucket to redirect requests for www.mydomain.com to mydomain.com
		wwwDomain := fmt.Sprintf("www.%s", input.Domain)
		wwwBucket, err = s3.NewBucket(ctx, fmt.Sprintf("%s-www-redirect", input.ServiceName), &s3.BucketArgs{
			Bucket:       sdk.String(wwwDomain),
			ForceDestroy: sdk.Bool(true),
			Website: s3.BucketWebsiteArgs{
				RedirectAllRequestsTo: sdk.StringPtr(input.Domain),
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision www bucket")
		}
		wwwRecord = wwwBucket.WebsiteEndpoint.ApplyT(func(endpoint string) (*api.ResourceOutput, error) {
			return input.Registrar.NewRecord(ctx, api.DnsRecord{
				Name:    wwwDomain,
				Type:    "CNAME",
				Value:   endpoint,
				Proxied: true,
			})
		}).(sdk.AnyOutput)
	}

	ctx.Export(fmt.Sprintf("%s-regionalDomainName", input.ServiceName), mainBucket.BucketRegionalDomainName)
	ctx.Export(fmt.Sprintf("%s-originHostname", input.ServiceName), mainBucket.WebsiteEndpoint)
	ctx.Export(fmt.Sprintf("%s-websiteURL", input.ServiceName), sdk.Sprintf("https://%s", bucketName))

	return &StaticSiteOutput{
		MainBucket:                  mainBucket,
		MainBucketPublicAccessBlock: publicAccessBlock,
		MainBucketOwnershipControls: ownershipControls,
		MainBucketPolicy:            mainBucketPolicy,
		MainRecord:                  mainRecord,
		WwwBucket:                   wwwBucket,
		WwwRecord:                   wwwRecord,
	}, nil
}
