// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

// ImageBuildDescriptor configures how images are built and pushed.
type ImageBuildDescriptor struct {
	// ReuseExistingCommitTag makes a deploy reuse the image already in the
	// registry when the version is a commit-pinned tag that is already present,
	// instead of pushing a rebuild over it. Only commit-pinned tags qualify;
	// see IsCommitPinnedVersion. Defaults to false.
	ReuseExistingCommitTag bool `json:"reuseExistingCommitTag,omitempty" yaml:"reuseExistingCommitTag,omitempty"`
}

// ReuseExistingCommitTagEnabled reports whether tag reuse is configured.
func (c *ClientDescriptor) ReuseExistingCommitTagEnabled() bool {
	return c != nil && c.ImageBuild != nil && c.ImageBuild.ReuseExistingCommitTag
}
