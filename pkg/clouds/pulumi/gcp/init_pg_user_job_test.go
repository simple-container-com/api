// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcp

import (
	"sync"
	"testing"

	. "github.com/onsi/gomega"

	gcpsdk "github.com/pulumi/pulumi-gcp/sdk/v8/go/gcp"
	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type jobMocks struct {
	mu   sync.Mutex
	jobs []resource.PropertyMap
}

func (m *jobMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if args.TypeToken == "kubernetes:batch/v1:Job" {
		m.mu.Lock()
		m.jobs = append(m.jobs, args.Inputs)
		m.mu.Unlock()
	}
	return args.Name + "_id", args.Inputs, nil
}

func (m *jobMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func renderInitDbUserJobPodSpec(t *testing.T) resource.PropertyMap {
	t.Helper()
	mocks := &jobMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		gcpProv, err := gcpsdk.NewProvider(ctx, "gcp", &gcpsdk.ProviderArgs{Project: pulumi.String("p")})
		if err != nil {
			return err
		}
		kubeProv, err := sdkK8s.NewProvider(ctx, "k8s", &sdkK8s.ProviderArgs{})
		if err != nil {
			return err
		}
		db := PostgresDBInstanceArgs{Project: "p", InstanceName: "postgres--production", Region: "europe-north1"}
		proxy, err := NewCloudsqlProxy(ctx, CloudSQLProxyArgs{
			Name:         "app-initcsql",
			DBInstance:   db,
			GcpProvider:  gcpProv,
			KubeProvider: kubeProv,
			Metadata:     &metav1.ObjectMetaArgs{Namespace: pulumi.String("app"), Name: pulumi.String("app-initcsql")},
			PrivateIp:    true,
		})
		if err != nil {
			return err
		}
		_, err = NewInitDbUserJob(ctx, "app", InitDbUserJobArgs{
			Namespace:      pulumi.String("app"),
			User:           CloudsqlDbUser{Database: "app", Username: "app"},
			RootPassword:   "pw",
			DBInstance:     db,
			CloudSQLProxy:  proxy,
			KubeProvider:   kubeProv,
			DBInstanceType: PostgreSQL,
		})
		return err
	}, pulumi.WithMocks("project", "stack", mocks))
	Expect(err).ToNot(HaveOccurred())
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	Expect(mocks.jobs).To(HaveLen(1), "exactly one db-user-init Job")
	return mocks.jobs[0]["spec"].ObjectValue()["template"].ObjectValue()["spec"].ObjectValue()
}

// The Job's psql must not race the proxy: the proxy is a native sidecar whose startup
// probe holds the job container until it listens, and the kubelet stops it once psql
// exits. Fixed sleeps on either side failed whenever the proxy started late (cold
// Autopilot node) or the job container was slow (apk add).
func TestInitDbUserJob_ProxyIsANativeSidecar(t *testing.T) {
	RegisterTestingT(t)
	spec := renderInitDbUserJobPodSpec(t)

	Expect(spec["restartPolicy"].StringValue()).To(Equal("Never"))

	inits := spec["initContainers"].ArrayValue()
	Expect(inits).To(HaveLen(1))
	proxy := inits[0].ObjectValue()
	Expect(proxy["name"].StringValue()).To(Equal("cloudsql-proxy"))
	Expect(proxy["restartPolicy"].StringValue()).To(Equal("Always"))
	Expect(proxy).To(HaveKey(resource.PropertyKey("startupProbe")))
	Expect(proxy["command"].ArrayValue()[0].StringValue()).To(Equal("/cloud-sql-proxy"), "no self-killing shell wrapper")
	var args []string
	for _, a := range proxy["args"].ArrayValue() {
		args = append(args, a.StringValue())
	}
	Expect(args).To(ContainElements("--private-ip", "--health-check", "p:europe-north1:postgres--production"))

	containers := spec["containers"].ArrayValue()
	Expect(containers).To(HaveLen(1), "the proxy must not also run as a regular container")
	job := containers[0].ObjectValue()
	Expect(job["name"].StringValue()).To(Equal("job"))
	script := job["command"].ArrayValue()[2].StringValue()
	Expect(script).To(ContainSubstring(`GRANT pg_read_all_data TO "app"`))
	Expect(script).ToNot(ContainSubstring("sleep"), "psql waits on the startup probe, not a timer")
}
