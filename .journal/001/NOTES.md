---
id: 001
title: Session kickoff
started: 2026-06-27
---

## 2026-06-27 21:28 — Kickoff
Goal for the session: not yet stated. Session opened via `/session-new`; awaiting the developer's first request.
Current state of the world: fresh repository `meigma/crossplane-cuefn` at initial commit (`dc4068a`) on `master`. Top-level holds a Go project scaffold (`cmd/`, `internal/`, `go.mod`), build/release tooling (`apko.yaml`, `melange.yaml`, `mise.toml`, `moon.yml`, `release-please-config.json`), docs, and the session/journal protocol. Journal branch `journal/jmgilman` initialized. No prior sessions.
Plan: wait for the developer to state a goal, then record it and proceed.

## 2026-06-27 21:30 — Goal: ramp on Crossplane v2 + CUE before build brief
Developer asked me to familiarize myself with two technology areas before they explain what we're building:
- Crossplane v2: composite functions (Go), Compositions, Configurations.
- CUE: basics (types, constraints, structs) and CUE modules (publishing/consuming via a container/OCI registry).
The repo name (`crossplane-cuefn`) strongly hints we're building a Crossplane composition function that uses CUE. Doing grounded research now; the actual build brief comes next.

## 2026-06-27 21:45 — Ramp complete; key findings
Loaded skills `git`, `worktrunk`, `cue`. Repo base is the `meigma/template-go` scaffold (module path still `github.com/meigma/template-go`; Cobra/Viper CLI; mise-pinned tooling: go 1.26.4, golangci-lint, moon, melange 0.54, apko 1.2.19, cosign 3.1.1; melange+apko+cosign supply-chain for signed Wolfi apk → OCI image). CUE v0.16.1 available locally. No crossplane CLI installed yet.

Current versions: Crossplane **v2.3** (docs.crossplane.io/latest), function-sdk-go uses `proto/v1` (GA) + `crossplane-runtime/v2`.

Crossplane v2 essentials:
- XRs are **namespaced by default**; XRD `apiVersion: apiextensions.crossplane.io/v2` adds `scope: Namespaced|Cluster|LegacyCluster`. **Claims are gone** (LegacyCluster keeps v1 behavior). XRs put machinery under `spec.crossplane` (compositionRef, resourceRefs, etc.).
- XRs can compose **any** resource (MRs, native k8s, third-party CRDs). MRs can be namespaced.
- **Native patch-and-transform is removed** — Compositions are `mode: Pipeline` only, a sequence of `pipeline:` steps each with a `functionRef` (+ optional `input`), passing desired state and shared `context` down the chain.
- Composition is still `apiextensions.crossplane.io/v1`, `compositeTypeRef` binds it to an XRD kind.

Go composition function (function-sdk-go):
- Implement `RunFunction(ctx, *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error)`.
- `rsp := response.To(req, response.DefaultTTL)`; read with `request.GetObservedCompositeResource(req)`, `request.GetDesiredComposedResources(req)`, `request.GetInput(req, in)`; XR has typed getters `GetString`/`GetStringArray`/etc by field path.
- Build desired resources, `composed.From(typedMR)` → unstructured, `desired[resource.Name("...")] = &resource.DesiredComposed{Resource: cd}`, then `response.SetDesiredComposedResources(rsp, desired)`. Status/UX via `response.ConditionTrue/False(...).TargetComposite()`, `response.Warning(...)`, `response.Fatal(...)`.
- Input type lives in `input/v1beta1/`, kubebuilder markers + `go generate` (controller-gen deepcopy). `main.go`/`Dockerfile` are boilerplate. Tests: table-driven with `protocmp`, `resource.MustStructJSON`.
- Package: `docker build` runtime image → `crossplane xpkg build --package-root=package --embed-runtime-image=...` → `crossplane xpkg push`. Local test loop: `crossplane composition render` (no cluster), or `Development` runtime annotation.

Configurations: OCI packages bundling XRDs+Compositions; `crossplane.yaml` (`meta.pkg.crossplane.io/v1`, kind `Configuration`) with `spec.dependsOn` (Functions/Providers/Configurations + version constraints). The three package types: Provider, Function, Configuration.

CUE side:
- Modules: `cue.mod/module.cue`, `cue mod init --source=git <path>@v0`, `cue mod tidy`, `cue vet -c`, `cue export`. Publish to OCI: `cue login`, `CUE_REGISTRY=host/prefix`, `cue mod publish vX.Y.Z` (CUE module artifact, NOT a runnable image). Consume: `cue mod get`, `CUE_REGISTRY` mapping, imports + `cue mod tidy`.
- Go API (the CUE↔function crux): `cuecontext.New()`, `load.Instances()` + `ctx.BuildInstance()`; for registry-backed module deps use `modconfig.NewRegistry(nil)` wired into `load.Config{Registry: ...}`; `mod/modregistry` reads/writes CUE modules from an OCI registry.

Prior art (study, don't necessarily copy):
- `crossplane-contrib/function-cue` — runs inline CUE scripts; injects `#request: {observed, desired, context}`, CUE returns a RunFunctionResponse-shaped value. **Inline source only today**; has `fn-cue-tools` (openapi, extract-schema, package-script via `cue def --inline-imports`, cue-test). There's also an archived predecessor (`function-cue-archived`).
- `crossplane-contrib/function-kcl` — analogous KCL function; notably DOES load modules from OCI/Git. This is the likely differentiator if our goal is CUE-modules-from-a-registry.

Open question for the developer: what specifically is the build brief, and how does it differ from `function-cue` (my guess: first-class CUE **modules consumed from an OCI registry**, leveraging this repo's supply-chain tooling).


