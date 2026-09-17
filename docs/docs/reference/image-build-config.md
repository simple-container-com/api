# Image Build Configuration Reference

Root-level `imageBuild` section of `client.yaml`.

```yaml
schemaVersion: "1.0"

imageBuild:
  reuseExistingCommitTag: boolean   # default: false
```

## reuseExistingCommitTag

When enabled, a deploy whose version is already present in the registry reuses
that image instead of pushing a rebuild over the same tag. The digest already in
the registry is what the security pipeline runs against and what the runtime is
pointed at.

Two things this fixes:

- **Re-running a deploy of one commit no longer changes what ships.** Container
  builds are not reproducible: timestamps, base image drift and layer ordering
  all move, so a rebuild of the same source produces a different digest. Without
  reuse, a rerun deploys an artifact that nothing previously scanned or signed.
- **Registries with immutable tags stop rejecting the rerun.** A second push
  under an existing tag is refused, which fails the deploy on a rerun that
  changed nothing.

The image is still **built** locally on every deploy. Only the push is skipped,
so reuse saves the registry write and the change of digest, not the build time.

### Which versions qualify

Only a version of the form `YYYY.MM.DD-<abbreviated commit sha>`, the CalVer tag
the release pipeline generates. The sha is seven to forty lowercase hex
characters: CI slices `GITHUB_SHA` to seven, and the local fallback is
`git rev-parse --short=7`, where `--short` is a minimum that git lengthens when
seven characters are ambiguous.

The check is on the **shape of the version string**, not on its provenance.
Nothing consults git to confirm the sha belongs to the commit being deployed, so
a version of this shape handed in through `VERSION` or `--deploy-version`
qualifies too.

| Version | Reused | Why |
|---|---|---|
| `2026.09.14-4fc2fda` | yes | has the shape of a tag naming one commit |
| `latest` | no | moves |
| `2026.09.14-nogit`, `-nohash`, `-gitfail` | no | the commit could not be read |
| `v1.2.3`, branch names, anything else | no | not of that shape |

The date comes from `time.Now()` at deploy time, in the runner's local timezone,
not from the commit. **Re-running the same commit on a different local day
produces a different tag**, misses the lookup, and rebuilds and pushes. The
guarantee holds within one day.

### Prerequisites

- **Registry push access is the trust boundary.** The tag shape says nothing
  about who wrote the tag. Reuse adopts whatever is under it, so anyone who can
  push to the registry can decide what a redeploy of that commit runs.
- **With `security.signing.enabled`, `security.signing.verify.enabled` is
  required**, together with `verify.oidcIssuer` and `verify.identityRegexp` for
  keyless, or `signing.publicKey` for key-based. Without verification the
  pipeline would sign and attest an artifact it did not build, turning registry
  push access into a valid signature from this pipeline. When signing is off,
  reuse proceeds without a verification step.
- Registry credentials must be configured. Without them the lookup cannot
  authenticate and reuse does not engage.

### Failure handling

Only a missing tag means "build". A registry that cannot be read, meaning bad
credentials, an unreachable daemon, a transport error or a 5xx, fails the deploy
instead. Treating those as absence would push over an existing tag exactly when
the registry could not be checked. The lookup is bounded by a 30 second timeout.

A digest the registry returns in an unexpected shape is also an error, never a
silent fallback to the tag.

These cases build and push rather than fail, each with a line in the deploy log
saying so:

- reuse is not enabled, or the version is not of the qualifying shape;
- the registry has no credentials configured;
- signing is enabled but verification is not configured;
- the image already under the tag does not carry a signature this stack accepts.
  Rebuilding overwrites it, which is what removes a planted image.

### Previews

A preview never pushes, so no lookup happens during a dry run. The reuse
decision is therefore not visible in `pulumi preview`; it is made on the update.

### Interaction with ECR lifecycle policies

The default ECR lifecycle policy in this toolchain keeps only the three most
recent images with `tagStatus: any`, and a signed deploy writes four artifacts
into the repository: the image, its signature, and two attestations. Earlier
commit tags are expired quickly, so on AWS reuse will often find nothing to
reuse. Raise `countNumber` or scope the expiry rule to untagged images if you
want reuse to engage across more than the last deploy.
