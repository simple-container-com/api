// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package cmd_cloudru

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestReadRequest(t *testing.T) {
	RegisterTestingT(t)

	req, err := readRequest(strings.NewReader(`{
		"keyId": "key-id",
		"keySecret": "key-secret",
		"projectId": "project-id",
		"name": "atrium--staging",
		"spec": {"name": "atrium--staging", "template": {"containers": [{"name": "atrium", "image": "r.cr.cloud.ru/a:1"}]}}
	}`))
	Expect(err).To(BeNil())
	Expect(req.KeyID).To(Equal("key-id"))
	Expect(req.ProjectID).To(Equal("project-id"))
	Expect(req.Name).To(Equal("atrium--staging"))
	Expect(req.Spec.Template.Containers).To(HaveLen(1))
}

// An unknown field means this `sc` is older than the module that generated the
// request, and the half of the spec it does not know would be silently dropped
// behind a green deploy.
func TestReadRequestRejectsUnknownField(t *testing.T) {
	RegisterTestingT(t)

	_, err := readRequest(strings.NewReader(`{"keyId":"k","keySecret":"s","projectId":"p","fieldFromTheFuture":true}`))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("new enough"))
}

// The decode error must not quote the input: it carries the IAM key secret and
// every secret the stack declares.
func TestReadRequestErrorDoesNotEchoInput(t *testing.T) {
	RegisterTestingT(t)

	_, err := readRequest(strings.NewReader(`{"keySecret":"SUPER-SECRET-VALUE",`))
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).NotTo(ContainSubstring("SUPER-SECRET-VALUE"))
}

// Both subcommand groups have to be reachable, and the names have to match what
// scScript renders in pkg/clouds/pulumi/cloudru — a rename there compiles fine and
// fails as `unknown command` mid-deploy.
func TestCommandTreeShape(t *testing.T) {
	RegisterTestingT(t)

	root := NewCloudruCmd()
	Expect(root.Use).To(Equal("cloudru"))

	byName := map[string][]string{}
	for _, c := range root.Commands() {
		for _, sub := range c.Commands() {
			byName[c.Name()] = append(byName[c.Name()], sub.Name())
		}
	}
	Expect(byName).To(HaveKeyWithValue("registry", ConsistOf("ensure")))
	Expect(byName).To(HaveKeyWithValue("container-app", ConsistOf("apply", "destroy")))
}
