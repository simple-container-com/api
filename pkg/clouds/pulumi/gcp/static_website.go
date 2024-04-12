package gcp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/simple-container-com/api/pkg/api/logger/color"

	gcpStorage "cloud.google.com/go/storage"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-gcp/sdk/v6/go/gcp/storage"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/atomic"
	gcpOptions "google.golang.org/api/option"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/gcloud"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

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
	Account            gcloud.Credentials
}

type StaticSiteOutput struct {
	Bucket             *storage.Bucket
	IamReadBinding     *storage.BucketIAMBinding
	DnsRecord          *api.ResourceOutput
	OverrideHeaderRule *api.ResourceOutput
	IamWriteBinding    *storage.BucketIAMBinding
}

func ProvisionStaticWebsite(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
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
	domain := in.Domain

	// Create a GCP storage bucket for the static website.
	bucket, err := storage.NewBucket(ctx, bucketName, &storage.BucketArgs{
		Name:         sdk.String(bucketName),
		Location:     sdk.String(in.Location),
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
	_, err = NewGcpBucketUploader(ctx, bucketName, BucketUploaderArgs{
		bucketName: bucket.Name,
		stackDir:   in.StackDir,
		relDir:     in.BundleDir,
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

	params.Log.Info(ctx.Context(), "creating override header rule from %q to %q for %q in %q...", domain, bucketDomain, stack.Name, input.StackParams.Environment)
	overrideHeaderRule, err := params.Registrar.NewOverrideHeaderRule(ctx, stack, pApi.OverrideHeaderRule{
		FromHost: domain,
		ToHost:   bucketDomain,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create override host rule from %q to %q", domain, bucketDomain)
	}
	ctx.Export(fmt.Sprintf("%s-override-header-rule-id", stack.Name), overrideHeaderRule.Ref.(sdk.Output))
	out.OverrideHeaderRule = overrideHeaderRule

	return &api.ResourceOutput{Ref: out}, nil
}

type GcpBucketUploader struct {
	sdk.ResourceState
}

type BucketUploaderArgs struct {
	bucketName sdk.StringInput
	stackDir   string
	relDir     string
	gcpCreds   string
	params     pApi.ProvisionParams
}

func NewGcpBucketUploader(ctx *sdk.Context, name string, args BucketUploaderArgs, opts ...sdk.ResourceOption) (*GcpBucketUploader, error) {
	resource := &GcpBucketUploader{}
	err := ctx.RegisterComponentResource("simple-container.com:module:GcpBucketUploader", name, resource, opts...)
	if err != nil {
		return nil, err
	}

	totals := args.bucketName.ToStringOutput().ApplyT(func(bucketName string) (any, error) {
		if ctx.DryRun() {
			return 0, nil
		}
		total, err := copyAllFilesToBucket(ctx.Context(), bucketName, args.stackDir, args.relDir, args.gcpCreds, args.params)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to sync files to GCP bucket %q", args.bucketName)
		}
		return total, nil
	})

	// Complete the component resource creation
	err = ctx.RegisterResourceOutputs(resource, sdk.Map{
		"totalBytesUploaded": totals,
	})
	if err != nil {
		return nil, err
	}
	return resource, nil
}

func copyAllFilesToBucket(ctx context.Context, bucketName string, stackDir, relDir, gcpCreds string, params pApi.ProvisionParams) (int64, error) {
	client, err := gcpStorage.NewClient(ctx, gcpOptions.WithCredentialsJSON([]byte(gcpCreds)))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to initialize gcp client")
	}
	defer func(client *gcpStorage.Client) {
		_ = client.Close()
	}(client)
	bucketRef := client.Bucket(bucketName)
	fullDirPath := path.Join(stackDir, relDir)
	totalBytes := atomic.NewInt64(0)
	params.Log.Info(ctx, "scanning directory %s...", fullDirPath)
	err = filepath.Walk(fullDirPath, func(filePath string, info fs.FileInfo, err error) error {
		if info == nil || info.IsDir() {
			return nil
		}
		copyPath, err := filepath.Rel(fullDirPath, filePath)
		if err != nil {
			return err
		}
		params.Log.Info(ctx, color.YellowFmt("uploading file %q to gs://%s/%s...", filePath, bucketName, copyPath))
		f, err := os.Open(path.Join(fullDirPath, copyPath))
		if err != nil {
			params.Log.Error(ctx, "Error uploading %s: %v", filePath, err)
			return fmt.Errorf("os.Open: %v", err)
		}
		defer func(f *os.File) {
			_ = f.Close()
		}(f)
		wc := bucketRef.Object(copyPath).NewWriter(ctx)
		bytesCopied, err := io.Copy(wc, f)
		if err != nil {
			params.Log.Error(ctx, "Error uploading %s: %v", filePath, err)
			return fmt.Errorf("io.Copy: %v", err)
		}
		totalBytes.Add(bytesCopied)
		if err := wc.Close(); err != nil {
			params.Log.Error(ctx, "Error closing bucket object %s: %v", filePath, err)
			return fmt.Errorf("Writer.Close: %v", err)
		}
		params.Log.Info(ctx, color.GreenFmt("DONE gs://%s/%s (%d bytes)", bucketName, copyPath, bytesCopied))
		return nil
	})
	return totalBytes.Load(), err
}
