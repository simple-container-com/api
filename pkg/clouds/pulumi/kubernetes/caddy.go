// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package kubernetes

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/pkg/errors"
	"github.com/samber/lo"

	sdkK8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/simple-container-com/api/internal/build"
	"github.com/simple-container-com/api/pkg/api"
	// Aliased: DeployCaddyService takes a parameter named `caddy`, which would
	// shadow the package inside the one function that needs it.
	caddyassets "github.com/simple-container-com/api/pkg/clouds/caddy"
	"github.com/simple-container-com/api/pkg/clouds/k8s"
	pApi "github.com/simple-container-com/api/pkg/clouds/pulumi/api"
)

type CaddyDeployment struct {
	*k8s.CaddyConfig
	ClusterName string
	// ObjectsName overrides caddy-<env> for every object of this Caddy. Empty keeps
	// caddy-<env>; see KubernetesCaddyDeploymentName. Deliberately NOT named
	// DeploymentName: that would shadow the promoted CaddyConfig.DeploymentName, which is
	// the user-facing adoption override and a different thing.
	ObjectsName     string
	ClusterResource sdk.Resource
	// CaddyfilePrefixOut is a Caddyfile prefix a CLOUD computed, from values only known
	// during the apply — today just GKE, whose cluster resource provisions a GCS bucket
	// and renders `storage gcs` with the bucket's name (gcp/gke_autopilot.go). It REPLACES
	// the statically-assembled prefix, which is why a cloud that sets it must bake the
	// trusted-proxies block in too.
	//
	// Left zero by every other path, and "is it set" is decided by isPulumiOutputSet,
	// which until 2026-11-08 always answered yes: `trustedProxies` and `caddyfilePrefix`
	// therefore reached NO non-GKE edge, and every such deploy passed an unknown output
	// as CADDYFILE_PREFIX instead.
	CaddyfilePrefixOut sdk.StringOutput
	// Cloud-specific configuration (following SimpleContainer patterns)
	SecretEnvs          map[string]string      // Secret environment variables (e.g., GOOGLE_APPLICATION_CREDENTIALS=/etc/gcp/credentials.json)
	SecretVolumes       []k8s.SimpleTextVolume // Secret volumes to mount (e.g., GCP credentials)
	SecretVolumeOutputs []any                  // Pulumi outputs for secret volumes
}

// isPulumiOutputSet reports whether a StringOutput was ever given a value, as opposed to
// being the zero value of its field.
//
// It checks the embedded *OutputState, which is the only thing that distinguishes the
// two. ElementType() CANNOT: it is a method on the VALUE type returning a package-level
// `reflect.TypeOf("")`, so it answers non-nil for every StringOutput that exists,
// including `var o sdk.StringOutput`. That made this function a constant `true` —
// see CaddyfilePrefixOut's own doc for what that cost.
//
// An unset output is not merely empty: a nil OutputState awaits as UNKNOWN (value nil,
// known false), so handing one to a provider as an input is not "" but an unresolved
// value.
func isPulumiOutputSet(output sdk.StringOutput) bool {
	return output.OutputState != nil
}

// caddyHSTSEnv returns env-var additions overriding the `(hsts)` snippet's
// header. Empty string is treated as unset — Caddy's `os.LookupEnv` would
// otherwise treat a present-but-empty var as "found" and ship a broken
// `Strict-Transport-Security ""`.
func caddyHSTSEnv(cfg *k8s.CaddyConfig) map[string]string {
	v := lo.FromPtr(cfg).HSTSValue
	if v == nil || *v == "" {
		return nil
	}
	return map[string]string{"HSTS_VALUE": *v}
}

var reservedAnnotationPrefixes = []string{"simple-container.com/", "pulumi.com/"}

// caddyPodAnnotations validates user pod annotations. SC-owned keys are rejected
// rather than silently overridden: simple-container.com/caddyfile-entry, for one,
// is read back by the Caddy init container as a route source.
func caddyPodAnnotations(cfg *k8s.CaddyConfig) (map[string]string, error) {
	annotations := lo.FromPtr(cfg).PodAnnotations
	for key := range annotations {
		if errs := validation.IsQualifiedName(key); len(errs) > 0 {
			return nil, errors.Errorf("caddy.podAnnotations: invalid key %q: %s", key, strings.Join(errs, "; "))
		}
		for _, prefix := range reservedAnnotationPrefixes {
			if strings.HasPrefix(key, prefix) {
				return nil, errors.Errorf("caddy.podAnnotations: key %q uses the reserved prefix %q", key, prefix)
			}
		}
	}
	return annotations, nil
}

// caddyfileRenderScript is the Caddy init container's entrypoint: it assembles
// /tmp/Caddyfile from the embedded base config plus every Service carrying a
// simple-container.com/caddyfile-entry annotation, and waits for the annotation
// change it was rolled for to become visible before doing so.
//
// A const rather than an inline literal so caddy_render_script_test.go can run it
// against a fake kubectl — the logic that matters here is shell, so the test that
// matters has to execute shell.
const caddyfileRenderScript = `
	      # set -e (exit on error) + pipefail (any pipe component fail = fail).
	      # Notably we do NOT enable -x here: tracing every command would dump
	      # the raw caddyfile-entry annotation body to stdout for every Service
	      # on every pod restart, which lands in cluster logs (GCP/Datadog/ELK).
	      # SC-generated annotations don't contain secrets, but consumer-side
	      # misuse (eg. basicauth credentials in Headers or LbConfig.ExtraHelpers
	      # that templated into the annotation) could leak via -x. The trade-off
	      # is debuggability — for live troubleshooting, re-enable -x by
	      # overriding the init-container command in the cluster.
	      set -eo pipefail;

	      # Defaults are the only values production ever uses; every one of these is
	      # overridable solely so the render + retry logic can be exercised by a unit
	      # test (caddy_render_script_test.go) without a cluster and without waiting a
	      # real minute. CADDY_SYNC_ATTEMPTS/INTERVAL double as an operator escape
	      # hatch if a very large cluster ever needs a longer window.
	      CADDY_SRC_CADDYFILE="${CADDY_SRC_CADDYFILE:-/etc/caddy/Caddyfile}"
	      CADDY_WORK_DIR="${CADDY_WORK_DIR:-/tmp}"
	      CADDY_SYNC_ATTEMPTS="${CADDY_SYNC_ATTEMPTS:-20}"
	      CADDY_SYNC_INTERVAL="${CADDY_SYNC_INTERVAL:-3}"

	      # render_caddyfile <out-file> <entry-hash-file>
	      #
	      # Assembles the whole Caddyfile from live cluster state, and records md5 of
	      # each Service's raw caddyfile-entry annotation into the hash file. The hash
	      # is what lets the caller below decide whether the state it rendered from is
	      # new enough — see the retry loop. A function, not straight-line script,
	      # purely so it can be re-run; the body is unchanged apart from writing to
	      # "$out" instead of /tmp/Caddyfile directly.
	      render_caddyfile() {
	      out="$1"; hashes="$2"
	      : > "$hashes"
	      cp -f "$CADDY_SRC_CADDYFILE" "$out";

	      # Inject custom Caddyfile prefix at the top (e.g., GCS storage configuration)
	      if [ -n "$CADDYFILE_PREFIX" ]; then
	        echo "$CADDYFILE_PREFIX" >> "$out"
	        echo "" >> "$out"
	      fi

	      # List Services carrying the caddyfile-entry annotation. We also pull
	      # creationTimestamp so we can dedup by site-address with the newest
	      # Service winning — during a Pulumi Replace of a namespace (or Service),
	      # the old and new Services transiently coexist and both carry the same
	      # annotation; without dedup that produced two "http://<domain> { ... }"
	      # blocks and Caddy aborted with "ambiguous site definition".
	      # kubectl and sort are split into separate assignments so a kubectl
	      # failure surfaces unambiguously even without pipefail (originally
	      # they were piped; pipefail was added in response to a review catch
	      # and we kept the structural split so future readers do not need
	      # to know about pipefail to reason about failure modes here).
	      # pipefail is kept on as belt-and-suspenders for the later
	      # printf-to-sort pipe and the printf-to-while-read pipeline below.
	      # If either listing step fails the init-container exits non-zero
	      # and K8s reschedules — preferable to a Caddyfile with only the
	      # default 503 block, which would mean a complete loss of routing
	      # for the entire cluster.
	      raw_services=$(kubectl get services --all-namespaces -o jsonpath='{range .items[?(@.metadata.annotations.simple-container\.com/caddyfile-entry)]}{.metadata.creationTimestamp}{" "}{.metadata.namespace}{" "}{.metadata.name}{"\n"}{end}')
	      services=$(printf '%s' "$raw_services" | sort -r)
          echo "$DEFAULT_ENTRY_START" >> "$out"
          if [ "$USE_PREFIXES" == "false" ]; then
            echo "$DEFAULT_ENTRY" >> "$out"
	        echo "}" >> "$out"
          fi
	      # Dedup state: first non-blank, non-comment line of each annotation is
	      # the site address (e.g. "http://support-payhey.pay.space {") or the
	      # "handle_path /<prefix>*" matcher for prefix routing. Whitespace is
	      # trimmed both sides so an indentation difference can't pass through as
	      # a distinct key. Already-seen keys are skipped — most-recently-created
	      # Service wins via sort -r.
	      #
	      # Cleaned up explicitly at the end of the function rather than via
	      # 'trap ... EXIT': the function runs more than once, and a trap would
	      # re-register on every call.
	      seen=$(mktemp)
	      raw=$(mktemp)
	      # Process each service that has Caddyfile entry annotation
	      printf '%s\n' "$services" | while read ts ns service; do
	          if [ -n "$ns" ] && [ -n "$service" ]; then
	              # Fetched to a FILE, not a $(...) capture, so the md5 below is taken
	              # over the annotation's exact bytes. kubectl -o jsonpath appends no
	              # newline, but the entry template ends with one, and command
	              # substitution strips trailing newlines — hashing the capture would
	              # give a digest that never equals CADDY_UPDATE_HASH.
	              kubectl get service -n "$ns" "$service" -o jsonpath='{.metadata.annotations.simple-container\.com/caddyfile-entry}' > "$raw" 2>/dev/null || true
	              entry=$(cat "$raw")
	              if [ -z "$entry" ]; then
	                  continue
	              fi
	              key=$(printf '%s\n' "$entry" | awk '
	                  /^[[:space:]]*$/ { next }
	                  /^[[:space:]]*#/ { next }
	                  { sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+$/, ""); print; exit }
	              ')
	              if [ -n "$key" ] && grep -qFx -- "$key" "$seen" 2>/dev/null; then
	                  echo "Skipping duplicate caddyfile-entry '$key' from $ns/$service (older Service)"
	                  continue
	              fi
	              [ -n "$key" ] && printf '%s\n' "$key" >> "$seen"
	              md5sum "$raw" | awk '{print $1}' >> "$hashes"
	              echo "Processing service: $service in namespace: $ns"
	              printf '%s\n' "$entry" >> "$out"
	              echo "" >> "$out"
	          fi
	      done
          if [ "$USE_PREFIXES" == "true" ]; then
            echo "$DEFAULT_ENTRY" >> "$out"
	        echo "}" >> "$out"
          fi
          echo "" >> "$out"
	      rm -f "$seen" "$raw"
	      }

	      # Render, and if this pod was rolled for a Caddyfile change it cannot yet
	      # see, render again.
	      #
	      # CADDY_UPDATE_HASH is md5 of the caddyfile-entry body whose change rolled
	      # this deployment. The patch that rolls us can land BEFORE the Service
	      # annotation it hashed is visible to a 'kubectl get' (measured 2026-11-08:
	      # pod template at 16:16:30, annotation at 16:16:39) — so the first pod of a
	      # rollout can render the PREVIOUS config and keep serving it for its whole
	      # lifetime, because nothing re-renders and the Deployment has no further
	      # reason to roll. With replicas > 1 the other pods render correctly and the
	      # result is a permanent ~1/N failure rate across every node address, with
	      # all pods Running and Ready.
	      #
	      # PatchDeployment now waits on the Service's applied state, so the ordering
	      # inversion should no longer occur from the SC-driven path. This wait stays
	      # because it is the only thing that covers the cases pulumi ordering cannot:
	      # a hand-edited or externally-reconciled annotation, and any future caller
	      # that forgets to pass the dependency.
	      #
	      # So: wait until one of the entries we just rendered hashes to the value we
	      # were rolled for. FAIL-OPEN after ~60s, because a hash that can never match
	      # is a legitimate state and must not take the edge down:
	      #   - the app that patched the hash has since been deleted, so its Service
	      #     (and its entry) no longer exist;
	      #   - a migrated stack whose Service annotation is re-rendered against the
	      #     legacy namespace while the hash is taken over the static render, so the
	      #     two differ by design.
	      # In both cases we proceed with exactly the config today's code would have
	      # produced, having only cost ourselves the delay — never readiness.
	      attempt=0
	      while : ; do
	          render_caddyfile "$CADDY_WORK_DIR/Caddyfile.rendering" "$CADDY_WORK_DIR/caddy-entry-hashes"
	          if [ -z "$CADDY_UPDATE_HASH" ] || grep -qFx -- "$CADDY_UPDATE_HASH" "$CADDY_WORK_DIR/caddy-entry-hashes"; then
	              break
	          fi
	          attempt=$((attempt + 1))
	          if [ "$attempt" -ge "$CADDY_SYNC_ATTEMPTS" ]; then
	              echo "WARNING: caddy-update-hash $CADDY_UPDATE_HASH is not present in any Service's caddyfile-entry after $attempt attempts; rendering from current cluster state anyway" >&2
	              break
	          fi
	          echo "caddy-update-hash $CADDY_UPDATE_HASH not visible in cluster state yet, retrying ($attempt/$CADDY_SYNC_ATTEMPTS)"
	          sleep "$CADDY_SYNC_INTERVAL"
	      done
	      # Single mv so a partially-rendered file is never what Caddy starts on.
	      mv -f "$CADDY_WORK_DIR/Caddyfile.rendering" "$CADDY_WORK_DIR/Caddyfile"
	      cat "$CADDY_WORK_DIR/Caddyfile"
		`

func CaddyResource(ctx *sdk.Context, stack api.Stack, input api.ResourceInput, params pApi.ProvisionParams) (*api.ResourceOutput, error) {
	if input.Descriptor.Type != k8s.ResourceTypeCaddy {
		return nil, errors.Errorf("unsupported caddy type %q", input.Descriptor.Type)
	}

	caddyCfg, ok := input.Descriptor.Config.Config.(*k8s.CaddyResource)
	if !ok {
		return nil, errors.Errorf("failed to convert caddy config for %q", input.Descriptor.Type)
	}

	objectsName, err := KubernetesCaddyDeploymentName(input.Descriptor.Name, input.StackParams.Environment, caddyCfg.UseResourceName,
		stack.Server.Resources.Resources)
	if err != nil {
		return nil, err
	}

	// Resolved, not read raw. A Caddy edge is the commonest consumer of
	// `${resource:<cluster>.kubeconfig}` — CaddyReadConfig already refuses a
	// MALFORMED reference — but this path builds its own provider
	// (DeployCaddyService below) instead of going through kubernetes.Provider, so
	// until this call a well-formed reference was handed to pulumi-kubernetes as
	// the literal string `${resource:smoke-k3s.kubeconfig}`. That parses as a YAML
	// string scalar, and the provider fails with "couldn't get version/kind; json
	// parse error: cannot unmarshal string into Go value of type struct {
	// APIVersion; Kind }" — an error that names neither the resource nor the
	// reference, on a resource (the ServiceAccount) that is not the one at fault.
	// Live-caught on the 3-VPS k3s smoke, 2026-11-09.
	kubeconfig, err := providerKubeconfig(caddyCfg.KubernetesConfig, params)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid kubeconfig for caddy resource %q", input.Descriptor.Name)
	}

	params.Log.Info(ctx.Context(), "Deploying caddy service...")
	caddyService, err := DeployCaddyService(ctx, CaddyDeployment{
		CaddyConfig: caddyCfg.CaddyConfig,
		ClusterName: input.Descriptor.Name,
		ObjectsName: objectsName,
	}, input, params, kubeconfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to deploy caddy service for stack %q", stack.Name)
	}

	return &api.ResourceOutput{
		Ref: caddyService,
	}, nil
}

func DeployCaddyService(ctx *sdk.Context, caddy CaddyDeployment, input api.ResourceInput, params pApi.ProvisionParams, kubeconfig sdk.StringOutput) (*SimpleContainer, error) {
	params.Log.Info(ctx.Context(), "Configure Caddy deployment for cluster %q in %q", input.Descriptor.Name, input.StackParams.Environment)
	podAnnotations, err := caddyPodAnnotations(caddy.CaddyConfig)
	if err != nil {
		return nil, err
	}
	kubeProvider, err := sdkK8s.NewProvider(ctx, fmt.Sprintf("%s-caddy-kubeprovider", input.ToResName(input.Descriptor.Name)), &sdkK8s.ProviderArgs{
		// Marked here rather than at each caller so every path into Caddy —
		// the k8s resource below, and gke_autopilot.go's derived kubeconfig —
		// crosses the provider boundary secret. See SecretKubeconfig.
		Kubeconfig: SecretKubeconfigOutput(kubeconfig),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision kubeconfig provider for %q/%q in %q",
			input.StackParams.StackName, input.Descriptor.Name, input.StackParams.Environment)
	}
	// Use the same naming convention as the patch operation for consistency
	deploymentName := lo.If(caddy.ObjectsName != "", caddy.ObjectsName).Else(GenerateCaddyDeploymentName(input.StackParams.Environment))
	namespace := lo.If(caddy.Namespace != nil, lo.FromPtr(caddy.Namespace)).Else("caddy")
	caddyImage := lo.If(caddy.Image != nil, lo.FromPtr(caddy.Image)).Else(fmt.Sprintf("simplecontainer/caddy:%s", build.Version))

	// Assembled here and not inside the container's Env closure below, which cannot
	// return an error: before this, BuildTrustedProxiesBlock's error was discarded with
	// `_` and an invalid CIDR silently dropped the whole block, leaving an edge that
	// trusts no proxy and reports every client as the load balancer. The certificate
	// store is validated on the way through, so a bad store is a refusal here rather
	// than an error from inside a Pulumi apply — see caddyAcmeStorageBlock.
	staticPrefix, err := staticCaddyfilePrefix(caddy, namespace, deploymentName)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid caddy global options for cluster %q", input.Descriptor.Name)
	}

	// Generate volume names using the same logic as SimpleContainer to ensure consistency
	// This fixes the volume mount name mismatch issue for custom stacks
	// Use the deploymentName (which includes environment suffix) as the service name
	parentEnv := input.StackParams.ParentEnv
	stackEnv := input.StackParams.Environment
	serviceName := sanitizeK8sName(deploymentName)
	volumesCfgName := generateConfigVolumesName(serviceName, stackEnv, parentEnv)

	// Prepare Caddy volumes (embedded config)
	var caddyVolumes []k8s.SimpleTextVolume
	caddyVolumes, err = EmbedFSToTextVolumes(caddyVolumes, caddyassets.FS, caddyassets.AssetsDir, "/etc/caddy")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read embedded caddy config files")
	}

	defaultCaddyFileEntryStart := `http:// {`
	// Default catch-all serves a hard 503 page from /etc/caddy/pages/503.html
	// instead of `file_server` over the whole pages dir (which used to serve
	// index.html with status 200 for any unknown Host — invisible to monitoring
	// when every backend was gone).
	//
	// Rationale for 503:
	// - When all Services with `simple-container.com/caddyfile-entry` for a
	//   given Host vanish (e.g. cascade-deletion from a namespace Replace gone
	//   wrong), the request now gets HTTP 503 + Retry-After. CDNs fail over,
	//   uptime checks alert, oncall sees it.
	//
	// Why file_server (not respond with inlined HTML):
	// - Symmetric with the existing `handle_bucket_error` / `handle_server_error`
	//   snippets in pkg/clouds/caddy/assets/Caddyfile, which serve {404,500,502}.html the
	//   same way for per-Service error fallbacks. One pattern for every status
	//   page in this codebase.
	// - file_server emits Content-Type automatically from the file extension,
	//   so no explicit `header Content-Type` needed.
	// - Operators can override the 503 body by mounting a different ConfigMap
	//   at /etc/caddy/pages/503.html without touching SC api code.
	//
	// Wrapped in `handle { ... }` so the directives below apply only to the
	// 503 path and nothing else can short-circuit (e.g. `import hsts` redir
	// firing before the response). We also intentionally do NOT `import hsts`
	// here — sending an HSTS header from a catch-all that answers any Host is
	// meaningless, and the HTTP→HTTPS redirect would only route the request
	// into a TLS handshake failure (Caddy has no cert for an unknown SNI),
	// which is invisible to HTTP-layer monitoring.
	defaultCaddyFileEntry := `
  import gzip
  handle {
    root * /etc/caddy/pages
    rewrite * /503.html
    header Cache-Control "no-store"
    header Retry-After "60"
    file_server {
      status 503
    }
  }
`
	// Still computed because it's threaded into per-stack Caddyfile entries
	// elsewhere in this function; intentionally NOT applied to the catch-all
	// default block above (see comment on `import hsts` omission).
	//
	// Read through a local value rather than off the promoted embedded pointer:
	// `caddy.ClientTlsEnabled()` would dereference a nil CaddyConfig, and this
	// function is already careful about that elsewhere (BuildTrustedProxiesBlock
	// below takes the same lo.FromPtrOr shape).
	caddyTls := lo.FromPtrOr(caddy.CaddyConfig, k8s.CaddyConfig{}).TlsConfig
	if err := caddyTls.ValidateTls(); err != nil {
		return nil, err
	}
	useSSL := caddyTls.ClientTlsEnabled()

	serviceAccountName := input.ToResName(fmt.Sprintf("%s-caddy-sa", input.Descriptor.Name))
	serviceAccount, err := NewSimpleServiceAccount(ctx, serviceAccountName, &SimpleServiceAccountArgs{
		Name:      serviceAccountName,
		Namespace: namespace,
		Resources: []string{"services"},
	}, sdk.Provider(kubeProvider))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to crate service account for caddy")
	}

	// Build Caddy container configuration
	caddyContainer := k8s.CloudRunContainer{
		Name:    deploymentName,
		Command: []string{"caddy", "run", "--config", "/tmp/Caddyfile", "--adapter", "caddyfile"},
		Image: api.ContainerImage{
			Name:     caddyImage,
			Platform: api.ImagePlatformLinuxAmd64,
		},
		Ports:     k8s.ContainerPorts(443, 80),
		MainPort:  lo.ToPtr(80),
		Resources: caddy.Resources, // Use custom resources if specified, otherwise defaults will be applied
	}
	initContainer := corev1.ContainerArgs{
		Name:  sdk.String("generate-caddyfile"),
		Image: sdk.String("simplecontainer/kubectl:latest"),
		VolumeMounts: corev1.VolumeMountArray{
			corev1.VolumeMountArgs{
				MountPath: sdk.String("/tmp"),
				Name:      sdk.String("tmp"),
			},
			corev1.VolumeMountArgs{
				MountPath: sdk.String("/etc/caddy/Caddyfile"),
				Name:      sdk.String(volumesCfgName),
				SubPath:   sdk.String("Caddyfile"),
			},
		},
		Env: func() corev1.EnvVarArray {
			envVars := corev1.EnvVarArray{
				corev1.EnvVarArgs{
					Name:  sdk.String("DEFAULT_ENTRY_START"),
					Value: sdk.String(defaultCaddyFileEntryStart),
				},
				corev1.EnvVarArgs{
					Name:  sdk.String("DEFAULT_ENTRY"),
					Value: sdk.String(defaultCaddyFileEntry),
				},
				corev1.EnvVarArgs{
					Name:  sdk.String("USE_PREFIXES"),
					Value: sdk.String(fmt.Sprintf("%t", caddy.UsePrefixes)),
				},
			}

			// The pod's own copy of the annotation that caused this rollout
			// (kube_run.go:371 patches it onto spec.template.metadata). The render
			// below waits until it can see the Service annotation this hash was
			// taken of — see caddyfileSyncAttempts. Absent on a Caddy that no app
			// has ever patched, which the downward API resolves to "".
			envVars = append(envVars, corev1.EnvVarArgs{
				Name: sdk.String("CADDY_UPDATE_HASH"),
				ValueFrom: corev1.EnvVarSourceArgs{
					FieldRef: corev1.ObjectFieldSelectorArgs{
						FieldPath: sdk.String(fmt.Sprintf("metadata.annotations['%s']", AnnotationCaddyUpdateHash)),
					},
				},
			})

			// Add Caddyfile prefix - prefer dynamic output over static config
			if isPulumiOutputSet(caddy.CaddyfilePrefixOut) {
				// Use dynamic Caddyfile prefix from cloud provider (e.g., GCS storage config with trusted proxies baked in)
				envVars = append(envVars, corev1.EnvVarArgs{
					Name:  sdk.String("CADDYFILE_PREFIX"),
					Value: caddy.CaddyfilePrefixOut.ToStringOutput(),
				})
			} else if staticPrefix != "" {
				// Static Caddyfile prefix from config (non-GKE path), assembled above.
				envVars = append(envVars, corev1.EnvVarArgs{
					Name:  sdk.String("CADDYFILE_PREFIX"),
					Value: sdk.String(staticPrefix),
				})
			}

			return envVars
		}(),
		Command: sdk.ToStringArray([]string{"bash", "-c", caddyfileRenderScript}),
	}

	var addOpts []sdk.ResourceOption
	if caddy.ClusterResource != nil {
		addOpts = append(addOpts, sdk.DependsOn([]sdk.Resource{caddy.ClusterResource}))
	}

	serviceType := lo.ToPtr("LoadBalancer")
	if lo.FromPtr(caddy.CaddyConfig).ServiceType != nil {
		serviceType = lo.FromPtr(caddy.CaddyConfig).ServiceType
	}

	// Prepare deployment config
	deploymentConfig := k8s.DeploymentConfig{
		StackConfig:      &api.StackConfigCompose{Env: make(map[string]string)},
		Containers:       []k8s.CloudRunContainer{caddyContainer},
		IngressContainer: &caddyContainer,
		Scale: &k8s.Scale{
			Replicas: lo.If(caddy.Replicas != nil, lo.FromPtr(caddy.Replicas)).Else(1),
		},
		TextVolumes:               caddyVolumes,
		TopologySpreadConstraints: lo.FromPtr(caddy.CaddyConfig).TopologySpreadConstraints,
	}

	for k, v := range caddyHSTSEnv(caddy.CaddyConfig) {
		deploymentConfig.StackConfig.Env[k] = v
	}

	// Prepare secret environment variables (e.g., GOOGLE_APPLICATION_CREDENTIALS for GCP)
	secretEnvs := make(map[string]string)
	// The shared store's credentials. They go in as SECRET envs, not into the
	// Caddyfile the init container renders: `storage s3` holds them as `{$NAME}`
	// placeholders that Caddy's parser resolves in this container at config load, so
	// the file on disk — world-readable, and echoed verbatim by `caddy adapt` — never
	// carries the value. Delivered under the names CredentialEnv chooses, which is the
	// same map the placeholders were rendered from.
	for k, v := range lo.FromPtr(caddy.CaddyConfig).AcmeStorage.CredentialEnv() {
		secretEnvs[k] = v
	}
	if len(caddy.SecretEnvs) > 0 {
		// Log secret environment variable names (not values for security)
		secretEnvNames := make([]string, 0, len(caddy.SecretEnvs))
		for k, v := range caddy.SecretEnvs {
			secretEnvs[k] = v
			secretEnvNames = append(secretEnvNames, k)
		}
		params.Log.Info(ctx.Context(), "🔐 Adding %d secret environment variables to Caddy: %v", len(caddy.SecretEnvs), secretEnvNames)
	}

	sc, err := DeploySimpleContainer(ctx, Args{
		ServiceType:                   serviceType, // to provision external IP
		ExternalTrafficPolicy:         lo.FromPtr(caddy.CaddyConfig).ExternalTrafficPolicy,
		ProvisionIngress:              caddy.ProvisionIngress,
		UseSSL:                        useSSL,
		Namespace:                     namespace,
		DeploymentName:                deploymentName,
		Input:                         input,
		ServiceAccountName:            lo.ToPtr(serviceAccount.Name),
		Deployment:                    deploymentConfig,
		SecretVolumes:                 caddy.SecretVolumes,       // Cloud credentials volumes (e.g., GCP service account)
		SecretVolumeOutputs:           caddy.SecretVolumeOutputs, // Pulumi outputs for secret volumes
		SecretEnvs:                    secretEnvs,                // Secret environment variables
		VPA:                           caddy.VPA,                 // Vertical Pod Autoscaler configuration for Caddy
		TerminationGracePeriodSeconds: lo.FromPtr(caddy.CaddyConfig).TerminationGracePeriodSeconds,
		PreStopSleepSeconds:           lo.FromPtr(caddy.CaddyConfig).PreStopSleepSeconds,
		NodeSelector:                  lo.FromPtr(caddy.CaddyConfig).NodeSelector,
		Images: []*ContainerImage{
			{
				Container: caddyContainer,
				ImageName: sdk.String(caddyImage).ToStringOutput(),
			},
		},
		Params:                 params,
		InitContainers:         []corev1.ContainerArgs{initContainer},
		KubeProvider:           kubeProvider,
		GenerateCaddyfileEntry: false,
		Annotations: map[string]string{
			"pulumi.com/patchForce": "true",
		},
		PodAnnotations: podAnnotations,
	}, addOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision simple container for caddy in GKE cluster %q in %q",
			input.Descriptor.Name, input.StackParams.Environment)
	}
	// Smart cluster name handling: avoid double environment suffix
	clusterName := caddy.ClusterName
	env := input.StackParams.Environment
	if env != "" && !strings.HasSuffix(clusterName, "--"+env) {
		// ClusterName doesn't have environment suffix, add it (Case 2: CaddyResource)
		clusterName = ToClusterName(input, caddy.ClusterName)
	}
	// Otherwise, use clusterName as-is (Case 1: GKE Autopilot - already has suffix)
	ctx.Export(ToIngressIpExport(clusterName), sc.Service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
		if status.LoadBalancer == nil || len(status.LoadBalancer.Ingress) == 0 {
			params.Log.Warn(ctx.Context(), "failed to export ingress IP: load balancer is nil and there is no ingress IP found")
			return ""
		}
		ip := lo.FromPtr(status.LoadBalancer.Ingress[0].Ip)
		params.Log.Info(ctx.Context(), "load balancer ip is %v", ip)
		return ip
	}))
	// Only marshal the CaddyConfig, not the entire CaddyDeployment struct
	// to avoid marshaling ClusterResource which contains Pulumi outputs
	if caddyJson, err := json.Marshal(exportedCaddyConfig(caddy)); err != nil {
		return nil, errors.Wrapf(err, "failed to marshal caddy config")
	} else {
		ctx.Export(ToCaddyConfigExport(clusterName), sdk.String(string(caddyJson)))
	}
	return sc, nil
}

// exportedCaddyConfig is the caddy config children read. A Caddy with a derived name
// carries that name, or its children would patch caddy-<env>, the other Caddy of the
// environment. A Caddy named caddy-<env> exports its config unchanged, except for the
// certificate store's credentials.
//
// Note this overwrites the adoption override CaddyConfig.DeploymentName: a Caddy that
// sets both deploymentName: and useResourceName: true exports the derived name, since
// that is the deployment its children actually have to patch.
func exportedCaddyConfig(caddy CaddyDeployment) *k8s.CaddyConfig {
	// This config is marshalled to JSON and exported as a PLAIN stack output — every
	// child stack reads it, and so does anyone who can read the state or run
	// `sc outputs`. `caddy.acmeStorage` is the first field of it to carry credentials,
	// so they are stripped here rather than marked secret: a child needs the deployment
	// name to patch, and has no use at all for the store's keys.
	if caddy.CaddyConfig != nil && caddy.AcmeStorage != nil {
		cfg := *caddy.CaddyConfig
		store := *caddy.AcmeStorage // copy: the pointer is shared with the live config
		store.AccessKey = ""
		store.SecretKey = ""
		store.EncryptionKey = ""
		cfg.AcmeStorage = &store
		caddy.CaddyConfig = &cfg
	}
	if caddy.ObjectsName == "" {
		return caddy.CaddyConfig
	}
	cfg := lo.FromPtr(caddy.CaddyConfig)
	cfg.DeploymentName = lo.ToPtr(caddy.ObjectsName)
	return &cfg
}

func ToIngressIpExport(clusterName string) string {
	return fmt.Sprintf("%s-ingress-ip", clusterName)
}

// ToIngressIpsExport names the output of a cluster whose front door is MORE than one
// address, so a domain-bearing workload on it gets one A record per address instead of
// one record pointing at whichever node happens to be first.
//
// A managed cluster has a single load-balancer address and exports only
// ToIngressIpExport. A k3s pool has no load balancer: klipper-lb binds :80/:443 on
// every node, so every node is the edge, and pointing the name at one of them throws
// away the other N-1 (and makes a node that is merely rebooting look like an outage).
//
// Deliberately a SEPARATE output rather than a change to ToIngressIpExport's value.
// That output is read by a child stack that may be deployed from a checkout older than
// the parent, and it must stay exactly one address: KubeRun writes it straight into an
// A record's value, where a comma-joined list is not a resolvable address but IS a
// successful deploy. A reader that does not know about this output keeps working; one
// that does gets the whole fleet. Joined with commas and read back with strings.Split
// for the same reason ToOpenPortsExport is a formatted string — a Pulumi stack output
// crosses the boundary through JSON, where a []string comes back as []any and a typed
// read of it fails.
func ToIngressIpsExport(clusterName string) string {
	return fmt.Sprintf("%s-ingress-ips", clusterName)
}

func ToClusterName(input api.ResourceInput, resName string) string {
	return input.ToResName(resName)
}
