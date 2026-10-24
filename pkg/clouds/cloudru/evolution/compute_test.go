// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/samber/lo"
)

// fakeCompute serves the Evolution Compute API off one httptest server.
//
// Deliberately picky about the things that are easy to get wrong against the real
// API and impossible to notice against a permissive fake, each of which cost a live
// round-trip to discover (2026-10-23/24):
//
//   - the project scope rides a **snake_case** `project_id` QUERY parameter on every
//     verb (the Containers and Registry APIs use camelCase `projectId` — the two
//     halves of the same cloud disagree);
//   - except on `POST /security-groups/{id}/rules`, which declares no such field and
//     answers 422 `extra_forbidden` when it is sent;
//   - `POST /vms` takes a **list** body, `POST /floating-ips` a single object (a list
//     there answers HTTP 500);
//   - `/availability-zones` and `/disk-types` answer a BARE ARRAY while the other
//     seven collections answer a paged envelope — the split follows nothing, so it
//     is measured rather than reasoned about;
//   - the server's own default page is 50, with the real count only in `total`;
//   - a fresh disk is `creating` and cannot be attached until it is `available`.
type fakeCompute struct {
	mu sync.Mutex

	projectID string

	zones    []AvailabilityZone
	flavors  []Flavor
	images   []Image
	diskType []DiskType
	subnets  []Subnet
	groups   []SecurityGroup
	disks    []Disk
	vms      []Vm
	fips     []FloatingIP

	// pageSize caps what a list answers regardless of the requested `limit`, which
	// is what the real API does: it reports the full count in `total` and truncates
	// silently. 0 means "answer everything".
	pageSize int

	// calls records every request as "METHOD /path", in order: for a converge whose
	// correctness IS the order (disk before VM, VM before floating-IP attach), the
	// sequence is the assertion.
	calls []string

	createdDisks []createDiskRequest
	createdVms   []createVmRequest
	createdFips  []createFloatingIPRequest
	createdRules []SecurityGroupRule

	deletedVms   []string
	deletedDisks []string
	deletedFips  []string
	// deletedRules is every rule id the destroy sweep asked to remove. The
	// assertion worth making is which ones are NOT here: the VM's own rules go, and
	// the rules of every other SC machine sharing the project's default group stay.
	deletedRules []string

	// onVmGet mutates the fake before each VM read, so a test can let a machine
	// progress from `creating` to `running` the way the platform does.
	onVmGet func(f *fakeCompute)
}

func (f *fakeCompute) record(r *http.Request) {
	f.calls = append(f.calls, r.Method+" "+strings.TrimPrefix(r.URL.Path, computeAPIPath))
}

// requireProject asserts the one thing every compute route shares.
func (f *fakeCompute) requireProject(r *http.Request) {
	Expect(r.Header.Get("Authorization")).To(Equal("Bearer token"))
	Expect(r.URL.Query().Get("project_id")).To(Equal(f.projectID),
		"every compute route is project-scoped through a snake_case `project_id` query parameter; "+
			"omitting it answers 403 `non_authorized`, which reads exactly like a missing IAM role")
}

// writePage answers a paged envelope, truncating at pageSize so that a client which
// does not follow `offset` reads a short list and does not notice.
func writePage[T any](w http.ResponseWriter, r *http.Request, all []T, pageSize int) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if pageSize > 0 && (limit == 0 || limit > pageSize) {
		limit = pageSize
	}
	if limit <= 0 {
		limit = len(all)
	}
	items := []T{}
	if offset < len(all) {
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		items = all[offset:end]
	}
	writeJSON(w, pagedEnvelope[T]{Items: items, Total: len(all), Limit: limit, Offset: offset})
}

func (f *fakeCompute) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, tokenResponse{AccessToken: "token", ExpiresIn: 3600, TokenType: "Bearer"})
	})

	// A bare-array collection (the other is `/disk-types`). A client that assumed
	// one shape for all nine decodes this into nothing at all.
	mux.HandleFunc(computeAPIPath+"/availability-zones", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		writeJSON(w, f.zones)
	})

	mux.HandleFunc(computeAPIPath+"/flavors", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		writePage(w, r, f.flavors, f.pageSize)
	})
	mux.HandleFunc(computeAPIPath+"/images", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		writePage(w, r, f.images, f.pageSize)
	})
	// A BARE ARRAY, like `/availability-zones` and unlike the other seven. This fake
	// answered an envelope here until 2026-10-24, when the first live VM create failed
	// on `cannot unmarshal array into pagedEnvelope` — the fake had been built from
	// the assumption that `/availability-zones` was the only exception, so it agreed
	// with the client and proved nothing.
	mux.HandleFunc(computeAPIPath+"/disk-types", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		writeJSON(w, f.diskType)
	})
	mux.HandleFunc(computeAPIPath+"/subnets", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		writePage(w, r, f.subnets, f.pageSize)
	})
	mux.HandleFunc(computeAPIPath+"/security-groups", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		// Rules STRIPPED, like the real route: it answers `ingress_rules_count` and
		// an empty `rules` (measured 2026-10-24). A fake that inlined them would let
		// a client read the group's rules from here, pass, and then re-create every
		// rule on every live deploy — and find nothing to clean up on destroy.
		blind := lo.Map(f.groups, func(g SecurityGroup, _ int) SecurityGroup {
			g.Rules = nil
			return g
		})
		writePage(w, r, blind, f.pageSize)
	})

	// The whole `/security-groups/{id}/rules[/{ruleId}]` sub-tree, which behaves
	// unlike every other compute route in the same two ways on all three verbs: it
	// declares no project field, and it is the only place rules are visible at all
	// (the group endpoints answer `ingress_rules_count` and an empty `rules`).
	mux.HandleFunc(computeAPIPath+"/security-groups/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		// The exception, and the one that cost twenty minutes of imagined permission
		// debugging: these routes declare no project field and refuse the request
		// outright when it is sent.
		Expect(r.URL.Query().Get("project_id")).To(BeEmpty(),
			"`/security-groups/{id}/rules` answers 422 `extra_forbidden` for a `project_id`")

		rest := strings.TrimPrefix(r.URL.Path, computeAPIPath+"/security-groups/")
		groupID, tail, _ := strings.Cut(rest, "/rules")
		ruleID := strings.TrimPrefix(tail, "/")
		Expect(strings.HasPrefix(tail, "/rules")).To(BeFalse(), "only `/rules` and `/rules/{id}` exist here")

		switch {
		case r.Method == http.MethodPost && ruleID == "":
			var rule SecurityGroupRule
			Expect(json.NewDecoder(r.Body).Decode(&rule)).To(Succeed())
			f.createdRules = append(f.createdRules, rule)
			// The id is assigned by the platform and is the only handle a delete
			// has — a fake that left it empty would make the destroy sweep look
			// like it worked while deleting nothing.
			if rule.ID == "" {
				rule.ID = fmt.Sprintf("rule-%d", len(f.createdRules))
			}
			for i := range f.groups {
				if f.groups[i].ID == groupID {
					f.groups[i].Rules = append(f.groups[i].Rules, rule)
				}
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && ruleID == "":
			for _, g := range f.groups {
				if g.ID == groupID {
					writeJSON(w, pagedEnvelope[SecurityGroupRule]{Items: g.Rules, Total: len(g.Rules)})
					return
				}
			}
			http.Error(w, `{"error_msg":"not found"}`, http.StatusNotFound)
		case r.Method == http.MethodDelete && ruleID != "":
			for i := range f.groups {
				if f.groups[i].ID != groupID {
					continue
				}
				for j, rule := range f.groups[i].Rules {
					if rule.ID != ruleID {
						continue
					}
					f.deletedRules = append(f.deletedRules, ruleID)
					f.groups[i].Rules = append(f.groups[i].Rules[:j], f.groups[i].Rules[j+1:]...)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			// A rule that is already gone answers 404, which the client treats as
			// success — the delete is async, so a retry lands here routinely.
			http.Error(w, `{"error_msg":"not found"}`, http.StatusNotFound)
		default:
			Expect(r.Method).To(BeEmpty(), "unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	mux.HandleFunc(computeAPIPath+"/disks", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		switch r.Method {
		case http.MethodGet:
			writePage(w, r, f.disks, f.pageSize)
		case http.MethodPost:
			var req createDiskRequest
			Expect(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			Expect(req.ProjectID).To(Equal(f.projectID), "the project rides the body too on a create")
			f.createdDisks = append(f.createdDisks, req)
			// `creating`, like the real API — NOT `available`. This fake answered
			// `available` here until 2026-10-24, which is why 16 green tests did not
			// predict `422 disk_can_not_be_attached_from_current_state` on the second
			// live create. A fake that skips a transient state cannot catch a race.
			disk := Disk{
				ID: "disk-1", Name: req.Name, Size: req.Size, State: "creating", Bootable: true,
				Zone: zoneRef{Name: req.Zone}, Image: namedRef{ID: req.ImageID},
			}
			f.disks = append(f.disks, disk)
			writeJSON(w, disk)
		default:
			t.Fatalf("unexpected %s /disks", r.Method)
		}
	})

	mux.HandleFunc(computeAPIPath+"/disks/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		id := strings.TrimPrefix(r.URL.Path, computeAPIPath+"/disks/")
		switch r.Method {
		case http.MethodGet:
			idx := lo.IndexOf(lo.Map(f.disks, func(d Disk, _ int) string { return d.ID }), id)
			if idx < 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			// A read of a `creating` disk settles it, so a poll converges in one tick
			// rather than hanging the test on a state that never changes.
			if f.disks[idx].State == "creating" {
				f.disks[idx].State = DiskStateAvailable
			}
			writeJSON(w, f.disks[idx])
		case http.MethodDelete:
			f.deletedDisks = append(f.deletedDisks, id)
			f.disks = lo.Filter(f.disks, func(d Disk, _ int) bool { return d.ID != id })
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected %s /disks/%s", r.Method, id)
		}
	})

	mux.HandleFunc(computeAPIPath+"/vms", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		switch r.Method {
		case http.MethodGet:
			writePage(w, r, f.vms, f.pageSize)
		case http.MethodPost:
			// A LIST, not an object: the API accepts a batch here. The raw body is
			// decoded into a list on purpose — an object body is a different request and
			// this is where the difference has to be caught.
			var reqs []createVmRequest
			Expect(json.NewDecoder(r.Body).Decode(&reqs)).To(Succeed())
			Expect(reqs).To(HaveLen(1), "SC always sends exactly one: a batch failure reports no per-item status")
			req := reqs[0]
			Expect(req.ProjectID).To(Equal(f.projectID))
			Expect(req.Disks).ToNot(BeEmpty(), "an empty `disks` answers HTTP 500, not a validation error")
			// The race the real API refuses. Modelled as the server's answer rather
			// than a test assertion so that removing the wait in ApplyVm fails the
			// test the same way it failed the live create.
			for _, d := range req.Disks {
				if idx := lo.IndexOf(lo.Map(f.disks, func(x Disk, _ int) string { return x.ID }), d.DiskID); idx >= 0 {
					if f.disks[idx].State != DiskStateAvailable {
						w.WriteHeader(http.StatusUnprocessableEntity)
						writeJSON(w, []map[string]string{{
							"message": "Disk can not be attached from current state",
							"code":    "disk_can_not_be_attached_from_current_state",
						}})
						return
					}
				}
			}
			f.createdVms = append(f.createdVms, req)
			vm := Vm{
				ID: "vm-1", Name: req.Name, State: "creating", ImageID: "image-1",
				Zone:   zoneRef{Name: req.Zone},
				Flavor: f.flavorByID(req.FlavorID),
				Disks:  []namedRef{{ID: req.Disks[0].DiskID}},
				Interfaces: []VmInterface{{
					ID: "iface-1", Primary: true, IPAddress: "10.0.0.5",
					Subnet:         namedRef{ID: "subnet-1"},
					SecurityGroups: []namedRef{{ID: "sg-default", Name: "default"}},
				}},
			}
			f.vms = append(f.vms, vm)
			writeJSON(w, []Vm{vm})
		default:
			t.Fatalf("unexpected %s /vms", r.Method)
		}
	})

	mux.HandleFunc(computeAPIPath+"/vms/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		id := strings.TrimPrefix(r.URL.Path, computeAPIPath+"/vms/")
		switch r.Method {
		case http.MethodGet:
			if f.onVmGet != nil {
				f.onVmGet(f)
			}
			for i := range f.vms {
				if f.vms[i].ID == id {
					writeJSON(w, f.vms[i])
					return
				}
			}
			http.Error(w, `{"error_msg":"not found"}`, http.StatusNotFound)
		case http.MethodDelete:
			f.deletedVms = append(f.deletedVms, id)
			f.vms = lo.Filter(f.vms, func(v Vm, _ int) bool { return v.ID != id })
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	mux.HandleFunc(computeAPIPath+"/floating-ips", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		switch r.Method {
		case http.MethodGet:
			writePage(w, r, f.fips, f.pageSize)
		case http.MethodPost:
			// A single OBJECT here, unlike /vms. A list answers HTTP 500, and the two
			// routes sitting one function apart in the client is exactly how that gets
			// made uniform by a later edit.
			var req createFloatingIPRequest
			Expect(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			Expect(req.ProjectID).To(Equal(f.projectID))
			f.createdFips = append(f.createdFips, req)
			fip := FloatingIP{ID: "fip-1", Name: req.Name, IPAddress: "", State: "creating", Zone: zoneRef{Name: req.Zone}}
			f.fips = append(f.fips, fip)
			writeJSON(w, fip)
		default:
			t.Fatalf("unexpected %s /floating-ips", r.Method)
		}
	})

	mux.HandleFunc(computeAPIPath+"/floating-ips/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.record(r)
		f.requireProject(r)
		id := strings.TrimPrefix(r.URL.Path, computeAPIPath+"/floating-ips/")
		switch r.Method {
		case http.MethodGet:
			for i := range f.fips {
				if f.fips[i].ID == id {
					// Becomes usable on the first re-read, which is what the poll in
					// ensureFloatingIP exists for.
					if f.fips[i].State == "creating" {
						f.fips[i].State = FloatingIPStateAvailable
						f.fips[i].IPAddress = "176.108.247.222"
					}
					writeJSON(w, f.fips[i])
					return
				}
			}
			http.Error(w, `{"error_msg":"not found"}`, http.StatusNotFound)
		case http.MethodPut:
			// PUT with `{"interface_id": …}`: there is no `POST …/attach` route and
			// PATCH is not allowed.
			var body map[string]string
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
			Expect(body).To(HaveKey("interface_id"))
			for i := range f.fips {
				if f.fips[i].ID == id {
					f.fips[i].State = FloatingIPStateInUse
					f.fips[i].Interface = &namedRef{ID: body["interface_id"]}
				}
			}
			for i := range f.vms {
				for j := range f.vms[i].Interfaces {
					if f.vms[i].Interfaces[j].ID == body["interface_id"] {
						f.vms[i].Interfaces[j].FloatingIP = &namedRef{ID: id, Name: "sc-vm-ip"}
					}
				}
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			f.deletedFips = append(f.deletedFips, id)
			f.fips = lo.Filter(f.fips, func(p FloatingIP, _ int) bool { return p.ID != id })
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	return mux
}

func (f *fakeCompute) flavorByID(id string) Flavor {
	for _, fl := range f.flavors {
		if fl.ID == id {
			return fl
		}
	}
	return Flavor{ID: id}
}

func (f *fakeCompute) callsTo(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return lo.CountBy(f.calls, func(c string) bool { return c == method+" "+path })
}

// indexOfCall is the position of the first matching call, for the assertions where
// the ORDER is the correctness: the disk must exist before the VM, the VM before its
// floating-IP attach, or the API refuses the step with an error about the wrong thing.
func (f *fakeCompute) indexOfCall(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return lo.IndexOf(f.calls, method+" "+path)
}

// ubuntuImage is the real Ubuntu-24.04 image of the operator's project: the metadata
// slots and `min_disk` are copied from the live `/images` answer (2026-10-24), because
// the whole boot contract on this cloud is those slots.
func ubuntuImage() Image {
	return Image{
		ID:          "fe51a6e6-a5ad-410e-b441-1a195208a8cf",
		Name:        "Ubuntu-24.04",
		DisplayName: "Ubuntu 24.04 LTS",
		MinDisk:     11,
		Metadata: []ImageMetadataField{
			{Name: "hostname", FieldType: "string"},
			{Name: "vmm_enabled", FieldType: "bool"},
			{Name: "name", FieldType: "string", MandatoryGroup: "login"},
			{Name: "public_key", FieldType: "string", MandatoryGroup: "authentication"},
			{Name: "linux_password", FieldType: "string", MandatoryGroup: "authentication", Sensitive: true},
			{Name: "guest_agent_ready", FieldType: "bool"},
		},
	}
}

func newComputeFake() *fakeCompute {
	zone := "ru-central-1a"
	return &fakeCompute{
		projectID: "project-1",
		zones: []AvailabilityZone{
			{ID: "az-b", Name: "ru-central-1b", Enabled: true},
			{ID: "az-a", Name: zone, Enabled: true, Default: true},
		},
		flavors: []Flavor{
			// Named the way the catalogue named them on 2026-10-24 — a day after the
			// same shapes were called `low-2-4` and `low-4-8`.
			{ID: "fl-2-4", Name: "lowcost10-2-4", CPU: 2, RAM: 4, Zones: []flavorZone{{ZoneName: zone, Enabled: true}}},
			{ID: "fl-4-8", Name: "lowcost10-4-8", CPU: 4, RAM: 8, Zones: []flavorZone{{ZoneName: zone, Enabled: true}}},
			{ID: "fl-gpu", Name: "gpu-2-4", CPU: 2, RAM: 4, GPU: 1, Zones: []flavorZone{{ZoneName: zone, Enabled: true}}},
			{ID: "fl-other-az", Name: "lowcost10-2-4", CPU: 2, RAM: 4, Zones: []flavorZone{{ZoneName: "ru-central-1b", Enabled: true}}},
		},
		images:   []Image{ubuntuImage(), {ID: "img-22", Name: "ubuntu-22.04", MinDisk: 11, Metadata: ubuntuImage().Metadata}},
		diskType: []DiskType{{ID: "dt-ssd", Name: "SSD", DisplayName: "SSD"}},
		subnets:  []Subnet{{ID: "subnet-1", Name: "default", Default: true, Zone: zoneRef{Name: zone}}},
		groups: []SecurityGroup{{
			ID: "sg-default", Name: "default", Default: true, Zone: zoneRef{Name: zone},
			// The default group's own rule: `any`/`any`, but scoped to the GROUP rather
			// than to 0.0.0.0/0, which is not the same reachability.
			Rules: []SecurityGroupRule{{Direction: "ingress", IPProtocol: "any", PortRange: "any", RemoteIPPrefix: ""}},
		}},
	}
}

func newComputeClient(t *testing.T, f *fakeCompute) *Client {
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return New(Credentials{KeyID: "key-id", Secret: "key-secret", ProjectID: f.projectID}, WithAllEndpoints(srv.URL))
}

func vmRequest(spec VmSpec) Request {
	return Request{
		Credentials:  Credentials{KeyID: "key-id", Secret: "key-secret", ProjectID: "project-1"},
		Name:         spec.Name,
		VmSpec:       &spec,
		PollInterval: "1ms",
		WaitTimeout:  "5s",
	}
}

func smokeVmSpec() VmSpec {
	return VmSpec{
		Name:      "ruvm--staging-vm-host-0",
		Cores:     2,
		MemoryGiB: 4,
		Image:     "ubuntu-2404-lts",
		SSHUser:   "ubuntu",
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDEPLOY\n",
		PublicIP:  true,
		// Described the way the driver describes them — through RuleDescription, which
		// embeds the VM's name. On this cloud the machine joins the project's SHARED
		// default group, so that name in the description is the only thing that tells
		// this VM's rules from another SC machine's at destroy time.
		IngressRules: []IngressRule{
			{Protocol: "tcp", Port: 22, Description: RuleDescription("ruvm--staging-vm-host-0", "ssh")},
			{Protocol: "udp", Port: 443, Description: RuleDescription("ruvm--staging-vm-host-0", "https-h3")},
		},
	}
}

// The name grammar, quoted from the 422 the API returns (`string_pattern_mismatch`).
// SC's own names are stack+environment+role, so both the leading character and the
// 64-char cap are reachable from real input.
func TestComputeNames(t *testing.T) {
	RegisterTestingT(t)

	// A doubled hyphen is fine here, unlike in Artifact Registry — the two halves of
	// this cloud have different name grammars and collapsing it would make SC's
	// compute names disagree with the ones in the console.
	Expect(toComputeName("ruvm--staging-vm")).To(Equal("ruvm--staging-vm"))
	Expect(ValidateComputeName("vm", "ruvm--staging-vm")).To(Succeed())

	// Must START with a letter. Prefixed rather than stripped: dropping the leading
	// character would let two differently-named resources collapse onto one VM.
	Expect(toComputeName("1stack-vm")).To(Equal("sc-1stack-vm"))
	Expect(toComputeName("-vm")).To(Equal("sc--vm"))
	Expect(toComputeName("_vm")).To(Equal("sc-_vm"))

	// Anything outside the grammar becomes a hyphen rather than disappearing.
	Expect(toComputeName("stack/env vm")).To(Equal("stack-env-vm"))
	Expect(toComputeName("  spaced  ")).To(Equal("spaced"))
	Expect(toComputeName("")).To(BeEmpty())

	// The cap, and no trailing separator left behind by the trim.
	long := "a" + strings.Repeat("b", 80)
	Expect(toComputeName(long)).To(HaveLen(ComputeNameMaxLen))
	Expect(toComputeName("a" + strings.Repeat("b", 62) + "-cde")).ToNot(HaveSuffix("-"))

	// The refusal names the grammar AND the part that is actually wrong: "does not
	// match ^[a-zA-Z]…" alone sends an operator checking their hyphens.
	err := ValidateComputeName("vm", "1vm")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("must START with a letter"))
	Expect(ValidateComputeName("disk", "")).To(HaveOccurred())

	// A satellite object's name is trimmed from the BASE, keeping the suffix: the
	// suffix is what makes the object identifiable in the console, and a destroy finds
	// it by exactly this name after a create that failed halfway.
	base := strings.Repeat("v", ComputeNameMaxLen)
	Expect(derivedName(base, vmDiskSuffix)).To(HaveSuffix(vmDiskSuffix))
	Expect(derivedName(base, vmDiskSuffix)).To(HaveLen(ComputeNameMaxLen))
	Expect(derivedName("sc-vm", vmFloatingIPSuffix)).To(Equal("sc-vm-ip"))
}

// The pagination trap, and the reason listPaged exists: the server's default page is
// 50 items and nothing but `total` says the answer was truncated. A client that read
// one page would decide the project has no VM by that name and create a second one.
func TestListPagedFollowsOffset(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.pageSize = 50
	f.vms = nil
	for i := 0; i < 120; i++ {
		f.vms = append(f.vms, Vm{ID: fmt.Sprintf("vm-%d", i), Name: fmt.Sprintf("vm-%d", i)})
	}
	c := newComputeClient(t, f)

	vms, err := c.ListVms(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(vms).To(HaveLen(120))
	Expect(f.callsTo(http.MethodGet, "/vms")).To(Equal(3), "120 items at 50 per page is three requests")

	// And the machine hiding on page three is found, which is the actual consequence.
	vm, found, err := c.FindVm(context.Background(), "vm-117")
	Expect(err).ToNot(HaveOccurred())
	Expect(found).To(BeTrue())
	Expect(vm.ID).To(Equal("vm-117"))
}

// `/availability-zones` is the one compute collection that answers a bare array. A
// client that assumed the envelope here decodes it into nothing and reports a project
// with no zones.
func TestAvailabilityZonesAndDefaultZone(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	c := newComputeClient(t, f)

	zones, err := c.ListAvailabilityZones(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(zones).To(HaveLen(2))

	// The one flagged `default`, not the first in the list — the API's order is not
	// the project's preference.
	zone, err := c.DefaultZone(context.Background())
	Expect(err).ToNot(HaveOccurred())
	Expect(zone).To(Equal("ru-central-1a"))
}

// The AZ flagged `default: true` is NOT where a VM can be placed, and preferring it
// is how the first SC-created VM failed. Measured on the operator's project
// 2026-10-24: three enabled AZs, `ru.AZ-1` flagged default, and the project's only
// subnet in `ru.AZ-3`. `POST /vms` takes no `subnet_id`, so a create in the flagged
// zone answers `422 default_subnet_not_found` — after the boot disk already exists.
func TestDefaultZoneFollowsTheSubnetNotTheAzFlag(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	// The live arrangement exactly.
	f.zones = []AvailabilityZone{
		{Name: "ru.AZ-1", Enabled: true, Default: true},
		{Name: "ru.AZ-2", Enabled: true},
		{Name: "ru.AZ-3", Enabled: true},
	}
	f.subnets = []Subnet{{ID: "subnet-1", Name: "Default_ru.AZ-3", Default: true, Zone: zoneRef{Name: "ru.AZ-3"}}}
	c := newComputeClient(t, f)
	ctx := context.Background()

	zone, err := c.DefaultZone(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(zone).To(Equal("ru.AZ-3"), "the zone with a subnet, not the zone with the flag")

	// A configured-but-unplaceable zone is refused up front, and the message names
	// the zones that would work — `default_subnet_not_found` names neither.
	err = c.AssertZonePlaceable(ctx, "ru.AZ-1")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ru.AZ-1"))
	Expect(err.Error()).To(ContainSubstring("ru.AZ-3"))
	Expect(c.AssertZonePlaceable(ctx, "ru.AZ-3")).To(Succeed())

	// Nothing may be created before that refusal — the whole point of checking here
	// rather than letting `POST /vms` do it is that the disk comes first.
	spec := smokeVmSpec()
	spec.Zone = "ru.AZ-1"
	_, err = c.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("no default subnet"))
	Expect(f.createdDisks).To(BeEmpty(), "a mis-zoned config must not orphan a boot disk")

	// When the flagged-default zone IS placeable it wins, so a project set up in
	// several AZs gets a stable answer rather than one that depends on sort order.
	f.subnets = append(f.subnets,
		Subnet{ID: "subnet-2", Name: "Default_ru.AZ-1", Default: true, Zone: zoneRef{Name: "ru.AZ-1"}})
	zone, err = c.DefaultZone(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(zone).To(Equal("ru.AZ-1"))

	// No subnet anywhere: the refusal says what to do once, in the console, rather
	// than naming a zone.
	f.subnets = nil
	_, err = c.DefaultZone(ctx)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("Create a network"))
	Expect(c.AssertZonePlaceable(ctx, "ru.AZ-3")).To(HaveOccurred())
}

// Either shape, on any path. This is the regression test for the bug that the
// first live VM create found and 15 unit tests did not: the client decoded every
// collection as an envelope except `/availability-zones`, because that was the only
// bare array anyone had noticed — and `/disk-types` is one too. The fake shared the
// same wrong assumption, so it agreed with the client all the way to the cloud.
//
// Measured 2026-10-24: bare array on `/availability-zones` and `/disk-types`,
// envelope on `/flavors`, `/images`, `/subnets`, `/security-groups`, `/disks`,
// `/vms`, `/floating-ips`. The split follows nothing, which is why the decoder
// sniffs the first byte instead of keying off a list of paths.
func TestDecodeCollectionTakesEitherShape(t *testing.T) {
	RegisterTestingT(t)

	envelope, err := decodeCollection[DiskType]([]byte(`{"items":[{"name":"SSD"}],"total":1,"limit":50,"offset":0}`))
	Expect(err).ToNot(HaveOccurred())
	Expect(envelope.Items).To(HaveLen(1))
	Expect(envelope.Total).To(Equal(1))

	// A bare array reports its own length as the total, so the caller stops after one
	// page rather than looping on a `total` of zero.
	bare, err := decodeCollection[DiskType]([]byte(` [{"name":"SSD"},{"name":"HDD"}] `))
	Expect(err).ToNot(HaveOccurred())
	Expect(bare.Items).To(HaveLen(2))
	Expect(bare.Total).To(Equal(2))

	// Leading whitespace must not decide the shape.
	nl, err := decodeCollection[DiskType]([]byte("\n\t[]"))
	Expect(err).ToNot(HaveOccurred())
	Expect(nl.Items).To(BeEmpty())
	Expect(nl.Total).To(BeZero())

	_, err = decodeCollection[DiskType]([]byte(`not json`))
	Expect(err).To(HaveOccurred())

	// And end to end: a bare-array `/disk-types` plus an enveloped `/flavors` in the
	// same converge, which is the exact pair the live create tripped on.
	f := newComputeFake()
	c := newComputeClient(t, f)
	dt, err := c.FindDiskType(context.Background(), "")
	Expect(err).ToNot(HaveOccurred())
	Expect(dt.Name).To(Equal("SSD"))
	fl, err := c.FindFlavor(context.Background(), "ru-central-1a", 2, 4)
	Expect(err).ToNot(HaveOccurred())
	Expect(fl.ID).To(Equal("fl-2-4"))
}

// A flavor is resolved by SHAPE, never by name or id: the catalogue renamed the
// 2 vCPU / 4 GiB size from `low-2-4` to `lowcost10-2-4` between 2026-10-23 and
// 2026-10-24, with no `low-2-4` left. A name compiled into SC would have failed a
// deploy on Cloud.ru's schedule.
func TestFindFlavor(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	c := newComputeClient(t, f)
	ctx := context.Background()

	flavor, err := c.FindFlavor(ctx, "ru-central-1a", 2, 4)
	Expect(err).ToNot(HaveOccurred())
	Expect(flavor.ID).To(Equal("fl-2-4"))

	// The SMALLEST that satisfies both, not the first that does: a request for
	// 3 vCPU takes the 4-vCPU flavor rather than the 2-vCPU one it does not fit in.
	flavor, err = c.FindFlavor(ctx, "ru-central-1a", 3, 4)
	Expect(err).ToNot(HaveOccurred())
	Expect(flavor.ID).To(Equal("fl-4-8"))

	// GPU flavors are excluded: they cost an order of magnitude more and are never
	// what a `size:` without a GPU field asked for. `fl-gpu` is the same 2/4 shape.
	Expect(flavor.GPU).To(Equal(0))
	all, err := c.ListFlavors(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(lo.SomeBy(all, func(fl Flavor) bool { return fl.GPU > 0 })).To(BeTrue(),
		"the fake must offer a GPU flavor or the exclusion is untested")

	// Zone-filtered: `fl-other-az` has exactly the shape asked for and is enabled in
	// a different AZ, which is a create that fails after the disk already exists.
	_, err = c.FindFlavor(ctx, "ru-central-1c", 2, 4)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ru-central-1c"))
	// The count is in the message: "no flavor with this shape" against a list that
	// came back empty is a different problem (a scope/credential one) from the same
	// message against 78 flavors.
	Expect(err.Error()).To(ContainSubstring("read 4 flavors"))

	_, err = c.FindFlavor(ctx, "ru-central-1a", 64, 512)
	Expect(err).To(HaveOccurred())
}

// `image:` accepts a UUID, a catalogue name, a display name, or one of SC's portable
// aliases — the alias is what lets the same client.yaml name an image on two clouds.
func TestFindImage(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	c := newComputeClient(t, f)
	ctx := context.Background()

	for _, name := range []string{
		"ubuntu-2404-lts", // SC's own default alias
		"Ubuntu-24.04",    // the catalogue name
		"ubuntu-24.04",    // and case-insensitively
		"Ubuntu 24.04 LTS",
		"fe51a6e6-a5ad-410e-b441-1a195208a8cf",
	} {
		img, err := c.FindImage(ctx, name)
		Expect(err).ToNot(HaveOccurred(), "image %q", name)
		Expect(img.ID).To(Equal(ubuntuImage().ID), "image %q", name)
	}

	// The refusal lists the aliases, because the operator's next move is to pick one.
	_, err := c.FindImage(ctx, "debian-12")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("ubuntu-2404-lts"))
	_, err = c.FindImage(ctx, "")
	Expect(err).To(HaveOccurred())
}

// The disk type defaults to the project's only one rather than to a name SC invented:
// the names are not the same across projects, and a wrong one fails the DISK create,
// which reads as a storage-quota problem.
func TestFindDiskType(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	c := newComputeClient(t, f)
	ctx := context.Background()

	dt, err := c.FindDiskType(ctx, "")
	Expect(err).ToNot(HaveOccurred())
	Expect(dt.Name).To(Equal("SSD"))

	dt, err = c.FindDiskType(ctx, "ssd")
	Expect(err).ToNot(HaveOccurred())
	Expect(dt.Name).To(Equal("SSD"))

	_, err = c.FindDiskType(ctx, "nvme")
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("available: SSD"))
}

// Drift is compared by cores and memory rather than by flavor id — BECAUSE the
// catalogue renames flavors. An id comparison would declare drift on a machine nobody
// touched and offer to destroy it.
func TestCheckVmSizeDrift(t *testing.T) {
	RegisterTestingT(t)

	running := &Vm{Name: "sc-vm", Flavor: Flavor{ID: "fl-old", Name: "low-2-4", CPU: 2, RAM: 4}}

	// Yesterday's name, today's catalogue, same machine: not drift.
	Expect(checkVmSizeDrift(running, &Flavor{ID: "fl-2-4", Name: "lowcost10-2-4", CPU: 2, RAM: 4})).To(Succeed())

	err := checkVmSizeDrift(running, &Flavor{ID: "fl-4-8", Name: "lowcost10-4-8", CPU: 4, RAM: 8})
	Expect(err).To(HaveOccurred())
	// Refused, not performed: Cloud.ru has no in-place resize, so "converging" this
	// would mean destroying the machine and everything on its disk. The message has to
	// say so and name the deliberate way out.
	Expect(err.Error()).To(ContainSubstring("will not resize"))
	Expect(err.Error()).To(ContainSubstring("sc destroy"))
	Expect(err.Error()).To(ContainSubstring("2 vCPU / 4 GiB"))
	Expect(err.Error()).To(ContainSubstring("4 vCPU / 8 GiB"))
}

// The whole converge, in order. Every assertion is a measured property of the API:
// each was learned from a live 422 or 500 against an account that bills for it.
func TestApplyVmCreatesTheMachine(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.onVmGet = func(f *fakeCompute) {
		// The platform's own progression, which waitVmRunning polls for.
		for i := range f.vms {
			if f.vms[i].State == "creating" {
				f.vms[i].State = VmStateRunning
			}
		}
	}
	c := newComputeClient(t, f)

	res, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())

	Expect(res.Name).To(Equal("ruvm--staging-vm-host-0"))
	Expect(res.VmID).To(Equal("vm-1"))
	// The FLOATING ip, not the private one: Address is what every later SSH
	// connection dials, and the private address is reported separately so an operator
	// can tell them apart.
	Expect(res.Address).To(Equal("176.108.247.222"))
	Expect(res.InternalAddress).To(Equal("10.0.0.5"))
	Expect(res.User).To(Equal("ubuntu"))
	Expect(res.FlavorName).To(Equal("lowcost10-2-4"))
	Expect(res.URL).To(Equal("ssh://ubuntu@176.108.247.222"))
	Expect(res.Status).To(Equal(VmStateRunning))

	// The boot disk, created FIRST and separately: `POST /vms` must reference an
	// existing disk and answers HTTP 500 (not a validation error) for an empty list.
	Expect(f.createdDisks).To(HaveLen(1))
	Expect(f.createdDisks[0].Name).To(Equal("ruvm--staging-vm-host-0-disk"))
	Expect(f.createdDisks[0].Size).To(Equal(DefaultVmDiskGiB))
	Expect(f.createdDisks[0].DiskType).To(Equal("SSD"))
	Expect(f.createdDisks[0].ImageID).To(Equal(ubuntuImage().ID))
	Expect(f.indexOfCall(http.MethodPost, "/disks")).
		To(BeNumerically("<", f.indexOfCall(http.MethodPost, "/vms")))

	// The VM, and the whole of its boot contract: `image_metadata` slots, because
	// `user_data` is refused outright. The `name` slot is the LINUX LOGIN, not the
	// machine name — a confusion that produces a machine nobody can log in to.
	Expect(f.createdVms).To(HaveLen(1))
	created := f.createdVms[0]
	Expect(created.FlavorID).To(Equal("fl-2-4"))
	Expect(created.Zone).To(Equal("ru-central-1a"), "no zone configured resolves to the project's default AZ")
	Expect(created.Disks).To(Equal([]createVmDisk{{DiskID: "disk-1"}}))
	Expect(created.ImageMetadata).To(Equal(map[string]string{
		"hostname":   "ruvm--staging-vm-host-0",
		"name":       "ubuntu",
		"public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDEPLOY",
	}))
	// Trimmed: the key goes into an authorized_keys-shaped slot where a trailing
	// newline has become a second, invalid entry before.
	Expect(created.ImageMetadata["public_key"]).ToNot(HaveSuffix("\n"))

	// Serialised, the request must carry no networking key at all — every spelling is
	// refused with `extra_forbidden`, so sending one fails the create outright rather
	// than being ignored.
	body, err := json.Marshal(created)
	Expect(err).ToNot(HaveOccurred())
	for _, forbidden := range []string{"user_data", "subnet_id", "security_group_ids", "interfaces", "nics", "networks"} {
		Expect(string(body)).ToNot(ContainSubstring(forbidden), "`POST /vms` refuses %q", forbidden)
	}

	// The floating IP: allocated, polled to `available` (the attach 422s while it is
	// still provisioning), then attached to the VM's PRIMARY interface.
	Expect(f.createdFips).To(HaveLen(1))
	Expect(f.createdFips[0].Name).To(Equal("ruvm--staging-vm-host-0-ip"))
	Expect(f.callsTo(http.MethodPut, "/floating-ips/fip-1")).To(Equal(1))
	Expect(f.indexOfCall(http.MethodPost, "/vms")).
		To(BeNumerically("<", f.indexOfCall(http.MethodPut, "/floating-ips/fip-1")))

	// The ports, opened on the group the VM's interface ACTUALLY joined rather than on
	// "the zone's default group" — those are not always the same object, and a rule
	// added to the wrong one is a deploy that reports success and a port that stays
	// shut.
	Expect(f.createdRules).To(HaveLen(2))
	Expect(lo.Map(f.createdRules, func(r SecurityGroupRule, _ int) string {
		return r.IPProtocol + " " + r.PortRange + " from " + r.RemoteIPPrefix
	})).To(ConsistOf("tcp 22:22 from 0.0.0.0/0", "udp 443:443 from 0.0.0.0/0"))
	for _, r := range f.createdRules {
		Expect(r.Direction).To(Equal("ingress"))
		Expect(r.EtherType).To(Equal("IPv4"))
		Expect(r.Description).To(HavePrefix("simple-container "))
	}
}

// Find-or-create at every step, because this runs under a pulumi-command resource: a
// retried deploy, or one that follows a create which failed between the disk and the
// VM, must converge rather than pile up a second disk.
func TestApplyVmIsIdempotent(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			f.vms[i].State = VmStateRunning
		}
	}
	c := newComputeClient(t, f)

	first, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())

	second, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())

	Expect(second.VmID).To(Equal(first.VmID))
	Expect(second.Address).To(Equal(first.Address))
	Expect(f.createdVms).To(HaveLen(1), "the second apply must find the machine, not make another")
	Expect(f.createdDisks).To(HaveLen(1))
	Expect(f.createdFips).To(HaveLen(1))
	// The attach is not repeated either: the rule and the attach are both checked
	// against observed state first.
	Expect(f.callsTo(http.MethodPut, "/floating-ips/fip-1")).To(Equal(1))
	Expect(f.createdRules).To(HaveLen(2), "a rule already present must not be added twice")
}

// The refusals. Each of these is a config an operator could plausibly write, and each
// would otherwise fail partway through a multi-object create — leaving a disk behind.
func TestApplyVmRefusals(t *testing.T) {
	RegisterTestingT(t)

	ctx := context.Background()

	// A chosen subnet or security group. Refused with the measured reason and the two
	// ways out, because "not supported" alone leaves an operator with a VPC and
	// nothing to do.
	f := newComputeFake()
	c := newComputeClient(t, f)
	spec := smokeVmSpec()
	spec.SubnetID = "subnet-1"
	_, err := c.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("DEFAULT subnet"))
	Expect(f.createdDisks).To(BeEmpty(), "nothing may be created before the refusal")

	spec = smokeVmSpec()
	spec.SecurityGroupIDs = []string{"sg-default"}
	_, err = c.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("security_group_ids"))

	// No public key. There is no other way into the machine on this cloud — no
	// user-data, no serial console — so a VM without one boots unreachable and bills.
	spec = smokeVmSpec()
	spec.PublicKey = ""
	_, err = c.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("image_metadata.public_key"))

	spec = smokeVmSpec()
	spec.SSHUser = ""
	_, err = c.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("sshUser"))

	// Neither a flavor nor a shape.
	spec = smokeVmSpec()
	spec.Cores, spec.MemoryGiB = 0, 0
	_, err = c.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`cores` and `memoryGiB`"))

	// A disk below the image's own minimum. Caught here rather than by the disk
	// create, whose error reads as a storage problem and sends the operator looking at
	// quota.
	spec = smokeVmSpec()
	spec.DiskGiB = 8
	_, err = c.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("at least 11 GiB"))

	// An image whose boot contract has no slot for an SSH key. Refused rather than
	// created: the result would be a running, billed, unreachable machine.
	f2 := newComputeFake()
	f2.images = []Image{{ID: "img-bare", Name: "alpine-3.20", Metadata: []ImageMetadataField{{Name: "hostname"}}}}
	c2 := newComputeClient(t, f2)
	spec = smokeVmSpec()
	spec.Image = "alpine-3.20"
	_, err = c2.ApplyVm(ctx, vmRequest(spec), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("metadata slot"))
	Expect(f2.createdDisks).To(BeEmpty())

	// No spec at all, and bad credentials: the two shapes a malformed stdin document
	// takes.
	_, err = c.ApplyVm(ctx, Request{Credentials: Credentials{KeyID: "k", Secret: "s", ProjectID: "p"}, Name: "x"}, nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`vmSpec` is required"))
}

// A machine already holding a floating IP that is not SC's is reported, not
// hijacked: detaching somebody else's address would take down whatever was answering
// on it.
func TestApplyVmRefusesAForeignFloatingIP(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.vms = []Vm{{
		ID: "vm-1", Name: "ruvm--staging-vm-host-0", State: VmStateRunning,
		Zone: zoneRef{Name: "ru-central-1a"}, Flavor: f.flavors[0],
		Interfaces: []VmInterface{{
			ID: "iface-1", Primary: true, IPAddress: "10.0.0.5",
			FloatingIP:     &namedRef{ID: "fip-other", Name: "someone-elses", IPAddress: "203.0.113.9"},
			SecurityGroups: []namedRef{{ID: "sg-default", Name: "default"}},
		}},
	}}
	c := newComputeClient(t, f)

	_, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("someone-elses"))
	Expect(err.Error()).To(ContainSubstring("detach it in the console"))
}

// Destroy order is not negotiable: the VM holds the interface the floating IP is
// attached to, and the disk is `in_use` until the VM is gone. Deleting the disk first
// fails; deleting the floating IP first strands it.
func TestDestroyVmOrderAndIdempotence(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			f.vms[i].State = VmStateRunning
		}
	}
	c := newComputeClient(t, f)
	ctx := context.Background()

	_, err := c.ApplyVm(ctx, vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())

	res, err := c.DestroyVm(ctx, vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(res.Name).To(Equal("ruvm--staging-vm-host-0"))

	Expect(f.deletedVms).To(Equal([]string{"vm-1"}))
	Expect(f.deletedFips).To(Equal([]string{"fip-1"}))
	Expect(f.deletedDisks).To(Equal([]string{"disk-1"}))
	Expect(f.indexOfCall(http.MethodDelete, "/vms/vm-1")).
		To(BeNumerically("<", f.indexOfCall(http.MethodDelete, "/floating-ips/fip-1")))
	Expect(f.indexOfCall(http.MethodDelete, "/floating-ips/fip-1")).
		To(BeNumerically("<", f.indexOfCall(http.MethodDelete, "/disks/disk-1")))

	// Absence is success at every step: a destroy that runs twice, or one that follows
	// a create which failed halfway, must still converge.
	before := len(f.deletedVms) + len(f.deletedFips) + len(f.deletedDisks)
	_, err = c.DestroyVm(ctx, vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(len(f.deletedVms) + len(f.deletedFips) + len(f.deletedDisks)).To(Equal(before))

	// Destroy needs only a name — the spec is gone by the time a stack is torn down.
	_, err = c.DestroyVm(ctx, Request{
		Credentials: Credentials{KeyID: "k", Secret: "s", ProjectID: "project-1"},
		Name:        "ruvm--staging-vm-host-0",
	}, nil)
	Expect(err).ToNot(HaveOccurred())

	_, err = c.DestroyVm(ctx, Request{Credentials: Credentials{KeyID: "k", Secret: "s", ProjectID: "project-1"}}, nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("`name` is required"))
}

// Node-to-node ports are opened from the VM's own SUBNET, never from the internet.
//
// The three k3s ports in this spec hand out cluster-admin-equivalent access to
// whoever can reach them — the kubelet execs into any pod on the node, and etcd IS
// the cluster's state — so the difference between these two sources is the
// difference between a cluster and a cluster anybody owns. In the console the two
// rules look alike.
func TestApplyVmScopesNodeToNodePortsToTheSubnet(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.subnets[0].SubnetAddress = "10.0.0.0/24"
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			f.vms[i].State = VmStateRunning
		}
	}
	c := newComputeClient(t, f)

	spec := smokeVmSpec()
	name := spec.Name
	spec.IngressRules = append(spec.IngressRules,
		IngressRule{Protocol: "tcp", Port: 6443, Description: RuleDescription(name, "k3s-api")},
		IngressRule{Protocol: "udp", Port: 8472, Internal: true, Description: RuleDescription(name, "k3s-flannel-vxlan")},
		IngressRule{Protocol: "tcp", Port: 10250, Internal: true, Description: RuleDescription(name, "k3s-kubelet")},
		IngressRule{
			Protocol: "tcp", PortRange: "2379:2380", Internal: true,
			Description: RuleDescription(name, "k3s-etcd"),
		})

	_, err := c.ApplyVm(context.Background(), vmRequest(spec), nil)
	Expect(err).ToNot(HaveOccurred())

	Expect(lo.Map(f.createdRules, func(r SecurityGroupRule, _ int) string {
		return r.IPProtocol + " " + r.PortRange + " from " + r.RemoteIPPrefix
	})).To(ConsistOf(
		"tcp 22:22 from 0.0.0.0/0",
		"udp 443:443 from 0.0.0.0/0",
		"tcp 6443:6443 from 0.0.0.0/0",
		"udp 8472:8472 from 10.0.0.0/24",
		"tcp 10250:10250 from 10.0.0.0/24",
		"tcp 2379:2380 from 10.0.0.0/24",
	))
}

// And when the subnet cannot be resolved, the port is left SHUT and said out loud.
//
// The tempting fallback is 0.0.0.0/0 — it makes the cluster form, the smoke pass and
// nothing look wrong. It also publishes the kubelet and etcd. A port an operator has
// to open by hand is recoverable; this is not.
func TestApplyVmSkipsInternalPortsWhenTheSubnetIsUnknown(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	// No `subnet_address` on the subnet the interface lands on, which is what a
	// response from an older API version or a differently-shaped VPC looks like.
	f.subnets[0].SubnetAddress = ""
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			f.vms[i].State = VmStateRunning
		}
	}
	c := newComputeClient(t, f)

	spec := smokeVmSpec()
	spec.IngressRules = append(spec.IngressRules, IngressRule{
		Protocol: "tcp", Port: 10250, Internal: true,
		Description: RuleDescription(spec.Name, "k3s-kubelet"),
	})

	var said []string
	_, err := c.ApplyVm(context.Background(), vmRequest(spec), func(msg string, args ...any) {
		said = append(said, fmt.Sprintf(msg, args...))
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(lo.Map(f.createdRules, func(r SecurityGroupRule, _ int) string { return r.PortRange })).
		ToNot(ContainElement("10250:10250"))
	Expect(strings.Join(said, "\n")).To(ContainSubstring("node-to-node port"))
	Expect(strings.Join(said, "\n")).To(ContainSubstring("will not open it to the internet instead"))
}

// A destroy closes the ports IT opened and leaves every other machine's alone.
//
// The machine joins the project's SHARED default group, so the sweep's scope is the
// whole risk here: a sweep of everything SC had written would take the ports of
// every other SC machine in the project down with it — one stack's destroy breaking
// another, with nothing in either stack's state to explain it.
func TestDestroyVmClosesOnlyItsOwnRules(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			f.vms[i].State = VmStateRunning
		}
	}
	f.groups[0].Rules = append(f.groups[0].Rules,
		// Another SC machine in the same project, sharing this group. On a port this
		// VM does not ask for — the shared-port case is below, and it behaves
		// differently enough to be worth keeping apart.
		SecurityGroupRule{
			ID: "rule-other", Direction: "ingress", IPProtocol: "tcp", PortRange: "9090:9090",
			RemoteIPPrefix: "0.0.0.0/0", Description: RuleDescription("other-vm", "metrics"),
		},
		// A machine whose name merely STARTS with this one's. The discriminator ends
		// in the description's separator for exactly this case: a prefix match on the
		// name alone would sweep `…-host-02` while destroying `…-host-0`.
		SecurityGroupRule{
			ID: "rule-sibling", Direction: "ingress", IPProtocol: "tcp", PortRange: "80:80",
			RemoteIPPrefix: "0.0.0.0/0", Description: RuleDescription("ruvm--staging-vm-host-02", "http"),
		},
		// Somebody's hand-written rule, which SC did not create and must not remove.
		SecurityGroupRule{
			ID: "rule-manual", Direction: "ingress", IPProtocol: "tcp", PortRange: "5432:5432",
			RemoteIPPrefix: "0.0.0.0/0", Description: "postgres from the office",
		})
	c := newComputeClient(t, f)
	ctx := context.Background()

	_, err := c.ApplyVm(ctx, vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())
	own := lo.Map(f.createdRules, func(r SecurityGroupRule, _ int) string { return r.Description })
	Expect(own).To(HaveLen(2))

	_, err = c.DestroyVm(ctx, vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())

	// The two rules the apply created, by the ids the fake assigned them, and
	// nothing else.
	Expect(f.deletedRules).To(ConsistOf("rule-1", "rule-2"))
	surviving := lo.Map(f.groups[0].Rules, func(r SecurityGroupRule, _ int) string { return r.ID })
	Expect(surviving).To(ContainElements("rule-other", "rule-sibling", "rule-manual"))

	// Collected BEFORE the VM is deleted, which is the only time they are knowable:
	// afterwards nothing says which group SC opened ports in.
	Expect(f.indexOfCall(http.MethodDelete, "/security-groups/sg-default/rules/rule-1")).
		To(BeNumerically(">", f.indexOfCall(http.MethodGet, "/vms")))

	// And a second destroy is quiet: the rules are gone, 404 is success, nothing is
	// deleted twice.
	before := len(f.deletedRules)
	_, err = c.DestroyVm(ctx, vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(f.deletedRules).To(HaveLen(before))
}

// A re-apply opens nothing: the rules are already there. This is the test the
// stripped `rules` field in the group-list route exists for — a client reading the
// group's inlined (empty) rules would re-create all of them here, every deploy,
// until the group hit its rule cap.
func TestApplyVmDoesNotReopenExistingPorts(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.subnets[0].SubnetAddress = "10.0.0.0/24"
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			f.vms[i].State = VmStateRunning
		}
	}
	c := newComputeClient(t, f)

	spec := smokeVmSpec()
	spec.IngressRules = append(spec.IngressRules, IngressRule{
		Protocol: "tcp", Port: 10250, Internal: true,
		Description: RuleDescription(spec.Name, "k3s-kubelet"),
	})

	_, err := c.ApplyVm(context.Background(), vmRequest(spec), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(f.createdRules).To(HaveLen(3))

	_, err = c.ApplyVm(context.Background(), vmRequest(spec), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(f.createdRules).To(HaveLen(3), "the second converge opens nothing")
}

// The default group carries an `any`/`any` ingress rule scoped to the GROUP, and that
// does not cover a rule from 0.0.0.0/0: it permits traffic from group members only.
// Reading it as coverage would make SC skip every port it needs to open and report a
// successful deploy onto an unreachable machine.
func TestHasIngressRule(t *testing.T) {
	RegisterTestingT(t)

	wanted := SecurityGroupRule{
		Direction: "ingress", EtherType: "IPv4", IPProtocol: "tcp", PortRange: "22:22", RemoteIPPrefix: "0.0.0.0/0",
	}

	groupScopedAny := SecurityGroup{Rules: []SecurityGroupRule{
		{Direction: "ingress", IPProtocol: "any", PortRange: "any", RemoteIPPrefix: ""},
	}}
	Expect(hasIngressRule(groupScopedAny, wanted)).To(BeFalse())

	// An `any`/`any` from everywhere does cover it.
	Expect(hasIngressRule(SecurityGroup{Rules: []SecurityGroupRule{
		{Direction: "ingress", IPProtocol: "any", PortRange: "any", RemoteIPPrefix: "0.0.0.0/0"},
	}}, wanted)).To(BeTrue())

	Expect(hasIngressRule(SecurityGroup{Rules: []SecurityGroupRule{wanted}}, wanted)).To(BeTrue())

	// Direction and port are both part of the match: an egress rule on the same port,
	// or ingress on a different one, is not this rule.
	Expect(hasIngressRule(SecurityGroup{Rules: []SecurityGroupRule{
		{Direction: "egress", IPProtocol: "tcp", PortRange: "22:22", RemoteIPPrefix: "0.0.0.0/0"},
	}}, wanted)).To(BeFalse())
	Expect(hasIngressRule(SecurityGroup{Rules: []SecurityGroupRule{
		{Direction: "ingress", IPProtocol: "tcp", PortRange: "80:80", RemoteIPPrefix: "0.0.0.0/0"},
	}}, wanted)).To(BeFalse())
	// And the protocol: UDP 443 and TCP 443 are two rules, which is the pairing Caddy
	// needs for HTTP/3.
	Expect(hasIngressRule(SecurityGroup{Rules: []SecurityGroupRule{
		{Direction: "ingress", IPProtocol: "udp", PortRange: "22:22", RemoteIPPrefix: "0.0.0.0/0"},
	}}, wanted)).To(BeFalse())
}

// A VM that reaches a state no amount of waiting fixes must fail the deploy rather
// than sit until the timeout — but an UNKNOWN state must keep polling: the API has no
// documented state machine, and reading a new transient state as a failure would break
// deploys on Cloud.ru's schedule.
func TestTerminalVmFailure(t *testing.T) {
	RegisterTestingT(t)

	for _, state := range []string{"error", "ERROR", "failed", "deleted", "deleting"} {
		Expect(isTerminalVmFailure(state)).To(BeTrue(), "state %q", state)
	}
	for _, state := range []string{"creating", "starting", "running", "resizing", "", "some-new-state"} {
		Expect(isTerminalVmFailure(state)).To(BeFalse(), "state %q", state)
	}
}

func TestApplyVmFailsOnTerminalState(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			f.vms[i].State = "error"
		}
	}
	c := newComputeClient(t, f)

	_, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("will not become \"running\""))
}

// The second bug the live smoke found: `POST /disks` answers immediately with the
// disk in `creating`, and `POST /vms` refuses to attach one in that state with
// `422 disk_can_not_be_attached_from_current_state` — a message that names neither
// the disk nor the state it wanted. The converge has to read the disk back.
func TestApplyVmWaitsForTheBootDiskToBecomeAttachable(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.onVmGet = func(f *fakeCompute) {
		for i := range f.vms {
			if f.vms[i].State == "creating" {
				f.vms[i].State = VmStateRunning
			}
		}
	}
	c := newComputeClient(t, f)

	res, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).ToNot(HaveOccurred())
	Expect(res.VmID).To(Equal("vm-1"))

	// Read back at least once, after the create and before the VM.
	Expect(f.callsTo(http.MethodGet, "/disks/disk-1")).To(BeNumerically(">=", 1))
	Expect(f.indexOfCall(http.MethodPost, "/disks")).
		To(BeNumerically("<", f.indexOfCall(http.MethodGet, "/disks/disk-1")))
	Expect(f.indexOfCall(http.MethodGet, "/disks/disk-1")).
		To(BeNumerically("<", f.indexOfCall(http.MethodPost, "/vms")))
}

func TestApplyVmRefusesADiskHeldByAnotherVm(t *testing.T) {
	RegisterTestingT(t)

	// A disk under the name SC derives for this VM, already attached elsewhere.
	// Waiting cannot help and attaching it would be destructive, so this is a
	// refusal and not a poll.
	f := newComputeFake()
	f.disks = []Disk{{
		ID: "disk-1", Name: "ruvm--staging-vm-host-0-disk", Size: DefaultVmDiskGiB,
		State: DiskStateInUse, Bootable: true,
		Zone: zoneRef{Name: "ru-central-1a"},
		Vms:  []namedRef{{ID: "vm-someone-else", Name: "other-stack-host-0"}},
	}}
	c := newComputeClient(t, f)

	_, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("already attached to other-stack-host-0"))
	Expect(err.Error()).To(ContainSubstring("rename one of them"))
	Expect(f.createdVms).To(BeEmpty())
	Expect(f.createdDisks).To(BeEmpty(), "the existing disk is reused, never duplicated")
}

func TestApplyVmRefusesATerminallyFailedDisk(t *testing.T) {
	RegisterTestingT(t)

	f := newComputeFake()
	f.disks = []Disk{{
		ID: "disk-1", Name: "ruvm--staging-vm-host-0-disk", Size: DefaultVmDiskGiB,
		State: "error", Bootable: true, Zone: zoneRef{Name: "ru-central-1a"},
	}}
	c := newComputeClient(t, f)

	_, err := c.ApplyVm(context.Background(), vmRequest(smokeVmSpec()), nil)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("entered state \"error\""))
	Expect(f.createdVms).To(BeEmpty())

	// An UNKNOWN state is not terminal — it keeps polling, because the API has no
	// documented state machine and a new transient state must not fail a deploy.
	Expect(isTerminalDiskFailure("provisioning")).To(BeFalse())
	Expect(isTerminalDiskFailure("DELETING")).To(BeTrue(), "the check is case-insensitive")
}
