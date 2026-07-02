// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package gcloud

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

// ---- ArtifactRegistry ----------------------------------------------------

func TestArtifactRegistryConfigReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps location/public/docker/basicAuth", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId": "my-gcp-project",
			"location":  "europe-west1",
			"public":    true,
			"domain":    "registry.example.com",
			"docker": map[string]any{
				"immutableTags": true,
			},
			"basicAuth": map[string]any{
				"username": "user",
				"password": "pass",
			},
		}}
		out, err := ArtifactRegistryConfigReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		ar, ok := out.Config.(*ArtifactRegistryConfig)
		Expect(ok).To(BeTrue())
		Expect(ar.Location).To(Equal("europe-west1"))
		Expect(ar.Public).ToNot(BeNil())
		Expect(*ar.Public).To(BeTrue())
		Expect(ar.Domain).ToNot(BeNil())
		Expect(*ar.Domain).To(Equal("registry.example.com"))
		Expect(ar.Docker).ToNot(BeNil())
		Expect(ar.Docker.ImmutableTags).ToNot(BeNil())
		Expect(*ar.Docker.ImmutableTags).To(BeTrue())
		Expect(ar.BasicAuth).ToNot(BeNil())
		Expect(ar.BasicAuth.Username).To(Equal("user"))
		Expect(ar.BasicAuth.Password).To(Equal("pass"))
	})

	t.Run("optional pointers stay nil when absent", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"location": "us-central1",
		}}
		out, err := ArtifactRegistryConfigReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		ar := out.Config.(*ArtifactRegistryConfig)
		Expect(ar.Public).To(BeNil())
		Expect(ar.Docker).To(BeNil())
		Expect(ar.BasicAuth).To(BeNil())
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"location": []int{1, 2},
		}}
		_, err := ArtifactRegistryConfigReadConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

func TestDockerRemoteImagePushReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":                "my-gcp-project",
			"remoteImage":              "docker.io/library/nginx:latest",
			"name":                     "nginx",
			"tag":                      "v1",
			"artifactRegistryResource": "my-registry",
		}}
		out, err := DockerRemoteImagePushReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		rip, ok := out.Config.(*RemoteImagePush)
		Expect(ok).To(BeTrue())
		Expect(rip.RemoteImage).To(Equal("docker.io/library/nginx:latest"))
		Expect(rip.Name).To(Equal("nginx"))
		Expect(rip.Tag).To(Equal("v1"))
		Expect(rip.ArtifactRegistryResource).To(Equal("my-registry"))
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"name": map[string]any{"bad": "type"},
		}}
		_, err := DockerRemoteImagePushReadConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

func TestRemoteImagePush_DependsOnResources(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name     string
		resource string
	}{
		{name: "named registry resource", resource: "my-registry"},
		{name: "empty resource", resource: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			rip := &RemoteImagePush{ArtifactRegistryResource: tc.resource}
			deps := rip.DependsOnResources()
			Expect(deps).To(HaveLen(1))
			Expect(deps[0].Name).To(Equal(tc.resource))
		})
	}
}

// ---- Bucket --------------------------------------------------------------

func TestGcpBucket_GetBucketName(t *testing.T) {
	RegisterTestingT(t)
	tests := []struct {
		name       string
		bucketName string
		nameField  string
		want       string
	}{
		{name: "bucketName takes precedence", bucketName: "bucket-a", nameField: "name-b", want: "bucket-a"},
		{name: "falls back to name", bucketName: "", nameField: "name-b", want: "name-b"},
		{name: "both empty", bucketName: "", nameField: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			b := &GcpBucket{BucketName: tc.bucketName, Name: tc.nameField}
			Expect(b.GetBucketName()).To(Equal(tc.want))
		})
	}
}

func TestGcpBucketReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":  "my-gcp-project",
			"name":       "my-bucket",
			"location":   "EU",
			"adopt":      true,
			"bucketName": "explicit-bucket",
		}}
		out, err := GcpBucketReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		b, ok := out.Config.(*GcpBucket)
		Expect(ok).To(BeTrue())
		Expect(b.Name).To(Equal("my-bucket"))
		Expect(b.Location).To(Equal("EU"))
		Expect(b.Adopt).To(BeTrue())
		Expect(b.BucketName).To(Equal("explicit-bucket"))
		Expect(b.GetBucketName()).To(Equal("explicit-bucket"))
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"adopt": "not-a-bool",
		}}
		_, err := GcpBucketReadConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

// ---- Postgres ------------------------------------------------------------

func TestPostgresqlGcpCloudsqlReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps scalars and pointers", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":          "my-gcp-project",
			"version":            "POSTGRES_16",
			"project":            "data-project",
			"tier":               "db-custom-2-7680",
			"region":             "europe-west1",
			"maxConnections":     200,
			"deletionProtection": true,
			"availabilityType":   "REGIONAL",
			"requireSsl":         true,
			"databaseFlags": map[string]any{
				"cloudsql.iam_authentication": "on",
				// Unquoted YAML int — users write flag values bare;
				// yaml.v3 coerces scalars into the string map.
				"log_min_duration_statement": 500,
			},
			"usersProvisionRuntime": map[string]any{
				"type":         "kube-job",
				"resourceName": "gke-cluster",
			},
		}}
		out, err := PostgresqlGcpCloudsqlReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		pg, ok := out.Config.(*PostgresGcpCloudsqlConfig)
		Expect(ok).To(BeTrue())
		Expect(pg.Version).To(Equal("POSTGRES_16"))
		Expect(pg.Project).To(Equal("data-project"))
		Expect(pg.Tier).ToNot(BeNil())
		Expect(*pg.Tier).To(Equal("db-custom-2-7680"))
		Expect(pg.Region).ToNot(BeNil())
		Expect(*pg.Region).To(Equal("europe-west1"))
		Expect(pg.MaxConnections).ToNot(BeNil())
		Expect(*pg.MaxConnections).To(Equal(200))
		Expect(pg.DatabaseFlags).To(Equal(map[string]string{
			"cloudsql.iam_authentication": "on",
			"log_min_duration_statement":  "500",
		}))
		Expect(pg.DeletionProtection).ToNot(BeNil())
		Expect(*pg.DeletionProtection).To(BeTrue())
		Expect(pg.AvailabilityType).ToNot(BeNil())
		Expect(*pg.AvailabilityType).To(Equal("REGIONAL"))
		Expect(pg.RequireSsl).ToNot(BeNil())
		Expect(*pg.RequireSsl).To(BeTrue())
		Expect(pg.UsersProvisionRuntime).ToNot(BeNil())
		Expect(pg.UsersProvisionRuntime.Type).To(Equal("kube-job"))
		Expect(pg.UsersProvisionRuntime.ResourceName).To(Equal("gke-cluster"))
	})

	t.Run("optional pointers stay nil when absent", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"version": "POSTGRES_15",
		}}
		out, err := PostgresqlGcpCloudsqlReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		pg := out.Config.(*PostgresGcpCloudsqlConfig)
		Expect(pg.Tier).To(BeNil())
		Expect(pg.Region).To(BeNil())
		Expect(pg.MaxConnections).To(BeNil())
		Expect(pg.DatabaseFlags).To(BeNil())
		Expect(pg.UsersProvisionRuntime).To(BeNil())
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"maxConnections": "not-an-int",
		}}
		_, err := PostgresqlGcpCloudsqlReadConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

// ---- PubSub --------------------------------------------------------------

func TestGcpPubSubTopicsReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps topics, subscriptions and labels", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId": "my-gcp-project",
			"labels": map[string]any{
				"env": "prod",
			},
			"topics": []any{
				map[string]any{
					"name":                     "events",
					"messageRetentionDuration": "604800s",
					"labels":                   map[string]any{"team": "core"},
				},
			},
			"subscriptions": []any{
				map[string]any{
					"name":                "events-sub",
					"topic":               "events",
					"exactlyOnceDelivery": true,
					"ackDeadlineSec":      30,
					"deadLetterPolicy": map[string]any{
						"deadLetterTopic":     "events-dlq",
						"maxDeliveryAttempts": 5,
					},
				},
			},
		}}
		out, err := GcpPubSubTopicsReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		ps, ok := out.Config.(*PubSubConfig)
		Expect(ok).To(BeTrue())
		Expect(ps.Labels).To(HaveKeyWithValue("env", "prod"))
		Expect(ps.Topics).To(HaveLen(1))
		Expect(ps.Topics[0].Name).To(Equal("events"))
		Expect(ps.Topics[0].MessageRetentionDuration).To(Equal("604800s"))
		Expect(ps.Topics[0].Labels).To(HaveKeyWithValue("team", "core"))
		Expect(ps.Subscriptions).To(HaveLen(1))
		sub := ps.Subscriptions[0]
		Expect(sub.Name).To(Equal("events-sub"))
		Expect(sub.Topic).To(Equal("events"))
		Expect(sub.ExactlyOnceDelivery).To(BeTrue())
		Expect(sub.AckDeadlineSec).To(Equal(30))
		Expect(sub.DeadLetterPolicy).ToNot(BeNil())
		Expect(sub.DeadLetterPolicy.DeadLetterTopic).ToNot(BeNil())
		Expect(*sub.DeadLetterPolicy.DeadLetterTopic).To(Equal("events-dlq"))
		Expect(sub.DeadLetterPolicy.MaxDeliveryAttempts).ToNot(BeNil())
		Expect(*sub.DeadLetterPolicy.MaxDeliveryAttempts).To(Equal(5))
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"topics": "not-a-list",
		}}
		_, err := GcpPubSubTopicsReadConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

// ---- Redis ---------------------------------------------------------------

func TestRedisReadConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":         "my-gcp-project",
			"version":           "REDIS_7_0",
			"project":           "data-project",
			"memorySizeGb":      4,
			"region":            "europe-west1",
			"authorizedNetwork": "projects/p/global/networks/default",
			"redisConfig": map[string]any{
				"maxmemory-policy": "allkeys-lru",
			},
		}}
		out, err := RedisReadConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		r, ok := out.Config.(*RedisConfig)
		Expect(ok).To(BeTrue())
		Expect(r.Version).To(Equal("REDIS_7_0"))
		Expect(r.Project).To(Equal("data-project"))
		Expect(r.MemorySizeGb).To(Equal(4))
		Expect(r.Region).ToNot(BeNil())
		Expect(*r.Region).To(Equal("europe-west1"))
		Expect(r.AuthorizedNetwork).ToNot(BeNil())
		Expect(*r.AuthorizedNetwork).To(Equal("projects/p/global/networks/default"))
		Expect(r.RedisConfig).To(HaveKeyWithValue("maxmemory-policy", "allkeys-lru"))
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"memorySizeGb": "not-an-int",
		}}
		_, err := RedisReadConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

// ---- TemplateConfig (gcloud.go) -----------------------------------------

func TestReadTemplateConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":   "my-gcp-project",
			"credentials": `{"type":"service_account"}`,
		}}
		out, err := ReadTemplateConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		tc, ok := out.Config.(*TemplateConfig)
		Expect(ok).To(BeTrue())
		Expect(tc.ProjectId).To(Equal("my-gcp-project"))
		Expect(tc.CredentialsValue()).To(Equal(`{"type":"service_account"}`))
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"credentials": []int{1, 2, 3},
		}}
		_, err := ReadTemplateConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

// ---- GKE Autopilot readers ----------------------------------------------

func TestReadGkeAutopilotTemplateConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":                "my-gcp-project",
			"gkeClusterResource":       "my-cluster",
			"artifactRegistryResource": "my-registry",
		}}
		out, err := ReadGkeAutopilotTemplateConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		tpl, ok := out.Config.(*GkeAutopilotTemplate)
		Expect(ok).To(BeTrue())
		Expect(tpl.GkeClusterResource).To(Equal("my-cluster"))
		Expect(tpl.ArtifactRegistryResource).To(Equal("my-registry"))
		Expect(tpl.ProjectId).To(Equal("my-gcp-project"))
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"gkeClusterResource": map[string]any{"bad": "type"},
		}}
		_, err := ReadGkeAutopilotTemplateConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}

func TestReadGkeAutopilotResourceConfig(t *testing.T) {
	RegisterTestingT(t)

	t.Run("happy path maps nested timeouts, caddy, egress", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"projectId":     "my-gcp-project",
			"gkeMinVersion": "1.29",
			"location":      "europe-west1",
			"zone":          "europe-west1-b",
			"privateVpc":    true,
			"adopt":         true,
			"clusterName":   "adopted-cluster",
			"timeouts": map[string]any{
				"create": "30m",
				"update": "20m",
				"delete": "15m",
			},
			"externalEgressIp": map[string]any{
				"enabled":  true,
				"existing": "projects/p/regions/europe-west1/addresses/egress",
			},
		}}
		out, err := ReadGkeAutopilotResourceConfig(cfg)
		Expect(err).ToNot(HaveOccurred())
		r, ok := out.Config.(*GkeAutopilotResource)
		Expect(ok).To(BeTrue())
		Expect(r.GkeMinVersion).To(Equal("1.29"))
		Expect(r.Location).To(Equal("europe-west1"))
		Expect(r.Zone).To(Equal("europe-west1-b"))
		Expect(r.PrivateVpc).To(BeTrue())
		Expect(r.Adopt).To(BeTrue())
		Expect(r.ClusterName).To(Equal("adopted-cluster"))
		Expect(r.Timeouts).ToNot(BeNil())
		Expect(r.Timeouts.Create).To(Equal("30m"))
		Expect(r.Timeouts.Update).To(Equal("20m"))
		Expect(r.Timeouts.Delete).To(Equal("15m"))
		Expect(r.ExternalEgressIp).ToNot(BeNil())
		Expect(r.ExternalEgressIp.Enabled).To(BeTrue())
		Expect(r.ExternalEgressIp.Existing).To(Equal("projects/p/regions/europe-west1/addresses/egress"))
	})

	t.Run("error path", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &api.Config{Config: map[string]any{
			"privateVpc": "not-a-bool",
		}}
		_, err := ReadGkeAutopilotResourceConfig(cfg)
		Expect(err).To(HaveOccurred())
	})
}
