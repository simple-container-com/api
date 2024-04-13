package gcp

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	gcpStorage "cloud.google.com/go/storage"
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/atomic"
	gcpOptions "google.golang.org/api/option"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type GcpBucketUploader struct {
	sdk.ResourceState
}

type BucketUploaderArgs struct {
	bucketName sdk.StringInput
	rootDir    string
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
		total, err := copyAllFilesToBucket(ctx.Context(), bucketName, args.rootDir, args.relDir, args.gcpCreds, args.params)
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

func copyAllFilesToBucket(ctx context.Context, bucketName string, rootDir, relDir, gcpCreds string, params pApi.ProvisionParams) (int64, error) {
	client, err := gcpStorage.NewClient(ctx, gcpOptions.WithCredentialsJSON([]byte(gcpCreds)))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to initialize gcp client")
	}
	defer func(client *gcpStorage.Client) {
		_ = client.Close()
	}(client)
	bucketRef := client.Bucket(bucketName)
	fullDirPath := path.Join(rootDir, relDir)
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
