package gcp

import (
	"fmt"
	"path"

	"github.com/pkg/errors"

	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/storage"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type StaticSiteOutput struct {
	Bucket             *storage.Bucket
	IamReadBinding     *storage.BucketIAMBinding
	DnsRecord          *api.ResourceOutput
	OverrideHeaderRule *api.ResourceOutput
	IamWriteBinding    *storage.BucketIAMBinding
}

func StaticWebsite(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != gcloud.TemplateTypeStaticWebsite {
		return nil, errors.Errorf("unsupported bucket type %q", input.Descriptor.Type)
	}

	in, ok := input.Descriptor.Config.Config.(*gcloud.StaticSiteInput)
	if !ok {
		return nil, errors.Errorf("failed to convert static site input to *aws.StaticSiteInput for %q", stack.Name)
	}

	gcpCreds := in.CredentialsValue()

	out := &StaticSiteOutput{}

	bucketName := fmt.Sprintf("%s--%s", ctx.Project(), stack.Name)
	domain := in.Site.Domain

	if in.BucketName != "" {
		// override bucket name
		bucketName = in.BucketName
	}

	bucketLocation := in.Location
	if bucketLocation == "" {
		return nil, errors.Errorf("location is required for gcp bucket, but wasn't set for %q in %q", stack.Name, input.StackParams.Environment)
	}

	// Create a GCP storage bucket for the static website.
	bucket, err := storage.NewBucket(ctx, bucketName, &storage.BucketArgs{
		Name:         sdk.String(bucketName),
		Location:     sdk.String(bucketLocation),
		ForceDestroy: sdk.BoolPtr(true),
		Website: &storage.BucketWebsiteArgs{
			MainPageSuffix: sdk.String("index.html"),
			NotFoundPage:   sdk.String("404.html"),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create storage bucket for stack %q in %q", stack.Name, input.StackParams.Environment)
	}
	ctx.Export(fmt.Sprintf("%s-bucket-name", stack.Name), bucket.Name)
	ctx.Export(fmt.Sprintf("%s-url", stack.Name), bucket.Url)
	out.Bucket = bucket

	// Set the public access on the bucket.
	iamReadBinding, err := storage.NewBucketIAMBinding(ctx, fmt.Sprintf("%s-read-iam", bucketName), &storage.BucketIAMBindingArgs{
		Bucket: bucket.Name,
		Role:   sdk.String("roles/storage.objectViewer"),
		Members: sdk.StringArray{
			sdk.String("allUsers"),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create read iam binding for bucket %q for stack %q in %q", bucketName, stack.Name, input.StackParams.Environment)
	}
	ctx.Export(fmt.Sprintf("%s-iam-read-id", stack.Name), iamReadBinding.ID())
	out.IamReadBinding = iamReadBinding

	gcpCredsParsed, err := in.CredentialsParsed()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to detect client email from provided service account for stack %q", stack.Name)
	}
	accountType := "user"
	if gcpCredsParsed.Type == "service_account" {
		accountType = "serviceAccount"
	}
	iamWriteBinding, err := storage.NewBucketIAMBinding(ctx, fmt.Sprintf("%s-write-iam", bucketName), &storage.BucketIAMBindingArgs{
		Bucket: bucket.Name,
		Role:   sdk.String("roles/storage.objectAdmin"),
		Members: sdk.StringArray{
			sdk.String(fmt.Sprintf("%s:%s", accountType, gcpCredsParsed.ClientEmail)),
		},
	}, sdk.Provider(params.Provider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create write iam binding for bucket %q for stack %q in %q", bucketName, stack.Name, input.StackParams.Environment)
	}
	ctx.Export(fmt.Sprintf("%s-iam-write-id", stack.Name), iamWriteBinding.ID())
	out.IamWriteBinding = iamWriteBinding

	params.Log.Info(ctx.Context(), "copying all files from %q to gs://%s for %q in %q...", in.BundleDir, bucketName, stack.Name, input.StackParams.Environment)
	syncDir := in.BundleDir
	if !path.IsAbs(syncDir) {
		syncDir = path.Join(in.StackDir, in.BundleDir)
	}
	_, err = NewGcpBucketUploader(ctx, bucketName, BucketUploaderArgs{
		bucketName: bucket.Name,
		syncDir:    syncDir,
		gcpCreds:   gcpCreds,
		params:     params,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to sync bucket")
	}

	params.Log.Info(ctx.Context(), "configure CNAME DNS record %q for %q in %q...", bucketName, stack.Name, input.StackParams.Environment)
	bucketDomain := fmt.Sprintf("%s.storage.googleapis.com", bucketName)
	dnsRecord, err := params.Registrar.NewRecord(ctx, api.DnsRecord{
		Name:    domain,
		Value:   bucketDomain,
		Type:    "CNAME",
		Proxied: true,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create iam binding for bucket %q for stack %q in %q", bucketName, stack.Name, input.StackParams.Environment)
	}
	ctx.Export(fmt.Sprintf("%s-dns-record-id", stack.Name), dnsRecord.Ref.(sdk.Output))
	out.DnsRecord = dnsRecord

	params.Log.Info(ctx.Context(), "configure override header rule from %q to %q for %q in %q...", domain, bucketDomain, stack.Name, input.StackParams.Environment)
	overrideHeaderRule, err := params.Registrar.NewOverrideHeaderRule(ctx, stack, pApi.OverrideHeaderRule{
		FromHost: domain,
		ToHost:   sdk.String(bucketDomain),
		OverridePages: &pApi.OverridePagesRule{
			IndexPage:    in.Site.IndexDocument,
			NotFoundPage: in.Site.ErrorDocument,
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create override host rule from %q to %q", domain, bucketDomain)
	}
	ctx.Export(fmt.Sprintf("%s-override-header-rule-id", stack.Name), overrideHeaderRule.Ref.(sdk.Output))
	out.OverrideHeaderRule = overrideHeaderRule

	return &api.ResourceOutput{Ref: out}, nil
}
