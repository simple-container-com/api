// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"github.com/simple-container-com/api/pkg/api"
)

// ResourceTypeObjectStorageBucket is deliberately NOT spelled `yc-s3-bucket`,
// even though Object Storage is the S3-compatible service and that name would
// read better. cmd/schema-gen's guessProviderFromResourceType matches the AWS
// substrings (`s3`, `aws`, `ecr`, ...) BEFORE it looks for `yc`/`yandex`, so any
// name carrying `s3` files the generated schema under docs/schemas/aws.
const ResourceTypeObjectStorageBucket = "yc-bucket"

// ObjectStorageBucket is a Yandex Object Storage bucket plus the identity a
// consuming stack uses to reach it.
//
// Unlike the AWS analogue there is no StaticSiteConfig here: YC bucket website
// hosting has a different certificate story (the `https` block on the bucket
// itself rather than CloudFront), and nothing in the fleet needs it yet. Adding
// it later is additive.
type ObjectStorageBucket struct {
	AccountConfig `json:",inline" yaml:",inline"`

	// Name overrides the descriptor name as the actual bucket name. YC bucket
	// names are global, like S3's.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// MaxSize caps the bucket in bytes. Zero means unlimited, which is YC's own
	// default.
	MaxSize int `json:"maxSize,omitempty" yaml:"maxSize,omitempty"`
	// ForceDestroy allows `sc destroy` to delete a non-empty bucket. Objects are
	// not recoverable, so it defaults off.
	ForceDestroy bool `json:"forceDestroy,omitempty" yaml:"forceDestroy,omitempty"`
	// Role is the folder-level IAM role granted to the bucket's service account.
	// `storage.editor` is read+write on every bucket in the folder; narrow it to
	// `storage.viewer` for a read-only consumer.
	Role string `json:"role,omitempty" yaml:"role,omitempty"`
}

// DefaultBucketRole is the folder-level role a bucket's own service account gets
// when the config does not name one.
const DefaultBucketRole = "storage.editor"

// EffectiveRole resolves Role, falling back to DefaultBucketRole.
func (b *ObjectStorageBucket) EffectiveRole() string {
	if b.Role == "" {
		return DefaultBucketRole
	}
	return b.Role
}

func ReadObjectStorageBucketConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &ObjectStorageBucket{})
}
