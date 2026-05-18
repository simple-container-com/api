# Roadmap

This document satisfies the OpenSSF Best Practices `documentation_roadmap`
criterion. It describes how Simple Container plans, tracks, and ships
future work in public.

## Release cadence

Simple Container does not gate features behind a future "version X" cut.
Production releases are cut **automatically on every merge to `main`**,
using calver tags `YYYY.M.X`. Detailed release mechanics, notes
generation, and security-fix labelling conventions are documented in
[`RELEASES.md`](RELEASES.md).

This means there is no separate "next release date" the roadmap targets.
What ships next is what's open and approaching merge.

## Where to read roadmap state

| What | Where |
|---|---|
| **Feature requests + planned work** | [Issues with `feature` label](https://github.com/simple-container-com/api/issues?q=is%3Aissue+label%3Afeature) |
| **Open security work** | [Issues with `security` label](https://github.com/simple-container-com/api/issues?q=is%3Aissue+label%3Asecurity) |
| **Active work in flight** | [Open pull requests](https://github.com/simple-container-com/api/pulls) |
| **Recently shipped** | [Releases page](https://github.com/simple-container-com/api/releases) |
| **Architectural direction** | [`ARCHITECTURE.md`](ARCHITECTURE.md) + design records under [`design/`](design/) |
| **Threat model + security posture** | [`SECURITY.md`](SECURITY.md) |
| **Dependency + SCA policy** | [`DEPENDENCIES.md`](DEPENDENCIES.md) |

## Themes for the current cycle

The following are the broad themes the maintainers are working on. They
are not commitments — priorities shift as security reports, downstream
consumer needs, and contributor bandwidth change. For real-time state,
the Issues page above is canonical.

1. **Supply-chain hardening.** Sigstore keyless signing, SLSA Build L3
   provenance, CycloneDX SBOM attachment, signed-release sidecars,
   reachability-aware SCA gating. The pieces already shipped are
   visible on the [Releases page](https://github.com/simple-container-com/api/releases);
   open work surfaces under the `security` label.
2. **Cloud-integration breadth.** New cloud-provider stack templates
   for AWS / GCP / Azure / Kubernetes deployment shapes. Tracked under
   the `feature` label.
3. **Documentation depth.** Per-cloud guides, troubleshooting,
   walkthrough examples published to
   [`docs.simple-container.com`](https://docs.simple-container.com/).
4. **OpenSSF maturity.** Climbing Scorecard score + completing
   bestpractices.dev attestation against the OpenSSF Baseline (project
   12886). State visible via the badges in the README.

## How a roadmap item becomes shipped code

1. **File an issue** with the appropriate label, or open a discussion
   in the repo's Discussions tab for larger ideas.
2. **Maintainer triage** assigns priority based on user demand, security
   impact, and alignment with the themes above. See
   [`MAINTAINERS.md`](MAINTAINERS.md) for the decision-making
   process maintainers use.
3. **Implementation** happens via PR per the contribution rules in
   [`CONTRIBUTING.md`](CONTRIBUTING.md).
4. **Merge ships it** — the next push to `main` cuts a release.

## Long-term direction

Simple Container is a maintained product, not a hobby project. The
long-term direction is set by the maintainers (see
[`MAINTAINERS.md`](MAINTAINERS.md)) in consultation with the
downstream consumers who depend on it. Material direction changes are
discussed in the public Discussions tab before any breaking change
ships.
