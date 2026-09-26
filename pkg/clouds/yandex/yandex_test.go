// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package yandex

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/api"
)

func intPtr(v int) *int { return &v }

func validSchedule() ContainerSchedule {
	return ContainerSchedule{
		Name:       "agents",
		Expression: "* * * * ? *",
		Request:    `{"path":"/api/v1/scheduler/agents/tick"}`,
	}
}

// TestNormalizedExpression covers the portability affordance: a schedule copied
// from an AWS client stack carries EventBridge's cron(...) wrapper, which YC does
// not accept. Stripping it means one schedule block works on both clouds.
func TestNormalizedExpression(t *testing.T) {
	RegisterTestingT(t)

	Expect((&ContainerSchedule{Expression: "cron(* * * * ? *)"}).NormalizedExpression()).
		To(Equal("* * * * ? *"))
	Expect((&ContainerSchedule{Expression: "  * * * * ? *  "}).NormalizedExpression()).
		To(Equal("* * * * ? *"))
	Expect((&ContainerSchedule{Expression: "cron(  0 8 ? * * *  )"}).NormalizedExpression()).
		To(Equal("0 8 ? * * *"))
}

func TestScheduleValidate(t *testing.T) {
	RegisterTestingT(t)

	for _, tc := range []struct {
		name    string
		mutate  func(*ContainerSchedule)
		wantErr string
	}{
		{name: "valid", mutate: func(*ContainerSchedule) {}},
		{
			name:    "empty name",
			mutate:  func(s *ContainerSchedule) { s.Name = "" },
			wantErr: "must not be empty",
		},
		{
			name:    "uppercase name",
			mutate:  func(s *ContainerSchedule) { s.Name = "Agents" },
			wantErr: "not a valid Yandex trigger name",
		},
		{
			name:    "name ending in hyphen",
			mutate:  func(s *ContainerSchedule) { s.Name = "agents-" },
			wantErr: "not a valid Yandex trigger name",
		},
		{
			name:    "name too short",
			mutate:  func(s *ContainerSchedule) { s.Name = "ab" },
			wantErr: "not a valid Yandex trigger name",
		},
		{
			name:    "five-field cron is rejected",
			mutate:  func(s *ContainerSchedule) { s.Expression = "* * * * *" },
			wantErr: "has 5 fields; Yandex requires 6",
		},
		{
			name:    "cron constraining both day-of-month and day-of-week",
			mutate:  func(s *ContainerSchedule) { s.Expression = "0 8 1 * MON *" },
			wantErr: "must use '?' for exactly one of day-of-month and day-of-week",
		},
		{
			name:    "cron with '?' in both positions",
			mutate:  func(s *ContainerSchedule) { s.Expression = "0 8 ? * ? *" },
			wantErr: "must use '?' for exactly one of day-of-month and day-of-week",
		},
		{
			name:    "AWS-wrapped cron is accepted",
			mutate:  func(s *ContainerSchedule) { s.Expression = "cron(* * * * ? *)" },
			wantErr: "",
		},
		{
			name:    "oversized payload",
			mutate:  func(s *ContainerSchedule) { s.Request = strings.Repeat("x", MaxRequestPayloadLen+1) },
			wantErr: "request payload is 4097 characters",
		},
		{
			name:    "payload measured in runes, not bytes",
			mutate:  func(s *ContainerSchedule) { s.Request = strings.Repeat("я", MaxRequestPayloadLen) },
			wantErr: "",
		},
		{
			name:    "retryAttempts above the cap",
			mutate:  func(s *ContainerSchedule) { s.RetryAttempts = intPtr(6) },
			wantErr: "retryAttempts is 6; Yandex allows 1..5",
		},
		{
			name:    "retryAttempts of zero",
			mutate:  func(s *ContainerSchedule) { s.RetryAttempts = intPtr(0) },
			wantErr: "retryAttempts is 0; Yandex allows 1..5",
		},
		{
			name:    "retryInterval below the floor",
			mutate:  func(s *ContainerSchedule) { s.RetryInterval = "5s" },
			wantErr: "retryInterval is 5s; Yandex allows 10s..1m0s",
		},
		{
			name:    "retryInterval above the ceiling",
			mutate:  func(s *ContainerSchedule) { s.RetryInterval = "90s" },
			wantErr: "retryInterval is 1m30s; Yandex allows 10s..1m0s",
		},
		{
			name:    "unitless retryInterval names the fix",
			mutate:  func(s *ContainerSchedule) { s.RetryInterval = "30" },
			wantErr: `must carry a unit, e.g. "30"s`,
		},
		{
			name:    "retryInterval at the bounds",
			mutate:  func(s *ContainerSchedule) { s.RetryInterval = "60s" },
			wantErr: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			s := validSchedule()
			tc.mutate(&s)
			err := s.Validate()
			if tc.wantErr == "" {
				Expect(err).ToNot(HaveOccurred())
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		})
	}
}

func TestCloudExtrasValidate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("duplicate schedule names are rejected", func(t *testing.T) {
		RegisterTestingT(t)
		ce := &CloudExtras{Schedules: []ContainerSchedule{validSchedule(), validSchedule()}}
		Expect(ce.Validate()).To(MatchError(ContainSubstring(`duplicate schedule name "agents"`)))
	})

	t.Run("zero concurrency is rejected", func(t *testing.T) {
		RegisterTestingT(t)
		Expect((&CloudExtras{Concurrency: intPtr(0)}).Validate()).
			To(MatchError(ContainSubstring("concurrency must be at least 1")))
	})

	t.Run("empty extras are valid", func(t *testing.T) {
		RegisterTestingT(t)
		Expect((&CloudExtras{}).Validate()).To(Succeed())
	})
}

// TestReadCloudExtrasFromClientYaml is the load-bearing test for this slice: it
// parses a cloudExtras block in the shape a real client.yaml carries (compare
// forge-conductor's, which declares lambdaSchedules the same way) and proves the
// schedules are reachable from the client stack — NOT from a server.yaml resource.
func TestReadCloudExtrasFromClientYaml(t *testing.T) {
	RegisterTestingT(t)

	const clientYaml = `
serviceAccountId: aje0example
roles:
  - lockbox.payloadViewer
  - ymq.writer
concurrency: 4
provisionedInstances: 1
schedules:
  - name: agents
    expression: "* * * * ? *"
    request: |-
      {"path": "/api/v1/scheduler/agents/tick", "httpMethod": "POST"}
  - name: wf-sched-tick
    expression: "cron(*/5 * * * ? *)"
    request: |-
      {"path": "/api/v1/scheduler/workflows/tick", "httpMethod": "POST"}
    retryAttempts: 3
    retryInterval: 15s
    dlq: sc-scheduler-dlq
`
	var raw any
	Expect(yaml.Unmarshal([]byte(clientYaml), &raw)).To(Succeed())

	ce, err := ReadCloudExtras(&raw)
	Expect(err).ToNot(HaveOccurred())
	Expect(ce.ServiceAccountID).To(Equal("aje0example"))
	Expect(ce.Roles).To(ConsistOf("lockbox.payloadViewer", "ymq.writer"))
	Expect(*ce.Concurrency).To(Equal(4))
	Expect(*ce.ProvisionedInstances).To(Equal(1))

	Expect(ce.Schedules).To(HaveLen(2))
	Expect(ce.Schedules[0].Name).To(Equal("agents"))
	Expect(ce.Schedules[0].Request).To(ContainSubstring("/api/v1/scheduler/agents/tick"))
	Expect(ce.Schedules[1].NormalizedExpression()).To(Equal("*/5 * * * ? *"))
	Expect(*ce.Schedules[1].RetryAttempts).To(Equal(3))
	Expect(ce.Schedules[1].DLQ).To(Equal("sc-scheduler-dlq"))

	interval, err := ce.Schedules[1].EffectiveRetryInterval()
	Expect(err).ToNot(HaveOccurred())
	Expect(interval.String()).To(Equal("15s"))
}

func TestReadCloudExtrasNilAndInvalid(t *testing.T) {
	RegisterTestingT(t)

	// A stack with no cloudExtras block must yield usable empty extras, so callers
	// need not nil-check before ranging over Schedules.
	ce, err := ReadCloudExtras(nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(ce.Schedules).To(BeEmpty())

	var raw any
	Expect(yaml.Unmarshal([]byte("schedules:\n  - name: Bad_Name\n    expression: \"* * * * ? *\"\n"), &raw)).To(Succeed())
	_, err = ReadCloudExtras(&raw)
	Expect(err).To(MatchError(ContainSubstring("not a valid Yandex trigger name")))
}

func TestReadTemplateConfig(t *testing.T) {
	RegisterTestingT(t)

	out, err := ReadTemplateConfig(&api.Config{Config: map[string]any{
		"folderId":         "b1gfolder",
		"registryId":       "crpexample",
		"serviceAccountId": "aje0example",
	}})
	Expect(err).ToNot(HaveOccurred())
	tpl, ok := out.Config.(*TemplateConfig)
	Expect(ok).To(BeTrue())
	Expect(tpl.FolderID).To(Equal("b1gfolder"))
	Expect(tpl.RegistryID).To(Equal("crpexample"))
	Expect(tpl.ServiceAccountID).To(Equal("aje0example"))
}
