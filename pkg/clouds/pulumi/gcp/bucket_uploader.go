package gcp

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	gcpStorage "cloud.google.com/go/storage"
	"go.uber.org/atomic"
	gcpOptions "google.golang.org/api/option"

	"github.com/MShekow/directory-checksum/directory_checksum"
	"github.com/pkg/errors"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/spf13/afero"

	"github.com/simple-container-com/api/pkg/api/logger/color"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type GcpBucketUploader struct {
	sdk.ResourceState
}

type BucketUploaderArgs struct {
	bucketName sdk.StringInput
	syncDir    string
	gcpCreds   string
	params     pApi.ProvisionParams
}

func NewGcpBucketUploader(ctx *sdk.Context, name string, args BucketUploaderArgs, opts ...sdk.ResourceOption) (*GcpBucketUploader, error) {
	resource := &GcpBucketUploader{}
	err := ctx.RegisterComponentResource("simple-container.com:module:GcpBucketUploader", name, resource, opts...)
	if err != nil {
		return nil, err
	}

	syncOutput := args.bucketName.ToStringOutput().ApplyT(func(bucketName string) (any, error) {
		var checksum string
		if dir, err := directory_checksum.ScanDirectory(args.syncDir, afero.NewOsFs()); err != nil {
			return nil, errors.Wrapf(err, "failed to scan directory %q", args.syncDir)
		} else if checksums, err := dir.ComputeDirectoryChecksums(); err != nil {
			return nil, errors.Wrapf(err, "failed to calculate directory checksums")
		} else {
			sum := md5.Sum([]byte(checksums))
			checksum = hex.EncodeToString(sum[:])
		}

		if ctx.DryRun() {
			return checksum, nil
		}
		_, err := copyAllFilesToBucket(ctx.Context(), bucketName, args.syncDir, args.gcpCreds, args.params)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to sync files to GCP bucket %q", args.bucketName)
		}
		return checksum, nil
	})

	// Complete the component resource creation
	err = ctx.RegisterResourceOutputs(resource, sdk.Map{
		"dirChecksum": syncOutput,
	})
	if err != nil {
		return nil, err
	}
	return resource, nil
}

func copyAllFilesToBucket(ctx context.Context, bucketName string, syncDir, gcpCreds string, params pApi.ProvisionParams) (int64, error) {
	client, err := gcpStorage.NewClient(ctx, gcpOptions.WithCredentialsJSON([]byte(gcpCreds)))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to initialize gcp client")
	}
	defer func(client *gcpStorage.Client) {
		_ = client.Close()
	}(client)
	bucketRef := client.Bucket(bucketName)
	totalBytes := atomic.NewInt64(0)
	params.Log.Info(ctx, "scanning directory %s...", syncDir)
	err = filepath.Walk(syncDir, func(filePath string, info fs.FileInfo, walkErr error) error {
		if info == nil || info.IsDir() {
			return nil
		}
		if walkErr != nil {
			params.Log.Error(ctx, color.RedFmt("failed to walk through path %q: %v", filePath, walkErr))
			return nil
		}
		copyPath, err := filepath.Rel(syncDir, filePath)
		if err != nil {
			return err
		}
		params.Log.Info(ctx, color.YellowFmt("uploading file %q to gs://%s/%s...", filePath, bucketName, copyPath))
		f, err := os.Open(path.Join(syncDir, copyPath))
		if err != nil {
			params.Log.Error(ctx, color.RedFmt("Error uploading %s: %v", filePath, err))
			return fmt.Errorf("os.Open: %v", err)
		}
		defer func(f *os.File) {
			_ = f.Close()
		}(f)
		object := bucketRef.Object(copyPath)
		if err := updateContentType(ctx, object, filePath); err != nil {
			return errors.Wrapf(err, "failed to update content type for %s", filePath)
		}
		wc := object.NewWriter(ctx)
		bytesCopied, err := io.Copy(wc, f)
		if err != nil {
			params.Log.Error(ctx, color.RedFmt("Error uploading %s: %v", filePath, err))
			return fmt.Errorf("io.Copy: %v", err)
		}
		totalBytes.Add(bytesCopied)
		if err := wc.Close(); err != nil {
			params.Log.Error(ctx, color.RedFmt("Error closing bucket object %s: %v", filePath, err))
			return fmt.Errorf("Writer.Close: %v", err)
		}
		params.Log.Info(ctx, color.GreenFmt("DONE gs://%s/%s (%d bytes)", bucketName, copyPath, bytesCopied))
		return nil
	})
	return totalBytes.Load(), err
}

func updateContentType(ctx context.Context, object *gcpStorage.ObjectHandle, filePath string) error {
	contentType := ""
	ext := filepath.Ext(strings.ToLower(filePath))
	switch ext {
	case ".css":
		contentType = "text/css"
	case ".js":
		contentType = "text/javascript"
	case ".png":
		contentType = "image/png"
	case ".apng":
		contentType = "image/apng"
	case ".avif":
		contentType = "image/avig"
	case ".gif":
		contentType = "image/gif"
	case ".jpeg":
	case ".jpg":
		contentType = "image/jpeg"
	case ".svg":
		contentType = "image/svg+xml"
	case ".webp":
		contentType = "image/webp"
	}
	if contentType == "" {
		if filebytes, err := os.ReadFile(filePath); err != nil {
			return errors.Wrapf(err, "failed to read file %s", filePath)
		} else {
			contentType = http.DetectContentType(filebytes)
		}
	}
	if _, err := object.Update(ctx, gcpStorage.ObjectAttrsToUpdate{
		ContentType: contentType,
	}); err != nil {
		return errors.Wrapf(err, "failed to update content type for %s", filePath)
	}
	return nil
}
