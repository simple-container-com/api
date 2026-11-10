// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package vm

import (
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/simple-container-com/api/pkg/clouds/vm"
)

// The store's remote scripts, asserted the same way the ingress and stack scripts are:
// pure functions, run through `sh -n`, no Pulumi and no SSH. Two of the assertions below
// exist because the stand found the bug live and no amount of reading found it first.

func TestAcmeStoreUpScript(t *testing.T) {
	RegisterTestingT(t)

	script := acmeStoreUpScript(acmeStoreUpSpec{
		Image:   "chrislusf/seaweedfs:4.50",
		Compose: "services:\n  store:\n    image: chrislusf/seaweedfs:4.50\n",
		Bucket:  "sc-acme",
		Port:    8333,
	})
	shellCheck(t, script)

	// The compose file is delivered as a blob and moved into place, like every other
	// config this package lays down.
	Expect(decodedBlobs(script)).To(ContainElement(ContainSubstring("image: chrislusf/seaweedfs:4.50")))
	Expect(script).To(ContainSubstring("install -d -m 0750 '" + vm.AcmeStoreDir() + "'"))
	Expect(script).To(ContainSubstring("chmod 0640 '" + vm.AcmeStoreComposePath() + ".new'"))

	// The data volume is created here because compose will not create an `external: true`
	// one — and `external` is what stops a hand-run `docker compose down -v` from
	// deleting the fleet's certificates.
	Expect(script).To(ContainSubstring("docker volume create '" + vm.AcmeStoreDataVolume + "'"))

	// A 403 counts as an answer from the health gate. An unauthenticated GET of the
	// bucket list is what a correctly-configured S3 gateway refuses, so a 2xx-only probe
	// would pass only while the store is misconfigured.
	Expect(script).To(ContainSubstring("2??|40?"))

	// The bucket-presence check reads the FIRST FIELD of each line, and that is the
	// regression this asserts. `s3.bucket.list` prints a padded table, not names:
	// "  sc-acme\tsize:0\tlogical:0\tchunk:0". A `grep -qx sc-acme` against that can
	// never match, so the create ran, printed `created bucket sc-acme`, and the
	// verification immediately after it failed the whole provision (live-caught on the
	// 3-host stand, 2026-11-10).
	Expect(script).To(ContainSubstring(`| awk '{print $1}' | grep -qx 'sc-acme'`))
	Expect(script).ToNot(MatchRegexp(`weed shell[^|\n]*\| grep -qx`))
	// Both the pre-create check and the post-create verification, not just one.
	Expect(strings.Count(script, `awk '{print $1}' | grep -qx`)).To(Equal(2))

	// `weed shell` takes its command on stdin; `-c <cmd>` is answered with
	// `flag provided but not defined: -c`, and `-master` is required in this layout.
	Expect(script).To(ContainSubstring("| docker exec -i '" + vm.AcmeStoreContainer +
		"' weed shell -master localhost:9333"))
	Expect(script).ToNot(ContainSubstring("weed shell -c"))

	// No credential anywhere in this script: the identity file is delivered by its own
	// command, over stdin, which is what keeps it out of every Pulumi log. The spec this
	// function takes carries no credential field at all, and that is deliberate.
	Expect(script).ToNot(ContainSubstring("accessKey"))
	Expect(script).ToNot(ContainSubstring("identities"))
}

// The quoting invariant, asserted through a real shell rather than by inspecting text —
// a bucket name does not pass through vm.ValidateNames.
func TestAcmeStoreUpScript_QuotesAHostileBucket(t *testing.T) {
	RegisterTestingT(t)

	script := acmeStoreUpScript(acmeStoreUpSpec{
		Image:   hostileName,
		Compose: "services: {}\n",
		Bucket:  hostileName,
		Port:    8333,
	})
	shellCheck(t, script)
}
