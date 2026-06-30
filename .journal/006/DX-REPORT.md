# crossplane-cuefn — Developer Experience Assessment

*Outside-in consumer sweep: 6 personas, each authoring a CUE module → publishing to OCI → installing and reconciling on a real kind cluster. Full end-to-end. All severities below are the corrected/verified set; confidence is the adversarial-reproduction verdict.*

---

## 1. Executive summary

The **core engine is genuinely good**: the local author→render→validate→generate inner loop is fast, cluster-free, and accurate; the CUE contract catches real authoring mistakes; XRD codegen translates bounds/enums/defaults/nested structs faithfully; and the publish/digest-lockstep mechanics work first try against real registries (ttl.sh, GHCR). Four of six personas reached a Ready XR. **But every one of them got there only by independently reverse-engineering the same set of undocumented in-cluster fixes**, and the one persona who followed the headline quickstart *verbatim* never reached a Ready XR — they hit ~8 distinct blockers. Three of those are hard blockers on the documented happy path (a 404 install tag, a function-name mismatch, and a missing `dependsOn`), and the as-shipped function **cannot render anything** until an operator hand-writes a `DeploymentRuntimeConfig` (a CRD whose name appears nowhere in the docs) to supply a writable cache and registry routing.

The gap is not the technology — it is that the documented path and the shipped defaults do not match what actually works in a cluster.

> **READINESS VERDICT: Not yet — close on the engine, blocked on onboarding.**
> The local tooling and packaging are arguably ready, but the documented quickstart fails end-to-end and the as-shipped function cannot render without an undocumented `DeploymentRuntimeConfig`; ship those fixes and this flips to "Ready-with-caveats" quickly.

---

## 2. Top sharp edges (ranked)

Blockers first, then by severity and breadth of impact.

| # | Sev | Area | Title | Who hit it | Confidence |
|---|-----|------|-------|-----------|------------|
| 1 | **Blocker** | cluster-install | Quickstart `install.yaml` pins `function-cuefn:v0` — a tag that 404s (only `v0.1.0/v0.1.1` exist) | p1 | CONFIRMED |
| 2 | **Blocker** | instantiate | Generated Composition's `functionRef.name: function-cuefn` matches neither the quickstart's Function (`cuefn`) nor the auto-installed dep (`meigma-function-cuefn`) | p1 | CONFIRMED |
| 3 | **Blocker** | publish-config | `publish` always emits a `function-environment-configs` step but omits it from the Configuration's `dependsOn` → first XR reconcile fails with "cannot find an active FunctionRevision" | p1, p3, p5 | CONFIRMED |
| 4 | High | reconcile | No documented way to point the installed function at a non-central CUE registry (`CUE_REGISTRY` via `DeploymentRuntimeConfig`) → every non-central module fails "module not found" | p1, p3, p4, p5, p6 | CONFIRMED |
| 5 | High | reconcile | Installed function ships no writable CUE cache → first render fails `mkdir /.cache: permission denied` (nonroot, read-only fs) | p1, p3, p4, p5 | CONFIRMED |
| 6 | High | generate | `generate` lists fully-defaulted nested structs in XRD `required` with no object-level default → apiserver rejects `spec: {}` that `validate`/`render` accept (breaks the documented no-drift guarantee) | p2, p4 | CONFIRMED |
| 7 | Medium | reconcile | Function registry/cache is single shared global state on one `DeploymentRuntimeConfig` → naive `kubectl apply` clobbers other teams (last-write-wins, silent "module not found") | p3, p4, p6 | CONFIRMED |
| 8 | Medium | reconcile | Conditionless composed resource (ConfigMap/Secret) with no `ready` hint holds the XR at `Ready=False` forever; the shipped example never reaches Ready in-cluster | p1, p3, p4 | CONFIRMED |
| 9 | Medium | render | Forgetting `out.input.spec: #Spec` (the docs' "key move") silently disables render-time validation — out-of-bounds XR renders with exit 0, no guardrail | p2 | CONFIRMED |
| 10 | Medium | reconcile | Composing native kinds beyond core workloads (StatefulSet, Ingress) hits a crossplane-SA RBAC wall the docs never mention | p4, p5 | CONFIRMED |
| 11 | Medium | cluster-install | Two Functions on the same package poison the entire Crossplane package lock ("node ... already exists", all packages unhealthy) — the exact trap the name mismatch lures you into | p1 | CONFIRMED |
| 12 | Medium | reconcile | Generated `function-environment-configs` step has no selector → EnvironmentConfig values (the quickstart's `tier=production`) are never merged | p1 | CONFIRMED |
| 13 | Medium | validate | Bound/enum violations on a *defaulted* field produce noisy "N errors in empty disjunction" + "conflicting values \<default\> and \<value\>" (reads as "you must use the default") | p2, p4, p5 | CONFIRMED |
| 14 | Medium | render | Forgetting `--dir` (major-only `@v0` over OCI) yields opaque "module version is not canonical" naming neither `--dir` nor the OCI full-version rule | p2 | CONFIRMED |
| 15 | Medium | docs | Documented `cue vet ./...` exits non-zero on a *correct* module; the working `-c=false` flag is documented nowhere | p2, p5, p6 | CONFIRMED |
| 16 | Medium | authoring | Optional map field (`env: [string]: string`) has no clean idiom — all three natural shapes break (required-no-default / render error / generate failure) | p5 | CONFIRMED |
| 17 | Medium | publish-module | Quickstart Step 3 publishes to `localhost:5000` but never tells you to start a registry | p1 | CONFIRMED |
| 18 | Low | publish-module | `localhost:5000` collides with macOS AirPlay Receiver → misleading `403 Forbidden` (AirTunes) | p1 | CONFIRMED |
| 19 | Low | install | No external install path for the `cuefn` CLI (build-from-source only); no crossplane-CLI link | p1 | CONFIRMED |
| 20 | Low | validate | Omitting a required no-default field reports "incomplete value", not "missing required field" | p2 | CONFIRMED |

---

## 3. Detailed findings

Grouped by severity, then area. Each finding: what / expected / repro / evidence / suggested fix.

### BLOCKERS — the documented happy path is broken end-to-end

#### B1 — Quickstart `install.yaml` pins `function-cuefn:v0`, a tag that does not exist *(cluster-install · CONFIRMED · p1)*
- **What:** Applying `install.yaml` verbatim leaves the Function `INSTALLED=False` / `HEALTHY=False` because `ghcr.io/meigma/function-cuefn:v0` cannot be resolved — only `:v0.1.0` and `:v0.1.1` are published.
- **Expected:** The Function installs (via a published moving `:v0` tag, or `install.yaml` pinned to `:v0.1.1`).
- **Repro:** `kubectl apply` a Function with `package: ghcr.io/meigma/function-cuefn:v0`; check `.status.conditions`.
- **Evidence:** `HEAD https://ghcr.io/v2/meigma/function-cuefn/manifests/v0 → 404 MANIFEST_UNKNOWN`; tags/list = `[v0.1.0, v0.1.1, +sha256 tags]`; cluster reason `UnpackingPackage`, message "cannot resolve ghcr.io/meigma/function-cuefn:v0 to digest ... MANIFEST_UNKNOWN". The string `:v0` appears exactly once in the docs (quickstart.md:230). Control: the env's working `:v0.1.1` tag installs `True/True` on the same cluster — only the tag is wrong.
- **Fix:** Publish a moving `:v0` tag at release, or change `install.yaml` to `:v0.1.1`.

#### B2 — Generated Composition's `cuefn` step references a Function name nobody installs *(instantiate · CONFIRMED · p1)*
- **What:** The generated pipeline step uses `functionRef.name: function-cuefn` (last path segment of `--function-ref`), but the quickstart `install.yaml` names the Function `cuefn`, and the Configuration's `dependsOn` auto-installs `meigma-function-cuefn`. Crossplane binds `functionRef` strictly by `metadata.name`, so following the quickstart verbatim breaks XR rendering at the final step. The hand-written `example/composition.yaml`, `example/functions.yaml`, and `example/deploy/functions.yaml` all use `cuefn`, so example and generated output also disagree.
- **Expected:** `cuefn publish` defaults the step's `functionRef.name` to the installed Function name, or exposes `--function-name` *and* the docs name the Function consistently.
- **Repro:** `gzip -dc out.gz | grep -A2 'step: cuefn'` → `name: function-cuefn`; compare to the installed Function name.
- **Evidence:** A Composition referencing `function-cuefn` succeeded only because the shared cluster's Function happens to be named `function-cuefn`; the isolated negative test referencing `cuefn` failed: `cannot get gRPC client connection for Function "cuefn": cannot find an active FunctionRevision`.
- **Fix:** Align the generated function name with what the install instructions create; document one canonical name.

#### B3 — `function-environment-configs` step emitted but omitted from `dependsOn` *(publish-config · CONFIRMED · p1, p3, p5)*
- **What:** `cuefn publish` *always* wires `function-environment-configs` as the first pipeline step (even with no `--environment-config`), but the generated Configuration's `spec.dependsOn` lists only the `cuefn` function. Crossplane only auto-installs `dependsOn` entries, so the env-configs function is never installed and the first XR reconcile fails. The documented install path never installs it (Step 6 calls the EnvironmentConfig "Optional").
- **Expected:** Gate the env-configs step on `--environment-config`, or add `function-environment-configs` to `dependsOn` unconditionally; document it.
- **Repro:** `cuefn publish <mod> --package ...` (no `--environment-config`); `crossplane xpkg extract` → pipeline has `function-environment-configs` but `dependsOn` lists only `ghcr.io/meigma/function-cuefn`. Apply Configuration + XR → reconcile fails.
- **Evidence:** `cannot run Composition pipeline step "function-environment-configs": ... cannot find an active FunctionRevision` (verbatim, reproduced fresh; the fresh package digest `10d0a1ebd38a` was byte-identical to p1's failing in-cluster ConfigurationRevision). On both clusters `function-environment-configs` had **empty `ownerReferences`** — it was only ever present via a persona workaround, never auto-pulled.
- **Fix:** Gate the step on `--environment-config` or add it to `dependsOn`; document in quickstart Step 5.

> **Why these three are blockers:** they sit squarely on the headline tutorial, hit 100% of doc-literal users, and B2/B3 surface errors naming Functions the docs never told the user to install — which is what lured p1 into the package-lock brick (F-M9 below). p1 followed the quickstart faithfully and **never reached a Ready XR**.

---

### HIGH — the as-shipped function cannot render without undocumented operator work

#### H1 — No documented way to route the installed function at a non-central registry *(reconcile · CONFIRMED · p1, p3, p4, p5, p6)*
- **What:** The in-cluster function fetches the module from its own `CUE_REGISTRY` at render time and defaults to the CUE Central Registry. Any module on a non-central registry (ttl.sh, private, local — i.e. every real team) fails to reconcile. The words `DeploymentRuntimeConfig` / `runtimeConfig` / `runtimeConfigRef` / `package-runtime` appear **nowhere** in the docs; the only `CUE_REGISTRY` guidance is for local CLI commands. The Input has no registry field, so the function's env var is the only knob.
- **Expected:** A copy-pasteable "point the function at your registry" how-to (DRC setting `CUE_REGISTRY` in prefix form on `package-runtime`, bound via `runtimeConfigRef`), cross-linked from the quickstart install step, with "module not found" in troubleshooting.
- **Repro:** Publish to a non-central registry; install Function with default (empty) DRC; apply XR. `grep -rn DeploymentRuntimeConfig docs/docs` → no matches.
- **Evidence:** Case A (`CUE_REGISTRY` unset → central): `module ttl.sh/...@v0.1.0: module not found`. Case B (`CUE_REGISTRY=ttl.sh`): renders, exit 0. The quickstart contradicts itself: it publishes to non-central `localhost:5000` (Step 3) but installs the Function with no `runtimeConfigRef` (Step 5). The shared cluster's working DRC sets exactly the prefix-form `CUE_REGISTRY` this finding asks to be documented.
- **Fix:** Ship the "point the function at your registry" how-to (prefix host/path form), cross-linked from quickstart + publish-configuration.

#### H2 — Installed function ships no writable cache → `mkdir /.cache: permission denied` *(reconcile · CONFIRMED · p1, p3, p4, p5)*
- **What:** The shipped package binds to a default DRC (`spec: {}`) with no `CUE_CACHE_DIR` and no writable volume; the container runs nonroot (uid 2000) on a read-only root fs, so CUE's cache resolves to `/.cache`, which cannot be created. Even with `CUE_REGISTRY` correct, **the as-shipped function cannot render even a central module.**
- **Expected:** The package defaults `CUE_CACHE_DIR`/`HOME` to a writable path with an `emptyDir`, or the install docs ship the exact DRC (env + emptyDir).
- **Repro:** Install with default runtime config, set `CUE_REGISTRY`, apply XR; `kubectl describe <xr>`.
- **Evidence:** `cannot create cache directory ... mkdir /.cache: permission denied`. Fixed by `CUE_CACHE_DIR=/tmp/cuefn-cache` + an `emptyDir`; render healed within one reconcile. The requirement is documented only in prose (reference/configuration.md, serve-function.md, README) — no doc ships a complete copy-paste DRC, and the package ships no working default. Quickstart Step 5 installs a bare `spec` and Step 6 expects render — so the headline tutorial hits this.
- **Fix:** Default `CUE_CACHE_DIR`/`HOME` to a writable path and ship an `emptyDir` in the package default runtime (highest-frequency first-render blocker — all four cluster personas hit it).

#### H3 — `generate` marks fully-defaulted nested structs as `required`, breaking apiserver apply *(generate · CONFIRMED · p2, p4)*
- **What:** `cuefn generate` puts defaulted fields into the XRD's `spec.required`. For top-level scalars this is mostly harmless (apiserver defaults before the required check). But for a nested struct whose every field is defaulted (e.g. `resources: {cpu: ...|*"250m", memoryMi: ...|*256}`), the XRD lists the struct in `required` with **no object-level default**, so the apiserver rejects `spec: {}` even though `cuefn validate`/`render` accept it — violating the documented no-drift guarantee.
- **Expected:** Emit an object-level default for any struct whose every field defaults (recursively) and/or omit CUE-defaultable fields from `required`.
- **Repro:** `cuefn validate xr-min.yaml --dir module` (`spec: {}`) → valid; `kubectl apply` / `--dry-run=server` → invalid.
- **Evidence:** `The WidgetThing "minimal" is invalid: spec.resources: Required value` (exit 1) while local tooling fills `cpu=250m, memoryMi=256`. Control: `spec: {resources: {}}` succeeds — the parent object must be present for nested defaults to apply. Directly contradicts module-contract.md:95-98 ("no drift between cluster-filled and render-filled values") and validate-xr.md:40-43 ("a spec that passes here will not be rejected for schema reasons at render time").
- **Fix:** Emit object-level defaults for fully-defaultable structs and/or drop them from `required`.

---

### MEDIUM

#### M1 — Function registry/cache is single shared global state (last-write-wins) *(reconcile · CONFIRMED · p3, p4, p6)*
- **What:** Every Composition routes through the one `function-cuefn` Function, so `CUE_REGISTRY` lives on a single `DeploymentRuntimeConfig/default`, not per-module. Multiple consumers must hand-merge prefixes into one comma-separated value; a naive `kubectl apply` with only one team's entry silently clobbers the others. No per-Composition/per-Input override exists.
- **Expected:** A documented merge-don't-replace convention + a footgun warning; ideally a per-Configuration/per-Input registry mechanism.
- **Repro:** Two consumers each `kubectl apply` a DRC/default from a stale read → last writer wins; the other's reconcile breaks "module not found".
- **Evidence:** Live repro — BEFORE = 4 prefixes; after a naive apply carrying only `db.dxtest.io=ttl.sh/00ddd364`, the stored value became exactly that one prefix; the other three were silently dropped (longest-prefix match then loses the mapping). Three personas independently authored a DRC/default. This is plain SSA-replace semantics on a shared cluster-scoped object.
- **Fix:** Document the merge pattern + the shared-global-mutable-state warning; consider per-Configuration/Input registry routing (a structural multi-tenancy gap, not just docs — see Feature F2).

#### M2 — Conditionless composed resource holds the XR at `Ready=False` forever *(reconcile · CONFIRMED · p1, p3, p4)*
- **What:** An absent `ready` hint maps to `Unspecified`; Crossplane's default readiness check finds no `Ready` condition on status-less kinds (ConfigMap/Secret/PVC), so the composite never reaches Ready — it sits at `Ready=False "Unready resources: <name>"` even though everything is provisioned. The shipped example leaves its ConfigMap hint absent, so the example as written never reaches XR Ready in-cluster.
- **Expected:** The contract/quickstart readiness section warns conditionless resources need `ready: "Ready"`; the example marks its ConfigMap.
- **Evidence:** XApp `demo` sat (25+ checks over minutes) at `Ready=False reason=Creating msg="Unready resources: config"` while `deployment/demo` was 2/2 Available and the ConfigMap existed with data. After adding `ready: "Ready"` and republishing v0.1.1 → `Ready=True reason=Available`. Docs only ever describe `Unspecified` as a benign third state (quickstart:172 "a ConfigMap marked Unspecified"); no "Unready"/"blocks"/"never ready" warning anywhere.
- **Fix:** Document that `Unspecified` blocks XR Ready; recommend `ready: "Ready"` for conditionless kinds; mark the example's ConfigMap.

#### M3 — Forgetting `out.input.spec: #Spec` silently disables render-time validation *(render · CONFIRMED · p2)*
- **What:** Docs call binding `out.input.spec: #Spec` "the key move," but omitting it is not flagged by `cue vet -c=false` (the contract's `#Input.spec` is open, `_`). `cuefn render` then accepts a fully out-of-bounds XR (`replicas: 99`, `maxMemoryMb: 99999`) and emits it verbatim with exit 0 — no bound enforcement, no `#Spec` defaulting — contradicting "a spec that violates `#Spec` fails the render" and "render matches what Crossplane renders in-cluster." (`validate` still works because it reads `#Spec` directly; the gap is the render/runtime path.)
- **Expected:** Fail closed (or warn) when `input.spec` is the open contract default; at minimum flag the footgun next to "the key move."
- **Evidence:** Correct module render → exit 1 with the `#Spec` bound error; broken module (binding deleted) → exit 0 emitting `99/99999`; `cue vet -c=false ./...` on the broken copy → exit 0 (does not catch it). Defaulting is also lost (`undefined field: port`). render-locally.md:56-60 states the guarantee unconditionally, with no mention it depends on the binding (which lives only on the quickstart/contract pages).
- **Fix:** Fail closed/warn when `input.spec` is the open default; flag the footgun in render-locally.md.

#### M4 — Composing native kinds beyond core workloads hits an RBAC wall *(reconcile · CONFIRMED · p4, p5)*
- **What:** `cuefn` renders the resource fine, but the core crossplane ServiceAccount has RBAC only for the few kinds the example composes (deployments/services/secrets/serviceaccounts/configmaps). Composing a StatefulSet or Ingress fails "forbidden ... cannot patch." The example masks this by using only pre-granted kinds; cuefn's pitch of "any Kubernetes kind" gives no warning.
- **Expected:** A note/how-to that composed native kinds beyond core workloads need the crossplane SA granted via an aggregated ClusterRole (`rbac.crossplane.io/aggregate-to-crossplane: "true"`), with example YAML.
- **Evidence:** `can-i` returns NO for cronjobs/jobs/daemonsets/replicasets/networkpolicies/PVCs/HPAs/PDBs, YES for deployments/services/secrets/serviceaccounts/configmaps (stock upstream contents, not persona pollution). End-to-end: `cuefn render` emitted a CronJob with zero warning, yet `can-i create cronjobs.batch` = no. Personas' literal events: `statefulsets.apps "orders" is forbidden ... cannot patch`, same for ingresses; `can-i` flipped no→yes after adding the aggregated ClusterRole. The only RBAC docs cover the required-resources *read* path, not the *write* path.
- **Fix:** Document the RBAC requirement + aggregation label with example YAML; consider generating the ClusterRole from the kinds a module composes (Feature F7).

#### M5 — Two Functions on the same package poison the package lock *(cluster-install · CONFIRMED · p1)*
- **What:** Adding a Function named `function-cuefn` while `meigma-function-cuefn` (same package) already exists produces "node ... already exists," marking **all** packages unhealthy. This is the trap a user falls into trying to reconcile the B2 name mismatch.
- **Expected:** The single-Function-per-package constraint is documented so users name the one Function correctly up front.
- **Evidence:** Lock `Resolved=False :: DependencyResolutionFailed :: ... node ghcr.io/meigma/function-cuefn already exists`; the unrelated `function-environment-configs` and every Configuration went `Healthy=False`. Clean and reversible: 1 node → healthy; +duplicate → lock broke + unrelated packages unhealthy; −duplicate → healthy within seconds. The constraint is undocumented (grep for "already exists"/"package lock"/"duplicate" finds nothing relevant).
- **Fix:** Document the constraint; resolving B2 removes the lure.

#### M6 — Generated env-configs step has no selector → EnvironmentConfig never merged *(reconcile · CONFIRMED · p1)*
- **What:** Distinct from B3 (the step is present but lacks a selector). The quickstart promises `tier=production` from an EnvironmentConfig, but the generated step is bare `- functionRef: {name: function-environment-configs}` with no `input:`, so nothing is merged and the field stays unset — with no error.
- **Expected:** `cuefn publish` emits the step *with* a selector when `--environment-config` is passed; the quickstart actually uses the flag (it currently does not).
- **Evidence:** Quickstart-literal Composition has no `input:`; the `--environment-config` variant emits `input.spec.environmentConfigs: [{ref: {name: app-environment}, type: Reference}]`. Local render: `out.input.environment.tier` = "unset" without an env, "production" with one. Quickstart Step 6 claims the tier "read production from the EnvironmentConfig," but Step 4 never passes the flag — a doc-literal user gets `tier=unset`. Hand-patching `environmentConfigs` into the live Composition flipped `tier` to production on both resources.
- **Fix:** Emit the step with a selector when wired; make the quickstart use `--environment-config` end-to-end (Coverage gap C10).

#### M7 — Noisy "empty disjunction" / "conflicting values" on defaulted-field violations *(validate · CONFIRMED · p2, p4, p5)*
- **What:** A bound/enum violation on a field that carries a default renders as "N errors in empty disjunction" + one "conflicting values \<default\> and \<value\>" line per disjunct. The `conflicting values 1 and 99` line (1 = the default) actively misleads ("you must use 1"); enum violations give one conflict line per allowed value with no "must be one of [...]" summary. The same violation on a no-default field is clean.
- **Expected:** A single message naming the bound or allowed enum values, with the default branch not surfaced as a conflict.
- **Evidence:** `replicas:99` (default `*1`) → "2 errors in empty disjunction: / conflicting values 1 and 99: / invalid value 99 (out of bound <=5)". Control no-default `maxMemoryMb=8` → single clean "invalid value 8 (out of bound >=16)". Apiserver for the same inputs: `Unsupported value: "13.0": supported values: "16.4", ...` and `should be less than or equal to 100` — single, clear. `cuefn generate` already emits the enum/bounds, so the info to phrase a clean message exists. validate-xr.md:37 frames the exact `replicas: 99` case as a clean single out-of-bounds message, and the repo's own example carries a default — so running the documented scenario yields the noisy output.
- **Fix:** Post-process CUE disjunction/bound errors into one enum/bounds message.

#### M8 — Forgetting `--dir` (major-only `@v0` over OCI) → opaque "not canonical" *(render · CONFIRMED · p2)*
- **What:** Every doc example uses major-only `@v0` *with* `--dir` and works. The natural first mistake — forgetting `--dir`, so the ref is fetched over OCI — gives "module version in `...@v0` is not canonical," which mentions neither `--dir` nor the real rule (an OCI fetch needs a full `@vX.Y.Z`; `--dir` accepts `@v0`). Same from `validate --module @v0`.
- **Expected:** Accept the major-only ref symmetrically, or an error naming the cause.
- **Evidence:** `--dir` form → exit 0; without `--dir` → `invalid module reference "...@v0" (want path@version): module version ... is not canonical` (misleading, since `path@version` *was* supplied). Control with `@v0.1.0` (no `--dir`) clears the canonical check and fails "not found in registry," proving the granularity is the gate. No doc explains the version-form rule.
- **Fix:** Accept major-only refs symmetrically, or emit an error naming the OCI full-version requirement.

#### M9 — `cue vet ./...` exits non-zero on a correct module; the working `-c=false` is undocumented *(docs · CONFIRMED · p2, p5, p6)*
- **What:** enforce-the-contract.md, the quickstart, README, and module-contract.md (4 places) tell authors to run `cue vet ./...`, but a correct module adopting `contract.#Transform` exits 1 with "some instances are incomplete; use the -c flag ... or -c=false" because the transform's `out`/`input` fields are intentionally non-concrete until an XR supplies values. A clean module looks broken.
- **Expected:** Docs use `cue vet -c=false ./...` with a one-line note on why bare `cue vet` is non-zero by design.
- **Evidence:** `cue vet ./...` → exit 1 "some instances are incomplete"; `cue vet -c=false ./...` → exit 0, and the negative control (misspelled `out.resorces`) still flags "field not allowed." `-c=false` appears nowhere in docs/README. CUE's own `cue vet --help` uses `cue vet -c=false ./...` as its example.
- **Fix:** Update all four docs to `cue vet -c=false ./...` with a one-sentence explanation.

#### M10 — Optional map field has no clean idiom *(authoring · CONFIRMED · p5)*
- **What:** The three natural map shapes each break: (1) `env: [string]: string` → XRD marks `env` `required` with no default, so every XR must include `env: {}`; (2) `env?: [string]: string` → render fails when env is omitted ("cannot reference optional field: env"), breaking at reconcile not apply; (3) `env: {[string]: string} | *{}` → `cuefn generate` fails ("type-crossing disjunction at Spec.env ... cannot be a Kubernetes structural schema"). A clean idiom exists (`env?:` in `#Spec` + re-assert `spec: #Spec & {env: [string]: string}` in the transform input) but nothing documents it.
- **Expected:** Codegen does not mark a pure pattern-constraint map (no concrete required entries) as required; docs add a maps/lists section.
- **Evidence:** (1) `spec.env: Required value` on apply, `spec: {env: {}}` accepted; (2) `out.resources.config.object.data: cannot reference optional field: env`; (3) verbatim type-crossing error. All reproduced. (Minor correction to the original finding: the re-assert is not the *only* clean shape — `_env: *input.spec.env | {}` and an `if` guard also work — but all are equally undocumented and non-obvious to a YAML-native author.)
- **Fix:** Treat maps with no required keys as non-required in the XRD; document the optional + input re-assert idiom.

#### M11 — Quickstart assumes a local registry on :5000 but never tells you to start one *(publish-module · CONFIRMED · p1)*
- **What:** Step 3 targets `cue mod publish` at `localhost:5000`, but no registry runs on a clean machine and the step never says to start one.
- **Expected:** Quickstart includes a command to run a local registry (and connect it to the kind network).
- **Evidence:** Against an empty port: "cannot make scratch config: ... connection refused"; positive control against a started registry: "published ... to localhost:5001 (exit 0)." `localhost:5000+insecure` appears in 6 doc files but no doc contains `docker run ... registry:2`.
- **Fix:** Add `docker run -d -p 5001:5000 registry:2` (+ `docker network connect kind`).

---

### LOW (CONFIRMED)

#### L1 — `localhost:5000` collides with macOS AirPlay Receiver → misleading 403 *(publish-module · CONFIRMED · p1)*
Step 3's verbatim publish to `localhost:5000` returns `403 Forbidden` because macOS ControlCenter/AirPlay owns port 5000. **Evidence:** `curl http://localhost:5000/v2/ → 403, Server: AirTunes/940.23.1`; `cue mod publish → cannot make scratch config: 403 Forbidden` (empty body matches AirTunes). The error names neither AirPlay nor the port collision. **Fix:** Use port 5001 in examples (also fixes M11's clean-machine case) or add a macOS AirPlay note.

#### L2 — No external install path for the `cuefn` CLI *(install · CONFIRMED · p1)*
The only acquisition guidance is build-from-source (`mise install` / `go build ./cmd/cuefn`) — no `go install`, release binary, or crossplane-CLI link. **Evidence:** `grep -rni "go install" docs/ README.md` = 0 hits; only crossplane URL is the homepage. But the repo is public with public tags, and `go install github.com/meigma/crossplane-cuefn/cmd/cuefn@latest` works (exit 0, all subcommands). **Fix:** Add the `go install ...@latest` path (or release binaries) and a crossplane-CLI link to prerequisites.

#### L3 — Omitting a required no-default field reports "incomplete value" *(validate · CONFIRMED · p2)*
validate-xr.md lists "a missing required field with no default" as a typical violation, but omitting required `maxMemoryMb` produces `#Spec.maxMemoryMb: incomplete value >=16 & <=4096 & int` — CUE jargon that echoes the bound and reads like a bounds problem, not "you forgot this field." `grep -rni incomplete docs/` returns nothing. **Fix:** Emit "required field not set" for omitted no-default fields, or document the mapping.

---

### Unverified low/nit observations (worth a glance, not yet reproduced)

- **registry.example.com placeholder** (p1) — must be replaced with a registry that is HTTPS *and* cluster-reachable *and* distinct from the CUE registry; none obvious. Suggest a concrete `ttl.sh/<name>:<ttl>` target.
- **function-environment-configs transient "missing required capabilities: composition"** (p3) — during revision activation; self-resolves, not cuefn-specific. Suggest pinning a known-good version in the quickstart.
- **No debug recipe for required-resources "silent under-render"** (p6) — the missing-RBAC empty-bucket case leaves a Ready-ish XR with a missing resource and no debug steps. Suggest a debugging checklist.
- **(nit) Every validate/render error printed twice** (p2) — summary line + full CUE tree; byte-identical for single-error cases, reads like a duplication bug.
- **(nit) Bogus positional module-ref accepted with `--dir`** (p2) — `cuefn render totally/wrong@v9 --dir redis ...` renders; documented, but a stale/typo'd ref passes silently.
- **(nit) `cuefn --version` reports "dev (none) built unknown"** (p3, p4) — on a release-commit binary; likely a build-flags artifact of the assessment binary, but consumers can't tell which build they run.
- **(nit) Generated XRD omits `additionalProperties: false`** (p4) — unknown spec fields are pruned server-side instead of rejected; strictness drift from the closed `#Spec` (only kubectl's client-side strict decoding caught it).
- **(nit) `CUE_REGISTRY` prefix form with a ttl.sh-domain module doubles the repo path** (p6) — `ttl.sh=ttl.sh` + module path `ttl.sh/<name>` pushes to `ttl.sh/ttl.sh/<name>`. Works, but surprising.

---

## 4. What works well (genuine positives)

These were reported consistently and are the reason the verdict is "close," not "no."

- **Local inner loop is excellent and accurate.** `cuefn render --dir` output matched the in-cluster render *exactly* (same engine) for every cluster persona — defaults, bounds, map→list comprehensions, and `if` conditionals all behaved identically locally and in-cluster (p1, p3, p4, p5). p2 called it "a genuinely pleasant cluster-free authoring loop."
- **Zero-config dependency resolution.** `cue mod tidy` pulled `cue.dev/x/k8s.io` and the contract from the CUE Central Registry with no `CUE_REGISTRY` set, every time (all personas). p6 saw it auto-bump the contract to v0.2.0 with no manual pinning.
- **The contract catches real authoring mistakes.** Misspelled `out` keys, typo'd resource fields (`out.resorces`), invalid `ready` hints, and out-of-envelope `#API` keys are all rejected with precise file:line "field not allowed" (p2, p5).
- **XRD codegen is high quality.** Bounded ints → `minimum`/`maximum`, same-type string enums → `enum`, regex → `pattern`, nested objects with per-field bounds/defaults, status schema, and CUE doc comments → OpenAPI descriptions, all faithful (p2, p4, p5). The `generate` type-crossing-disjunction error is exemplary ("names the field, gives an example, explains the cause" — p2's recorded model for what the other messages should aim for).
- **Packaging + digest lockstep are real and transparent.** `cuefn publish` records the module ref + resolved digest; the in-cluster function fetched and verified that exact digest (p1's `sha256:b9fbaa25...` matched). The lockstep survived a v0.1.0→v0.1.1 republish (p4).
- **ttl.sh works as both CUE-module and xpkg registry** over HTTPS with plain semver tags (p3, p4, p5); Crossplane pulled and installed Configurations in ~13s.
- **The prefix-with-path `CUE_REGISTRY` form keeps central as a fallback**, so one mapping serves root-from-ttl.sh + public-deps-from-central — and let three personas share one function runtime without conflict (p3, p4, p5).
- **The required-resources feature matched its docs with zero source-peeking** (p6): `cuefn render` always echoes emitted `requirements` (great "what do I need to provide?" discoverability), `--required-resources <file>` matched by selector and ran the real two-pass fixpoint offline, and the how-to's aggregate ClusterRole shape matched reality.
- **Apiserver-side error messages are clearer than the CLI's** for enums/bounds — a positive to mirror in M7.
- **Crossplane `>=v0.0.0` dependency resolution and additive DRC env-merge** behaved correctly (controller-injected env like `TLS_SERVER_CERTS_DIR` was preserved when personas added their own — p4).

---

## 5. Coverage gaps (what a future sweep should exercise)

None of the six journeys touched day-2 lifecycle, multi-tenancy isolation, or the self-hosted function path. Ranked by adoption risk:

1. **Day-2 deletion / teardown / GC ordering.** *No persona deleted anything* — every journey ends at `Ready=True`. Delete an XR composing a StatefulSet + PVC and confirm GC vs. PVC retention; delete a Configuration / uninstall `function-cuefn` while a live XR exists and check for finalizer hangs. (Teardown is half the lifecycle and the part most likely to leak/destroy data.)
2. **Schema-changing upgrades & multiple API versions.** Only patch bumps (v0.1.0→v0.1.1, hint-only) were run. Republish adding a required-no-default field + tightening a bound, upgrade the Configuration under a stored XR, and bump to a second served XRD version.
3. **Live XR spec mutation (day-2 update / drift).** Every XR was create-once. `kubectl edit` a Ready postgres XR (`storageGi` 20→40 = immutable PVC; `version`; benign `replicas`) and observe how cuefn surfaces partial/immutable-field rejections during reconcile.
4. **Crossplane v2 claims & cluster-scoped (`scope: Cluster`) XRs.** All six XRs were directly-created Namespaced XRs; no claim, no cluster-scoped XR composing namespaced resources.
5. **Private transitive OCI module deps (module-imports-module).** Every module was self-contained. The digest lock covers only the *root* module — a republished private transitive dep is not digest-checked and must resolve via `CUE_REGISTRY`. Both implications untested.
6. **Authenticated / private HTTPS registries** (pull secrets, CUE-registry auth, private CA/TLS). Only ttl.sh (anon) and localhost+insecure were used. How the in-cluster function authenticates a private CUE pull is undocumented and untested — the gating question for any enterprise install.
7. **Air-gapped / central-unreachable cluster.** Central was always reachable, even in-cluster. With central removed, the prefix form's catch-all disappears — on current evidence (F-bare-registry) this path is fragile.
8. **In-cluster observability** — function pod logs, Prometheus `:8080` metrics, `-d/--debug` via the DRC, `--metrics-address ""`. Every cluster persona blind-debugged from XR events alone.
9. **Function version pinning & upgrading `function-cuefn` under live Configurations.** Everyone used floating `>=v0.0.0`; the digest lock guards the *module*, not the function.
10. **Proper `--environment-config` wiring end-to-end** (see M6) — never exercised correctly; p1 hand-patched, p3/p5 just installed the function to satisfy the pipeline.
11. **Connection-detail / secret propagation to the XR** (`writeConnectionSecretToRef`). p4 composed a credentials Secret but couldn't surface it as a connection secret — the entire point of a DB/cache platform. Likely a stateful-platform adoption blocker if unsupported.
12. **`publish-function` / building & shipping a custom function image.** Zero personas; the whole self-hosted/air-gapped function workflow (multi-arch, embedded Input CRD, signing) is unvalidated outside-in.
13. **Multi-tenant runtime isolation** — can two teams get independent runtime config (separate Functions with distinct `runtimeConfigRef`)? Untested; today they collide on one DRC (M1).
14. **Applying generated XRD + Composition as raw manifests via GitOps** (Argo/Flux), bypassing the xpkg/package-manager path — a common deployment model that would sidestep B2/B3, but untested and undocumented.

---

## 6. Feature ideas (ranked by value)

**High value**

1. **Function image defaults to a writable cache (no DRC required to render).** Fall back to `/tmp/cuefn-cache` when the OS cache dir is uncreatable, and/or ship the package default runtime with an `emptyDir`. *Rationale:* the highest-frequency first-render blocker — all four cluster personas hit `mkdir /.cache: permission denied` (H2). A fresh install should render before any operator writes a DRC.
2. **Per-Configuration / per-Input registry routing instead of one global `CUE_REGISTRY`.** Let a Configuration or the cuefn Input declare its own module registry/prefix. *Rationale:* today every Composition funnels through one shared DRC env var that every team must hand-merge, with a built-in lost-update race (M1). This is the difference between a single-team demo and a multi-tenant platform.
3. **Add `function-environment-configs` to the generated `dependsOn` (or gate the step on `--environment-config`).** *Rationale:* pure correctness — blocks the documented happy path on a fresh cluster for every Configuration-installing persona (B3).
4. **Fix XRD generation for fully-defaulted (incl. nested) structs.** Emit an object-level default for any recursively-defaultable struct and/or drop fully-defaultable fields from `required`. *Rationale:* restores the documented no-drift guarantee the apiserver currently violates (H3).
5. **Ship the in-cluster `DeploymentRuntimeConfig` recipe (and optionally `cuefn publish` emits one).** A copy-pasteable "point the function at your registry" how-to (`CUE_REGISTRY` prefix + `CUE_CACHE_DIR` + `emptyDir`), cross-linked from the quickstart. *Rationale:* the single largest doc gap — DRC/`runtimeConfigRef` appear nowhere yet every cluster persona had to reverse-engineer it (H1, H2).

**Medium value**

6. **`cuefn doctor` / preflight diagnostics.** Inspect the cluster and report in one shot: is `function-cuefn` healthy, does the DRC route this module's registry, is the cache writable, is `function-environment-configs` installed, does the crossplane SA have RBAC for the composed kinds. *Rationale:* collapses the multi-hour blind-debug loops every cluster persona ran into seconds.
7. **Generate the aggregated ClusterRole for composed kinds.** cuefn statically knows which kinds a transform composes; emit (via a publish/generate flag) a ClusterRole labelled `rbac.crossplane.io/aggregate-to-crossplane:true` granting exactly those. *Rationale:* turns the M4 surprise reconcile wall into a generated manifest.
8. **Collapse CUE disjunction/bounds noise into single human errors in `validate`.** `generate` already has the enum/bounds to phrase "must be one of [...]" / "out of bound (>=x & <=y)". *Rationale:* three personas flagged the misleading "conflicting values \<default\>" noise (M7).
9. **An author-time `cuefn check`/`lint` that wraps the correct vet and catches the missing `#Spec` binding.** Runs `cue vet -c=false` and warns when `out.input.spec` is still the open contract default. *Rationale:* closes both the M9 false-broken trap and the M3 silent-validation-disable footgun.
10. **Publish a rolling `:v0` tag and align the quickstart `install.yaml` / function name.** *Rationale:* unblocks the verbatim quickstart (B1, B2).
11. **Document/support connection-detail propagation for stateful platforms.** *Rationale:* without it the postgres/redis "successes" stop one step short of usable (Coverage C11).

**Low value**

12. **Stamp version/commit/date on distributed builds** — removes a universal trust/debuggability paper cut (`cuefn --version` → "dev (none) built unknown").
13. **Emit `additionalProperties: false` and a clean optional-map idiom in generated XRDs** — fixes the server-side-pruning strictness drift (nit) and the M10 map ergonomics.

---

## 7. Per-persona outcome

| Persona | Reached Ready XR? | Headline friction |
|---------|-------------------|-------------------|
| **p1-doclit** (quickstart verbatim, fresh cluster) | **No** | The documented path is broken end-to-end: `:v0` 404 (B1), function-name mismatch (B2), env-configs missing from `dependsOn` (B3), then duplicate-Function lock brick (M5), no registry routing/cache (H1/H2); even after ~8 workarounds the ConfigMap held the XR at `Ready=False` forever (M2) and `tier=production` needed a hand-patched selector (M6). Local Steps 1–4 were clean. |
| **p2-local** (local-only, no cluster) | **n/a** | Reached its goal of a pleasant local loop. Three first-touch traps: `cue vet ./...` reports a correct module broken (M9), forgetting `out.input.spec: #Spec` silently disables render validation (M3), and noisy CUE jargon errors (M7, M8, L3). |
| **p3-redis** | **Yes** (`RedisCache` Ready/Available) | Author + packaging worked first try; in-cluster Ready required three undocumented fixes — install `function-environment-configs` (B3), set `CUE_REGISTRY` (H1), give the function a writable cache (H2) — plus a `ready: "Ready"` hint on the ConfigMap (M2). |
| **p4-postgres** (rich schema) | **Yes** (`PostgresInstance` Synced/Ready, StatefulSet+Service+Secret+20Gi PVC) | One correctness drift (nested-struct defaulting, H3) and three ops gaps the docs miss: shared last-write-wins DRC (M1), non-writable cache (H2), and missing RBAC for composed StatefulSets (M4). |
| **p5-webapp** (multi-kind + Ingress) | **Yes** (both `shop` and `blog` XRs Synced/Ready) | Empty shipped DRC (H1/H2), env-configs not in `dependsOn` (B3), crossplane SA lacked Ingress RBAC (M4), no clean optional-map idiom (M10), and the `cue vet` flag trap (M9). |
| **p6-configreader** (required resources) | **Yes** (`ConfigReaderApp` Ready/Synced, fixpoint delivered config) | The required-resources contract matched docs with zero source-peeking; main friction was the undocumented `CUE_REGISTRY` routing (H1, effectively a blocker for non-central modules) and the shared-DRC lost-update clobber (M1). |

**Bottom line:** 4/6 reached a Ready XR, but only by independently rediscovering the same undocumented `DeploymentRuntimeConfig` (registry + writable cache) — and the one persona who trusted the docs reached nothing. Fix the three blockers, ship a working default function runtime (or its documented DRC), and this is a credible "Ready-with-caveats."
