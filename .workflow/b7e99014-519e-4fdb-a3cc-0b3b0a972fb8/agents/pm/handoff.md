# PM handoff — `sc-yc-deploy-fidelity`

1. **Commit and branch**
   - No code mutations in this turn.
   - Branch: `forge/workflow/99171cbe-7c27-4976-a999-931b834b5d62/run/b7e99014-519e-4fdb-a3cc-0b3b0a972fb8`
   - Roadmap item: `sc-yc-deploy-fidelity`; scope is **S1+S2**. Source: `forge/docs/roadmap/items/sc-yc-deploy-fidelity.md`.

2. **Scope decisions**
   
   | Priority | Decision |
   |---|---|
   | **MUST** | Make a failed YC revision deployment fail SC on **create and update**, identifying the container and folder and retaining the underlying diagnostic when available. |
   | **MUST** | Make the first revision wait for SC-managed service-account IAM bindings, including `container-registry.images.puller`. Do not assume SC manages IAM for a supplied, pre-existing service account. |
   | **MUST** | Remove YC-reserved names from the environment sent to the revision, including inherited `PORT`; log or document **names only**, never values. Keep the shared `client.yaml` and AWS Lambda environment behavior unchanged. |
   | **MUST** | Add focused regressions, a design note, API and `forge-atrium` build/test evidence, and—only when authorized—a GitHub Actions deploy and live smoke. Report code-complete separately from shipped. |
   | **NICE-TO-HAVE** | Include additional reserved names only after checking YC's current documented set; do not invent a broad prefix rule. |
   | **CUT** | S3's general Pulumi/provider-warning rendering change; local `sc deploy`; changes to the shared YAML merely to work around `PORT`. |

3. **Acceptance criteria for Architect → Developer → QA**

   | AC# | Criterion | Current status | Evidence required downstream |
   |---|---|---|---|
   | AC1 | A deliberately broken revision makes SC deploy fail, with actionable YC failure and container/folder identity, on create **and** update. | FAIL — not yet implemented | Regression tests for both paths; authorized live failure probe if safe. An update must not pass merely because an *older* revision still exists. |
   | AC2 | Shared `client.yaml` with `PORT` deploys unchanged on YC; `PORT` is absent from the revision environment, without changing AWS behavior. | FAIL — not yet implemented | Environment-construction test and authorized YC revision inspection/live smoke. |
   | AC3 | Regression tests cover failed create/update, reserved-name filtering, and first-revision IAM ordering. | FAIL — not yet implemented | Named test results and dependency assertion. |
   | AC4 | Build/test API and `forge-atrium`; deploy and live-smoke through GitHub Actions when authorized, otherwise state the precise operator gate and distinguish code-complete from shipped. | FAIL — not yet run | Commands and outputs, workflow-run link and live response, or the exact missing authorization/action. |

4. **Architect handoff**
   1. Resolve the **update postcondition** explicitly: the roadmap's “revision is unset” check catches first-create failure but may miss a failed update when an old revision survives. Define how to prove the attempted revision deployed without making a no-change deployment fail.
   2. Trace where the provider warning and underlying diagnostic can be recovered. If unavailable through this resource, specify the best actionable failure SC can reliably emit; do not claim diagnostic propagation that tests cannot demonstrate.
   3. Check all environment inputs at the final YC boundary, and order the dependency against the IAM binding resources rather than only the service-account resource. Consult `pkg/clouds/pulumi/yandex/serverless_container.go` and the linked Atrium design §12.
   4. Identify the exact authorized GitHub Actions path and live-smoke gate before anyone claims the fix shipped. No local `sc deploy` and no secret values in logs or handoffs.

5. **Files touched**

   | Path | Lines added / removed | What changed |
   |---|---:|---|
   
   None. PM planning turn; no tests run.

6. **Next step:** Architect designs the create/update revision proof and IAM dependency, then hands the minimal implementation and test seams to Developer.

```json
{"slice_id":"sc-yc-deploy-fidelity"}
```

**Verdict:** signoff