// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package api

// ImageBuildDescriptor configures how images are built and pushed.
type ImageBuildDescriptor struct {
	// ReuseExistingCommitTag makes a deploy reuse the image already in the
	// registry when the version already names a commit and is already present,
	// instead of pushing a rebuild over it. Only versions of the form
	// YYYY.MM.DD-<abbreviated commit sha> qualify. Defaults to false.
	ReuseExistingCommitTag bool `json:"reuseExistingCommitTag,omitempty" yaml:"reuseExistingCommitTag,omitempty"`
}
