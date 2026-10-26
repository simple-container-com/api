// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// The Evolution Compute API, measured against the operator's own project on
// 2026-10-24. There is no published OpenAPI document for it (`/api/v1/docs`
// serves a six-path "SVC Public API" that does not include any of this), so
// every shape below came off a live call, and the FastAPI 422 body — which names
// the offending field and its pattern — was the schema.
//
// The five asymmetries worth knowing before changing anything here, each of them
// paid for:
//
//  1. Project scope is `project_id` (snake_case) in the QUERY on every verb —
//     except `/security-groups/{id}/rules`, which rejects it outright (422
//     `extra_forbidden`). POST bodies ALSO carry `project_id`.
//  2. `POST /vms` takes a JSON **list**; `POST /disks` and `POST /floating-ips`
//     take a single object (a list body on floating-ips answers 500).
//  3. A VM cannot be created with an empty `disks` array — that answers **500**,
//     not a 422. The boot disk is a separate object, created first, referenced by
//     id.
//  4. Attaching a floating IP is `PUT /floating-ips/{id}` with
//     `{"interface_id": …}`. `PATCH` and `POST …/attach` do not exist, and the PUT
//     422s until the address has reached `available`.
//  5. Every name — vm, disk, floating ip, and the `disk_type_name` /
//     `availability_zone_name` *references* — must match
//     `^[a-zA-Z][a-zA-Z0-9.\-_]*$`. It must START WITH A LETTER, which SC's own
//     `<stack>--<env>` names satisfy only by luck; see toComputeName.
//  6. A VM's networking cannot be chosen at create time: `subnet_id`,
//     `security_group_ids`, `interfaces`, `networks`, `nics` and `user_data` are
//     all `extra_forbidden`. The VM lands in its AZ's default subnet and default
//     security group; a different subnet needs a second `POST /interfaces` after
//     the fact, which this driver does not do yet (it refuses the config
//     instead).
//
// And the one that silently corrupts a lookup rather than failing: **list
// endpoints default to `limit=50`** and report the real count in `total`. The
// flavor catalogue has 78 entries, so an unpaginated "find me a 32-core flavor"
// finds nothing and looks like a capability gap. Everything here goes through
// listPaged.

const (
	// computeAPIPath prefixes every compute route.
	computeAPIPath = "/api/v1"

	// computePageLimit is the page size used for every list call. The API caps
	// nothing visibly at 1000; 500 is one request for every collection that
	// exists today (the largest, flavors, is 78).
	computePageLimit = 500

	// computePageLimitGuard bounds the pagination loop. A server that keeps
	// reporting a larger `total` than it returns would otherwise spin forever.
	computePageLimitGuard = 50

	// VmStateRunning is the only VM state SC treats as usable.
	VmStateRunning = "running"
	// FloatingIPStateAvailable is the state a floating IP must reach before it
	// can be attached to an interface.
	FloatingIPStateAvailable = "available"
	// FloatingIPStateInUse is a floating IP already attached.
	FloatingIPStateInUse = "in_use"

	// DiskStateAvailable is the only state a disk can be attached FROM. A disk
	// answers `POST /disks` immediately and spends a few seconds in `creating`;
	// attaching one in that window answers `422 disk_can_not_be_attached_from_
	// current_state` (live-caught 2026-10-24 on the second SC-created VM, where
	// `POST /vms` followed `POST /disks` with no wait in between).
	DiskStateAvailable = "available"
	// DiskStateInUse is a disk already attached to a VM.
	DiskStateInUse = "in_use"
)

// ComputeNameMaxLen is the longest name any compute object accepts (`too_long`
// past it). SC's own names are stack+environment+role, which gets close.
const ComputeNameMaxLen = 64

// computeNameRegexp is the grammar every compute object name must match. Quoted
// from the 422 the API returns for a violation (`string_pattern_mismatch`).
var computeNameRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.\-_]*$`)

// toComputeName makes an SC resource name acceptable to the compute API.
//
// SC names are built from stack and environment (`ruvm--staging-vm`), which is
// already legal here — the compute API, unlike Artifact Registry, does not mind a
// doubled hyphen. What it does mind is a name that does not start with a letter,
// so a leading digit, dot or dash is prefixed rather than stripped: dropping it
// would let two differently-named resources collapse onto one VM.
func toComputeName(name string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(name))
	if out == "" {
		return ""
	}
	if !computeNameRegexp.MatchString(out) {
		out = "sc-" + out
	}
	if len(out) > ComputeNameMaxLen {
		out = strings.TrimRight(out[:ComputeNameMaxLen], ".-_")
	}
	return out
}

// derivedName appends a suffix to a VM's name for its satellite objects (boot
// disk, floating IP), keeping the result inside ComputeNameMaxLen by trimming
// the base rather than the suffix — the suffix is what makes the object
// identifiable in the console.
func derivedName(base, suffix string) string {
	if len(base)+len(suffix) > ComputeNameMaxLen {
		base = strings.TrimRight(base[:ComputeNameMaxLen-len(suffix)], ".-_")
	}
	return base + suffix
}

// ValidateComputeName reports why a name would be refused, before a call that
// would otherwise fail halfway through a multi-object create.
func ValidateComputeName(kind, name string) error {
	if name == "" {
		return errors.Errorf("cloud.ru %s name is empty", kind)
	}
	if !computeNameRegexp.MatchString(name) {
		return errors.Errorf("cloud.ru %s name %q is not accepted: it must match %s "+
			"(in particular it must START with a letter)", kind, name, computeNameRegexp.String())
	}
	return nil
}

// --- wire types -------------------------------------------------------------

// zoneRef / projectRef / namedRef are the embedded references the API answers
// with. Only the fields SC reads are declared; the API adds more freely and an
// exhaustive struct would be a maintenance cost with no payoff.
type zoneRef struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled,omitempty"`
}

type namedRef struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	State     string `json:"state,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	Primary   bool   `json:"primary,omitempty"`
}

// AvailabilityZone is one AZ. `/availability-zones` is the one compute
// collection that answers a bare JSON array rather than a paged envelope.
type AvailabilityZone struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name,omitempty"`
	Enabled   bool   `json:"enabled"`
	Default   bool   `json:"default"`
}

// Subnet is a subnet. There is no separate "network" object on this cloud —
// `/networks` is a 404 and the subnet carries the `vpc_id` — so a subnet is what
// a VM interface attaches to.
type Subnet struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	SubnetAddress  string  `json:"subnet_address,omitempty"`
	DefaultGateway string  `json:"default_gateway,omitempty"`
	Default        bool    `json:"default"`
	State          string  `json:"state,omitempty"`
	VpcID          string  `json:"vpc_id,omitempty"`
	Zone           zoneRef `json:"availability_zone"`
}

// SecurityGroupRule is one ingress or egress rule.
type SecurityGroupRule struct {
	ID             string `json:"id,omitempty"`
	Direction      string `json:"direction"`
	EtherType      string `json:"ether_type"`
	IPProtocol     string `json:"ip_protocol"`
	PortRange      string `json:"port_range"`
	RemoteIPPrefix string `json:"remote_ip_prefix,omitempty"`
	Description    string `json:"description,omitempty"`
	State          string `json:"state,omitempty"`
}

// SecurityGroup is a security group with its rules inlined.
type SecurityGroup struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Default bool                `json:"default"`
	State   string              `json:"state,omitempty"`
	Zone    zoneRef             `json:"availability_zone"`
	Rules   []SecurityGroupRule `json:"rules,omitempty"`
}

// flavorZone is a flavor's per-AZ availability. Note the key names differ from
// the embedded zoneRef used everywhere else.
type flavorZone struct {
	ZoneID   string `json:"availability_zone_id,omitempty"`
	ZoneName string `json:"availability_zone_name,omitempty"`
	Enabled  bool   `json:"enabled,omitempty"`
}

// Flavor is a machine size. `RAM` is in **whole GiB**, not MiB.
//
// Flavor names are not stable: the smoke VM of 2026-10-23 runs `low-2-4`, and by
// 2026-10-24 the catalogue offered `lowcost10-2-4` with the same 2 vCPU / 4 GiB
// and no `low-2-4` at all. So a flavor is resolved by cores+memory, never by a
// name or id baked into SC.
type Flavor struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Type  string       `json:"type,omitempty"`
	CPU   int          `json:"cpu"`
	RAM   int          `json:"ram"`
	GPU   int          `json:"gpu"`
	Zones []flavorZone `json:"availability_zones,omitempty"`
}

// availableIn reports whether the flavor can be used in the named AZ.
func (f Flavor) availableIn(zone string) bool {
	if zone == "" {
		return true
	}
	for _, z := range f.Zones {
		if z.ZoneName == zone && z.Enabled {
			return true
		}
	}
	return false
}

// Image is a bootable image. `Metadata` is what makes this cloud different from
// every other one SC drives: see VmSpec.
//
// MinDisk is in GiB and is enforced — Ubuntu 24.04 asks for 11, so a `size.disk`
// smaller than that fails the disk create, not the VM create, which makes the
// error read as a storage problem.
type Image struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	DisplayName string               `json:"display_name,omitempty"`
	ProjectID   string               `json:"project_id,omitempty"`
	Type        string               `json:"type,omitempty"`
	MinCPU      int                  `json:"min_cpu,omitempty"`
	MinRAM      int                  `json:"min_ram,omitempty"`
	MinDisk     int                  `json:"min_disk,omitempty"`
	Metadata    []ImageMetadataField `json:"image_metadata,omitempty"`
	Zones       []flavorZone         `json:"availability_zones,omitempty"`
}

// ImageMetadataField is one declared slot of the image's `image_metadata` dict.
//
// This is the whole of the boot-time contract on Cloud.ru Evolution. The image
// declares its slots and renders a FIXED Jinja cloud-init template from them;
// `POST /vms` refuses a `user_data` key outright (`extra_forbidden`, measured
// 2026-10-24). So SC cannot deliver arbitrary cloud-init here, which is why the
// driver above reports `CloudInit: false` and installs docker over SSH instead.
//
// `MandatoryGroup` is an OR-group: Ubuntu 24.04 puts `public_key` and
// `linux_password` both in `authentication`, meaning *one of them* is required.
type ImageMetadataField struct {
	Name           string `json:"name,omitempty"`
	FieldType      string `json:"field_type,omitempty"`
	DefaultValue   string `json:"default_value,omitempty"`
	MandatoryGroup string `json:"mandatory_group,omitempty"`
	Sensitive      bool   `json:"sensitive,omitempty"`
}

// supportsMetadata reports whether the image declares the named slot. Used to
// refuse an image whose boot contract has no place for an SSH key, rather than
// creating an unreachable VM.
func (i Image) supportsMetadata(name string) bool {
	for _, f := range i.Metadata {
		if f.Name == name {
			return true
		}
	}
	return false
}

// DiskType is a storage class. Today the project offers exactly one (`SSD`).
type DiskType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
}

// Disk is a block device. `Size` is GiB.
type Disk struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Size     int        `json:"size"`
	State    string     `json:"state,omitempty"`
	Bootable bool       `json:"bootable,omitempty"`
	Zone     zoneRef    `json:"availability_zone"`
	Image    namedRef   `json:"image"`
	DiskType namedRef   `json:"disk_type"`
	Vms      []namedRef `json:"vms,omitempty"`
}

// VmInterface is a NIC. The `ID` is what a floating-IP attach takes.
type VmInterface struct {
	ID             string     `json:"id"`
	Name           string     `json:"name,omitempty"`
	IPAddress      string     `json:"ip_address,omitempty"`
	Primary        bool       `json:"primary,omitempty"`
	State          string     `json:"state,omitempty"`
	Subnet         namedRef   `json:"subnet"`
	FloatingIP     *namedRef  `json:"floating_ip,omitempty"`
	SecurityGroups []namedRef `json:"security_groups,omitempty"`
}

// Vm is a virtual machine.
type Vm struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	State      string        `json:"state,omitempty"`
	ImageID    string        `json:"image_id,omitempty"`
	Zone       zoneRef       `json:"availability_zone"`
	Flavor     Flavor        `json:"flavor"`
	Disks      []namedRef    `json:"disks,omitempty"`
	Interfaces []VmInterface `json:"interfaces,omitempty"`
}

// primaryInterface returns the NIC a floating IP should be attached to.
func (v Vm) primaryInterface() *VmInterface {
	for i := range v.Interfaces {
		if v.Interfaces[i].Primary {
			return &v.Interfaces[i]
		}
	}
	if len(v.Interfaces) > 0 {
		return &v.Interfaces[0]
	}
	return nil
}

// PrivateAddress is the VM's in-subnet address.
func (v Vm) PrivateAddress() string {
	if iface := v.primaryInterface(); iface != nil {
		return iface.IPAddress
	}
	return ""
}

// FloatingIP is a public address, attachable to one interface.
type FloatingIP struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IPAddress string    `json:"ip_address,omitempty"`
	State     string    `json:"state,omitempty"`
	Zone      zoneRef   `json:"availability_zone"`
	Interface *namedRef `json:"interface,omitempty"`
	Vm        *namedRef `json:"vm,omitempty"`
}

// pagedEnvelope is the shape MOST compute list endpoints answer with. Not all of
// them: see decodeCollection.
type pagedEnvelope[T any] struct {
	Items  []T `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// --- list / get -------------------------------------------------------------

// decodeCollection reads a compute list answer that may be EITHER a paged
// envelope or a bare JSON array, deciding by the first non-whitespace byte.
//
// Two shapes rather than one is the API's own doing, and the split does not
// follow anything: `/availability-zones` and `/disk-types` answer bare arrays
// while `/flavors`, `/images`, `/subnets`, `/security-groups`, `/disks`, `/vms`
// and `/floating-ips` answer envelopes (all nine measured live, 2026-10-24).
//
// Sniffed rather than keyed off a hand-curated list of paths, because the first
// version of this client WAS such a list — built from `/availability-zones`
// being the only bare array anyone had noticed — and `/disk-types` broke the very
// first live VM create with `cannot unmarshal array into pagedEnvelope`. A tenth
// collection in either shape now costs nothing, and a server-side change from one
// shape to the other does not break a deploy.
//
// A bare array carries no `total`, which is exactly right: there is nothing to
// paginate, so the reported total is the length and the caller stops after one page.
func decodeCollection[T any](raw json.RawMessage) (pagedEnvelope[T], error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []T
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return pagedEnvelope[T]{}, errors.Wrapf(err, "failed to decode bare-array collection")
		}
		return pagedEnvelope[T]{Items: items, Total: len(items), Limit: len(items)}, nil
	}
	var envelope pagedEnvelope[T]
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return pagedEnvelope[T]{}, errors.Wrapf(err, "failed to decode paged collection")
	}
	return envelope, nil
}

// listPaged reads a whole compute collection, following `offset` until `total`
// is covered. Never call a compute list endpoint without it: the default page is
// 50 items and the API does not signal truncation anywhere but `total`.
func listPaged[T any](ctx context.Context, c *Client, path string, extra url.Values) ([]T, error) {
	var out []T
	offset := 0
	for page := 0; page < computePageLimitGuard; page++ {
		query := c.computeQuery()
		for k, vs := range extra {
			query[k] = vs
		}
		query.Set("limit", fmt.Sprintf("%d", computePageLimit))
		query.Set("offset", fmt.Sprintf("%d", offset))

		var raw json.RawMessage
		if err := c.request(ctx, http.MethodGet, c.computeURL, computeAPIPath+path, query, nil, &raw); err != nil {
			return nil, err
		}
		envelope, err := decodeCollection[T](raw)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode cloud.ru compute %s", path)
		}
		out = append(out, envelope.Items...)
		offset += len(envelope.Items)
		if len(envelope.Items) == 0 || offset >= envelope.Total {
			return out, nil
		}
	}
	return out, errors.Errorf("cloud.ru compute %s did not finish paginating after %d pages (%d items read)",
		path, computePageLimitGuard, len(out))
}

// ListAvailabilityZones lists the project's AZs.
//
// Goes through listPaged like every other collection even though this one answers
// a bare array today: decodeCollection takes either shape, so there is no longer a
// reason for this to be a special case — and the special case is what made
// `/disk-types` a surprise.
func (c *Client) ListAvailabilityZones(ctx context.Context) ([]AvailabilityZone, error) {
	return listPaged[AvailabilityZone](ctx, c, "/availability-zones", nil)
}

// DefaultZone is the AZ to place a VM in when the config names none.
//
// Resolved from where a DEFAULT SUBNET exists, deliberately NOT from the `default`
// flag on `/availability-zones` — the two disagree, and the AZ flag is the useless
// one for this purpose. Measured on the operator's project, 2026-10-24: all three
// AZs are enabled, `ru.AZ-1` is flagged `default: true`, and the project's only
// subnet is `Default_ru.AZ-3` in `ru.AZ-3`. A create in the flagged-default AZ
// answers `422 default_subnet_not_found`.
//
// A subnet is created per-AZ when the operator first uses that AZ in the console,
// so "the AZs with a default subnet" is really "the AZs this project has been set
// up in" — which is exactly the placement question, and nothing else answers it.
func (c *Client) DefaultZone(ctx context.Context) (string, error) {
	placeable, err := c.placeableZones(ctx)
	if err != nil {
		return "", err
	}
	if len(placeable) == 0 {
		return "", errors.Errorf("cloud.ru project %s has no availability zone with a default subnet, so a VM cannot be "+
			"placed anywhere: `POST /vms` takes no `subnet_id` and lands the machine in its zone's DEFAULT subnet. Create a "+
			"network in the Cloud.ru console once (any zone), then retry", c.creds.ProjectID)
	}
	// Among the placeable zones prefer the one the project itself calls default, so
	// that a project set up in several AZs gets a stable, unsurprising answer.
	zones, err := c.ListAvailabilityZones(ctx)
	if err != nil {
		return "", err
	}
	for _, z := range zones {
		if z.Enabled && z.Default && lo.Contains(placeable, z.Name) {
			return z.Name, nil
		}
	}
	return placeable[0], nil
}

// placeableZones are the AZs holding a default subnet, sorted, so the chosen zone
// does not depend on the API's listing order.
func (c *Client) placeableZones(ctx context.Context) ([]string, error) {
	subnets, err := c.ListSubnets(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range subnets {
		if s.Default && s.Zone.Name != "" {
			out = append(out, s.Zone.Name)
		}
	}
	out = lo.Uniq(out)
	sort.Strings(out)
	return out, nil
}

// AssertZonePlaceable refuses a zone with no default subnet BEFORE anything is
// created.
//
// Without it the refusal arrives from `POST /vms`, which is after the boot disk
// exists — so a mis-zoned config leaves a 30 GiB orphan disk behind on every
// attempt, and the error (`default_subnet_not_found`) names neither the zone it
// looked in nor the zones that would have worked. Live-caught 2026-10-24 on the
// first SC-created VM, which orphaned exactly such a disk.
func (c *Client) AssertZonePlaceable(ctx context.Context, zone string) error {
	placeable, err := c.placeableZones(ctx)
	if err != nil {
		return err
	}
	if lo.Contains(placeable, zone) {
		return nil
	}
	if len(placeable) == 0 {
		return errors.Errorf("cloud.ru availability zone %q has no default subnet, and neither does any other zone in "+
			"project %s: `POST /vms` takes no `subnet_id` and lands the machine in its zone's DEFAULT subnet. Create a "+
			"network in the Cloud.ru console once, then retry", zone, c.creds.ProjectID)
	}
	return errors.Errorf("cloud.ru availability zone %q has no default subnet, so a VM cannot be placed there "+
		"(`POST /vms` takes no `subnet_id` and uses the zone's DEFAULT subnet, answering `default_subnet_not_found` "+
		"otherwise). Zones in project %s that do have one: %s. Set `zones: [%s]` on the vm resource, or create a network "+
		"in %s in the Cloud.ru console", zone, c.creds.ProjectID, strings.Join(placeable, ", "), placeable[0], zone)
}

func (c *Client) ListSubnets(ctx context.Context) ([]Subnet, error) {
	return listPaged[Subnet](ctx, c, "/subnets", nil)
}

func (c *Client) ListSecurityGroups(ctx context.Context) ([]SecurityGroup, error) {
	return listPaged[SecurityGroup](ctx, c, "/security-groups", nil)
}

func (c *Client) ListFlavors(ctx context.Context) ([]Flavor, error) {
	return listPaged[Flavor](ctx, c, "/flavors", nil)
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	return listPaged[Image](ctx, c, "/images", nil)
}

func (c *Client) ListDiskTypes(ctx context.Context) ([]DiskType, error) {
	return listPaged[DiskType](ctx, c, "/disk-types", nil)
}

func (c *Client) ListDisks(ctx context.Context) ([]Disk, error) {
	return listPaged[Disk](ctx, c, "/disks", nil)
}

func (c *Client) ListVms(ctx context.Context) ([]Vm, error) {
	return listPaged[Vm](ctx, c, "/vms", nil)
}

func (c *Client) ListFloatingIPs(ctx context.Context) ([]FloatingIP, error) {
	return listPaged[FloatingIP](ctx, c, "/floating-ips", nil)
}

// GetVm reads one VM, including its interfaces (which the list endpoint also
// returns, but a freshly-created VM is read back by id).
func (c *Client) GetVm(ctx context.Context, id string) (*Vm, error) {
	var vm Vm
	if err := c.request(ctx, http.MethodGet, c.computeURL, computeAPIPath+"/vms/"+url.PathEscape(id),
		c.computeQuery(), nil, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

// GetDisk reads one disk. `GET /disks/{id}` tolerates the `project_id` query
// being present or absent (both measured 2026-10-24); it is sent for consistency
// with every other compute GET.
func (c *Client) GetDisk(ctx context.Context, id string) (*Disk, error) {
	var disk Disk
	if err := c.request(ctx, http.MethodGet, c.computeURL, computeAPIPath+"/disks/"+url.PathEscape(id),
		c.computeQuery(), nil, &disk); err != nil {
		return nil, err
	}
	return &disk, nil
}

// GetFloatingIP reads one floating IP.
func (c *Client) GetFloatingIP(ctx context.Context, id string) (*FloatingIP, error) {
	var fip FloatingIP
	if err := c.request(ctx, http.MethodGet, c.computeURL, computeAPIPath+"/floating-ips/"+url.PathEscape(id),
		c.computeQuery(), nil, &fip); err != nil {
		return nil, err
	}
	return &fip, nil
}

// FindVm looks a VM up by name. There is no server-side name filter, so this is
// a full (paginated) list plus a client-side match.
func (c *Client) FindVm(ctx context.Context, name string) (*Vm, bool, error) {
	vms, err := c.ListVms(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range vms {
		if vms[i].Name == name {
			return &vms[i], true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) FindDisk(ctx context.Context, name string) (*Disk, bool, error) {
	disks, err := c.ListDisks(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range disks {
		if disks[i].Name == name {
			return &disks[i], true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) FindFloatingIP(ctx context.Context, name string) (*FloatingIP, bool, error) {
	fips, err := c.ListFloatingIPs(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range fips {
		if fips[i].Name == name {
			return &fips[i], true, nil
		}
	}
	return nil, false, nil
}

// FindFlavor resolves cores+memory to the smallest flavor that satisfies both in
// the given zone, excluding GPU flavors (which cost an order of magnitude more
// and are never what a `size:` without a GPU field asked for).
func (c *Client) FindFlavor(ctx context.Context, zone string, cores, memoryGiB int) (*Flavor, error) {
	flavors, err := c.ListFlavors(ctx)
	if err != nil {
		return nil, err
	}
	var candidates []Flavor
	for _, f := range flavors {
		if f.GPU > 0 || f.CPU < cores || f.RAM < memoryGiB || !f.availableIn(zone) {
			continue
		}
		candidates = append(candidates, f)
	}
	if len(candidates) == 0 {
		return nil, errors.Errorf("cloud.ru has no non-GPU flavor with at least %d vCPU and %d GiB RAM in zone %q "+
			"(read %d flavors)", cores, memoryGiB, zone, len(flavors))
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].CPU != candidates[j].CPU {
			return candidates[i].CPU < candidates[j].CPU
		}
		if candidates[i].RAM != candidates[j].RAM {
			return candidates[i].RAM < candidates[j].RAM
		}
		return candidates[i].Name < candidates[j].Name
	})
	picked := candidates[0]
	return &picked, nil
}

// FindFlavorByName resolves an explicitly-configured flavor name.
func (c *Client) FindFlavorByName(ctx context.Context, zone, name string) (*Flavor, error) {
	flavors, err := c.ListFlavors(ctx)
	if err != nil {
		return nil, err
	}
	for i := range flavors {
		if flavors[i].Name != name {
			continue
		}
		if !flavors[i].availableIn(zone) {
			return nil, errors.Errorf("cloud.ru flavor %q exists but is not available in zone %q", name, zone)
		}
		return &flavors[i], nil
	}
	return nil, errors.Errorf("cloud.ru has no flavor named %q (flavor names are NOT stable on this cloud — "+
		"prefer `size: {cores, memory}` so SC resolves one)", name)
}

// cloudruImageNames maps SC's portable image aliases onto this cloud's catalogue
// names. Mapped rather than passed through, because `ubuntu-2404-lts` (SC's
// default, and the spelling the other VM drivers accept) is not a name Cloud.ru
// knows — it calls the same image `Ubuntu-24.04`.
var cloudruEvolutionImageAliases = map[string]string{
	"ubuntu-2404-lts": "Ubuntu-24.04",
	"ubuntu-2204-lts": "ubuntu-22.04",
	"ubuntu-24.04":    "Ubuntu-24.04",
	"ubuntu-22.04":    "ubuntu-22.04",
}

// FindImage resolves an image by UUID, by catalogue name, by display name, or by
// one of SC's portable aliases.
func (c *Client) FindImage(ctx context.Context, name string) (*Image, error) {
	if name == "" {
		return nil, errors.Errorf("cloud.ru image is required")
	}
	wanted := name
	if alias, ok := cloudruEvolutionImageAliases[strings.ToLower(name)]; ok {
		wanted = alias
	}
	images, err := c.ListImages(ctx)
	if err != nil {
		return nil, err
	}
	for i := range images {
		if images[i].ID == name {
			return &images[i], nil
		}
	}
	for i := range images {
		if strings.EqualFold(images[i].Name, wanted) || strings.EqualFold(images[i].DisplayName, wanted) {
			return &images[i], nil
		}
	}
	return nil, errors.Errorf("cloud.ru has no image named %q (resolved to %q); SC aliases: %s",
		name, wanted, strings.Join(sortedMapKeys(cloudruEvolutionImageAliases), ", "))
}

// FindDiskType resolves a storage class, defaulting to the project's only one
// when the config names none.
func (c *Client) FindDiskType(ctx context.Context, name string) (*DiskType, error) {
	types, err := c.ListDiskTypes(ctx)
	if err != nil {
		return nil, err
	}
	if len(types) == 0 {
		return nil, errors.Errorf("cloud.ru project offers no disk types")
	}
	if name == "" {
		return &types[0], nil
	}
	for i := range types {
		if strings.EqualFold(types[i].Name, name) || strings.EqualFold(types[i].DisplayName, name) {
			return &types[i], nil
		}
	}
	names := make([]string, 0, len(types))
	for _, t := range types {
		names = append(names, t.Name)
	}
	return nil, errors.Errorf("cloud.ru has no disk type %q (available: %s)", name, strings.Join(names, ", "))
}

// FindSubnet picks the subnet a VM's interface attaches to: the configured one,
// or the zone's default.
func (c *Client) FindSubnet(ctx context.Context, zone, id string) (*Subnet, error) {
	subnets, err := c.ListSubnets(ctx)
	if err != nil {
		return nil, err
	}
	if id != "" {
		for i := range subnets {
			if subnets[i].ID == id || subnets[i].Name == id {
				return &subnets[i], nil
			}
		}
		return nil, errors.Errorf("cloud.ru has no subnet %q in this project", id)
	}
	var inZone []Subnet
	for _, s := range subnets {
		if s.Zone.Name == zone {
			inZone = append(inZone, s)
		}
	}
	if len(inZone) == 0 {
		return nil, errors.Errorf("cloud.ru project has no subnet in zone %q "+
			"(note: this cloud has no separate `network` object — /networks is a 404 — the subnet IS the network)", zone)
	}
	for i := range inZone {
		if inZone[i].Default {
			return &inZone[i], nil
		}
	}
	return &inZone[0], nil
}

// FindSecurityGroups resolves the groups a VM joins: the configured ones, or the
// zone's default group.
func (c *Client) FindSecurityGroups(ctx context.Context, zone string, ids []string) ([]SecurityGroup, error) {
	groups, err := c.ListSecurityGroups(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		out := make([]SecurityGroup, 0, len(ids))
		for _, id := range ids {
			found := false
			for i := range groups {
				if groups[i].ID == id || groups[i].Name == id {
					out = append(out, groups[i])
					found = true
					break
				}
			}
			if !found {
				return nil, errors.Errorf("cloud.ru has no security group %q in this project", id)
			}
		}
		return out, nil
	}
	for i := range groups {
		if groups[i].Zone.Name == zone && groups[i].Default {
			return []SecurityGroup{groups[i]}, nil
		}
	}
	for i := range groups {
		if groups[i].Zone.Name == zone {
			return []SecurityGroup{groups[i]}, nil
		}
	}
	return nil, errors.Errorf("cloud.ru project has no security group in zone %q", zone)
}

// --- create / delete --------------------------------------------------------

type createDiskRequest struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Size      int    `json:"size"`
	DiskType  string `json:"disk_type_name"`
	Zone      string `json:"availability_zone_name"`
	ImageID   string `json:"image_id,omitempty"`
}

// CreateDisk creates one boot disk. Size is GiB.
func (c *Client) CreateDisk(ctx context.Context, name string, sizeGiB int, diskType, zone, imageID string) (*Disk, error) {
	if err := ValidateComputeName("disk", name); err != nil {
		return nil, err
	}
	body := createDiskRequest{
		ProjectID: c.creds.ProjectID,
		Name:      name,
		Size:      sizeGiB,
		DiskType:  diskType,
		Zone:      zone,
		ImageID:   imageID,
	}
	var disk Disk
	if err := c.request(ctx, http.MethodPost, c.computeURL, computeAPIPath+"/disks",
		c.computeQuery(), body, &disk); err != nil {
		return nil, err
	}
	return &disk, nil
}

func (c *Client) DeleteDisk(ctx context.Context, id string) error {
	err := c.request(ctx, http.MethodDelete, c.computeURL, computeAPIPath+"/disks/"+url.PathEscape(id),
		c.computeQuery(), nil, nil)
	if err != nil && NotFound(err) {
		return nil
	}
	return err
}

type createVmDisk struct {
	DiskID string `json:"disk_id"`
}

// createVmRequest is the whole accepted field set of `POST /vms`, discovered by
// sending every plausible key and keeping the ones FastAPI did not answer
// `extra_forbidden` for (2026-10-24).
//
// Two absences decide the shape of the driver above:
//
//   - **`user_data` is refused.** So is every spelling of networking —
//     `subnet_id`, `security_group_ids`, `interfaces`, `networks`, `nics`,
//     `subnet`, `security_groups`. A new VM therefore lands in its AZ's DEFAULT
//     subnet and DEFAULT security group, and the only way to shape its boot is
//     the image's own `image_metadata` slots.
//   - There is no `count`: a batch is expressed by repeating the object in the
//     request list.
type createVmRequest struct {
	ProjectID     string            `json:"project_id"`
	Name          string            `json:"name"`
	Zone          string            `json:"availability_zone_name"`
	FlavorID      string            `json:"flavor_id"`
	Disks         []createVmDisk    `json:"disks"`
	Description   string            `json:"description,omitempty"`
	ImageMetadata map[string]string `json:"image_metadata,omitempty"`
}

// CreateVm creates one VM. The request body is a **list** — the API accepts a
// batch and SC always sends exactly one, because a batch failure reports no
// per-item status and would leave SC unable to say which machine exists.
//
// `disks` must reference an already-created disk: an empty array answers HTTP
// 500.
func (c *Client) CreateVm(ctx context.Context, req createVmRequest) (*Vm, error) {
	if err := ValidateComputeName("vm", req.Name); err != nil {
		return nil, err
	}
	if len(req.Disks) == 0 {
		return nil, errors.Errorf("cloud.ru VM %q cannot be created without a boot disk "+
			"(the API answers HTTP 500, not a validation error, for an empty `disks`)", req.Name)
	}
	var created []Vm
	if err := c.request(ctx, http.MethodPost, c.computeURL, computeAPIPath+"/vms",
		c.computeQuery(), []createVmRequest{req}, &created); err != nil {
		return nil, err
	}
	if len(created) == 0 {
		return nil, errors.Errorf("cloud.ru accepted the create of VM %q but returned no object", req.Name)
	}
	return &created[0], nil
}

func (c *Client) DeleteVm(ctx context.Context, id string) error {
	err := c.request(ctx, http.MethodDelete, c.computeURL, computeAPIPath+"/vms/"+url.PathEscape(id),
		c.computeQuery(), nil, nil)
	if err != nil && NotFound(err) {
		return nil
	}
	return err
}

type createFloatingIPRequest struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Zone      string `json:"availability_zone_name"`
}

// CreateFloatingIP allocates one public address. Unlike /vms, this endpoint takes
// a single object — a list body answers HTTP 500.
func (c *Client) CreateFloatingIP(ctx context.Context, name, zone string) (*FloatingIP, error) {
	if err := ValidateComputeName("floating ip", name); err != nil {
		return nil, err
	}
	body := createFloatingIPRequest{ProjectID: c.creds.ProjectID, Name: name, Zone: zone}
	var fip FloatingIP
	if err := c.request(ctx, http.MethodPost, c.computeURL, computeAPIPath+"/floating-ips",
		c.computeQuery(), body, &fip); err != nil {
		return nil, err
	}
	return &fip, nil
}

// AttachFloatingIP binds an address to an interface.
//
// `PUT` with `{"interface_id": …}`: there is no `POST …/attach` route and `PATCH`
// is not allowed. The call 422s while the address is still provisioning, which is
// why the caller waits for `available` first.
func (c *Client) AttachFloatingIP(ctx context.Context, fipID, interfaceID string) error {
	body := map[string]string{"interface_id": interfaceID}
	return c.request(ctx, http.MethodPut, c.computeURL, computeAPIPath+"/floating-ips/"+url.PathEscape(fipID),
		c.computeQuery(), body, nil)
}

func (c *Client) DeleteFloatingIP(ctx context.Context, id string) error {
	err := c.request(ctx, http.MethodDelete, c.computeURL, computeAPIPath+"/floating-ips/"+url.PathEscape(id),
		c.computeQuery(), nil, nil)
	if err != nil && NotFound(err) {
		return nil
	}
	return err
}

// CreateSecurityGroupRule adds one rule to a group.
//
// This is the one compute route that must be called WITHOUT the `project_id`
// query parameter: the endpoint declares no such field and FastAPI refuses the
// request outright (422 `extra_forbidden`). Passing the project here was a
// twenty-minute diversion into imagined permission problems.
func (c *Client) CreateSecurityGroupRule(ctx context.Context, groupID string, rule SecurityGroupRule) error {
	return c.request(ctx, http.MethodPost, c.computeURL,
		computeAPIPath+"/security-groups/"+url.PathEscape(groupID)+"/rules", nil, rule, nil)
}

// ListSecurityGroupRules reads one group's rules.
//
// Needed even though SecurityGroup has a `Rules` field, because the group
// endpoints do NOT fill it: `GET /security-groups` and `GET /security-groups/{id}`
// both answer with `ingress_rules_count` and no rules at all (measured
// 2026-10-24). A caller that trusted the inlined slice would see every group as
// empty — and would therefore re-create a rule it already had on every deploy, and
// find nothing to clean up on destroy.
//
// Shares CreateSecurityGroupRule's one quirk, hence the hand-rolled request rather
// than listPaged: the `/rules` sub-route declares no `project_id` and refuses it
// with 422 `extra_forbidden`, while listPaged always injects it. Paging is not a
// concern here for the same reason the route has no project — it answers with the
// group's whole rule set in one `{"items":[…]}` envelope.
func (c *Client) ListSecurityGroupRules(ctx context.Context, groupID string) ([]SecurityGroupRule, error) {
	var raw json.RawMessage
	if err := c.request(ctx, http.MethodGet, c.computeURL,
		computeAPIPath+"/security-groups/"+url.PathEscape(groupID)+"/rules", nil, nil, &raw); err != nil {
		if NotFound(err) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to list rules of cloud.ru security group %q", groupID)
	}
	envelope, err := decodeCollection[SecurityGroupRule](raw)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode rules of cloud.ru security group %q", groupID)
	}
	return envelope.Items, nil
}

// DeleteSecurityGroupRule removes one rule. Absence is success, as everywhere else
// on the destroy path.
//
// The call answers 204 and the rule survives for ~20 s afterwards (the delete is
// async on this cloud); a repeat then answers 404. So a caller must not read the
// group back to confirm — it will see the rule and conclude the delete failed.
func (c *Client) DeleteSecurityGroupRule(ctx context.Context, groupID, ruleID string) error {
	err := c.request(ctx, http.MethodDelete, c.computeURL,
		computeAPIPath+"/security-groups/"+url.PathEscape(groupID)+"/rules/"+url.PathEscape(ruleID), nil, nil, nil)
	if err != nil && NotFound(err) {
		return nil
	}
	return err
}

// sortedMapKeys is a tiny helper so error messages list aliases deterministically.
func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
