// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package scoped

import (
	"regexp"

	"github.com/pkg/errors"
)

var yamlQuotedValue = regexp.MustCompile("`[^`]*`")

// RedactYAMLError drops the values a YAML decoding error quotes ("cannot
// unmarshal !!str `ghp_abc...`"): they are the start of a secret value when the
// YAML is a decrypted secret or a file of them.
func RedactYAMLError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(yamlQuotedValue.ReplaceAllString(err.Error(), "<value>"))
}
