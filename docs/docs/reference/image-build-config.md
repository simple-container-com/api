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
  builds are not reproducible — timestamps, base image drift and layer ordering
  all move — so a rebuild of the same source produces a different digest. Without
  reuse, a rerun deploys an artifact that nothing previously scanned or signed.
- **Registries with immutable tags stop rejecting the rerun.** A second push
  under an existing tag is refused, which fails the deploy on a rerun that
  changed nothing.

### Which versions qualify

Only a version of the form `YYYY.MM.DD-<7 hex>` — the CalVer tag the release
pipeline generates from the commit sha. A tag of that shape names exactly one
commit, which is the property that makes reuse safe.

Everything else always builds and pushes:

| Version | Reused | Why |
|---|---|---|
| `2026.09.14-4fc2fda` | yes | names one commit |
| `latest` | no | moves |
| `2026.09.14-nogit`, `-nohash`, `-gitfail` | no | the commit could not be read |
| `v1.2.3`, branch names, anything from `VERSION` or `--deploy-version` | no | not commit-derived |

### Failure handling

Only a missing tag means "build". A registry that cannot be read — bad
credentials, an unreachable daemon, a transport error, a 5xx — fails the deploy
instead. Treating those as absence would push over an existing tag exactly when
the registry could not be checked.

A digest the registry returns in an unexpected shape is also an error, never a
silent fallback to the tag.

### Previews

A preview never pushes, so no lookup happens during a dry run.
