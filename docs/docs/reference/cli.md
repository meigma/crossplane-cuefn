# CLI reference

`cuefn` is the operator CLI and the composition-function server. This page
describes every shipped subcommand: its inputs, flags, output, and exit
behavior.

## Invocation and global behavior

```
cuefn [command] [flags]
```

Running `cuefn` with no subcommand prints the value of `--message` and exits
zero; it does **not** start the function server. Use `cuefn function` to serve
(see [function](#function)).

- `--message <string>` — persistent flag on the root command; the line the bare
  `cuefn` invocation prints. Defaults to the build summary.
- `-v`, `--version` — prints `cuefn <version> (<commit>) built <date>` and exits.
- `-h`, `--help` — help for the command.
- `completion` and `help` are the standard Cobra-provided commands.

Every flag is also settable through a `CUEFN_*` environment variable (Viper,
prefix `CUEFN`, with `-` and `.` mapped to `_`). See
[Configuration & environment](configuration.md).

**Exit codes.** A command exits zero on success and non-zero when its `RunE`
returns an error. The root command sets `SilenceUsage` and `SilenceErrors`, so a
failure prints the error once to stderr with no usage dump.

## Shipped subcommands

The default build ships exactly eight subcommands:

| Command | Purpose |
|---------|---------|
| [`function`](#function) | Serve the composition function over gRPC. |
| [`render`](#render) | Render a module against an XR locally. |
| [`check`](#check) | Run a module's static health checks (fmt, vet, xrd). |
| [`test`](#test) | Run a module's txtar test cases. |
| [`generate`](#generate) | Generate a structural XRD from a module. |
| [`validate`](#validate) | Validate an XR's spec against a module's `#Spec`. |
| [`publish`](#publish) | Build and push a Configuration package. |
| [`publish-function`](#publish-function) | Build and push the Function package. |

The `noxpkg` build (the binary embedded in the runtime image) omits `publish`
and `publish-function` (all other commands, including `check` and `test`, are
present in both builds); see
[Lean runtime image](../explanation/noxpkg-split.md).

Three of those verbs sound alike but answer different questions:

| Command | Question it answers | Needs |
|---------|---------------------|-------|
| [`check`](#check) | Is the module statically healthy, and does its XRD still generate (and match its golden)? | Nothing but the module. |
| [`test`](#test) | Does the module render the right resources for these inputs? | txtar cases carrying XR instances. |
| [`validate`](#validate) | Will this one concrete XR be accepted by the module's schema? | One XR file. |

---

## function

```
cuefn function [flags]
```

Serves the cuefn Crossplane composition function over gRPC. It fetches each
pipeline step's CUE module from the OCI registry configured by `CUE_REGISTRY`,
evaluates it against the observed XR and merged `EnvironmentConfig`, and returns
the rendered objects as desired composed resources plus a patched composite
status. Crossplane connects over mTLS by default; pass `--insecure` for local
`crossplane render`.

The generic runtime image uses `/usr/bin/cuefn` as its entrypoint and `function`
as its default command, leaving other consumers free to replace the subcommand.
When `publish-function` assembles a Function xpkg, it moves that default command
into the package entrypoint and clears `Cmd`, so Crossplane's standard Docker
runtime can replace `Cmd` with flags such as `--insecure`.

| Flag | Default | Description |
|------|---------|-------------|
| `--network <string>` | `tcp` | Network to listen on for gRPC connections. |
| `--address <string>` | `:9443` | Address to listen on for gRPC connections. |
| `--tls-certs-dir <string>` | `$TLS_SERVER_CERTS_DIR` | Directory holding the server certs (`tls.key`, `tls.crt`) and the client CA (`ca.crt`). Defaults to the `TLS_SERVER_CERTS_DIR` environment variable, which Crossplane sets on the in-cluster runtime container, so the packaged Function serves mTLS with no extra flags. |
| `--insecure` | `false` | Serve without mTLS credentials (development only; ignores `--tls-certs-dir`). |
| `--cache-dir <string>` | _(empty)_ | Writable directory for the CUE module cache (defaults to `CUE_CACHE_DIR`, the OS cache, then a temp dir). |
| `--metrics-address <string>` | `:8080` | Address for the Prometheus metrics endpoint; pass an empty string (`--metrics-address ""`) to disable it. |
| `-d`, `--debug` | `false` | Emit debug logs in addition to info logs. |

**Output.** Long-running server; logs to stderr. Returns a non-zero exit only on
a startup failure (logger creation or serve error).

**Metrics.** `function-sdk-go` serves Prometheus metrics on `--metrics-address`
(default `:8080`), in addition to the gRPC `--address`. Pass
`--metrics-address ""` to disable the endpoint entirely — useful when `:8080`
would collide with another local service, or in-cluster where the metrics
listener is unwanted.

A freshly installed function needs no `--cache-dir`: it falls back to a writable
temp dir. Only a hardened read-only-root deployment requires one — see
[Configuration & environment](configuration.md).

---

## render

```
cuefn render <module-ref> --xr <file> [flags]
```

Evaluates a CUE module against an observed XR and optional environment, printing
the rendered composed resources and composite status as YAML. It is cluster-free
and `crossplane`-CLI-free, and reuses the same engine and loaders the served
function uses, so local output matches what Crossplane renders.

**Argument.** `<module-ref>` — the module to evaluate, in `path@version` form
(e.g. `cuefn.example/app@v0`). With `--dir` the ref still positions the call but
the bytes come from the directory.

| Flag | Default | Description |
|------|---------|-------------|
| `--dir <string>` | _(empty)_ | Serve the module from this local directory instead of fetching it over OCI. Transitive dependencies are resolved from the configured/default registry. |
| `--xr <string>` | _(required)_ | Path to the observed XR YAML. |
| `--env <string>` | _(empty)_ | Path to a merged environment YAML. Its top-level keys become `out.input.environment` in the module. |
| `--cache-dir <string>` | _(empty)_ | Writable directory for the CUE module cache (defaults to `CUE_CACHE_DIR`, the OS cache, then a temp dir). |
| `--required-resources <string>` | _(empty)_ | Path to a YAML file or directory of cluster objects matched against the module's emitted `out.requirements` selectors (mirrors `crossplane render --required-resources`). Multi-document files are split; objects are matched by selector, not keyed by filename. |
| `--observed-resources <string>` | _(empty)_ | Path to a YAML file or directory of raw observed composed objects (mirrors `crossplane render --observed-resources`). Each object must carry a non-empty `crossplane.io/composition-resource-name` annotation; that value becomes the stable map key. Multi-document files are split. |

For either resource flag, a directory contributes only its immediate `.yaml`
and `.yml` files; nested directories are ignored. Supplying a directory with no
immediate YAML files is an error, matching `crossplane render`.

**Output.** YAML to stdout: a `resources` map keyed by the author's resource
names, each entry carrying `object` (the rendered Kubernetes object) and `ready`
(`"True"`, `"False"`, or `Unspecified`), plus an optional top-level `status`.
When the module emits `out.requirements`, render also prints a `requirements` map
of the emitted selectors, so authors discover what to supply via
`--required-resources` even when they pass none. Exits non-zero if the XR cannot
be read, the module cannot be loaded, the spec violates `#Spec`, or
`resources`/`status` do not fully evaluate.

**Required resources.** When `--required-resources` is supplied and the module
emits requirements, render runs a fixed two-pass loop — render to discover the
selectors, match the supplied objects against them, then re-render with the
matches delivered under `out.input.requiredResources`. Because requirements must
be a pure function of stable inputs, this provably converges in two passes; if
the second pass emits different requirements, render fails with a stabilization
error rather than printing a bogus result (the same `requirements didn't
stabilize` outcome Crossplane produces in-cluster). See
[how to require cluster resources](../how-to/require-resources.md) and
[required resources and the fixpoint](../explanation/required-resources-fixpoint.md).

**Observed resources.** `--observed-resources` supplies point-in-time composed
object snapshots to modules that explicitly opt in to
`out.input.observedResources`. cuefn preserves each full object body and derives
its key from the standard composition-resource-name annotation; a missing, empty,
or duplicate key is an error. The object's physical `metadata.name` is not used
as the key. Modules that do not opt in render exactly as before. See
[derive readiness from observed resources](../how-to/derive-readiness-from-observed-resources.md).

---

## generate

```
cuefn generate <module-ref> [flags]
```

Loads a CUE module and emits the structural Crossplane v2
`CompositeResourceDefinition` generated from its `#API` envelope and
`#Spec`/`#Status` schemas. `.properties.status` is emitted only when the module
declares `#Status`.

**Argument.** `<module-ref>` — `path@version` (or any positioning value with
`--dir`).

| Flag | Default | Description |
|------|---------|-------------|
| `--dir <string>` | _(empty)_ | Serve the module from this local directory instead of fetching it over OCI. Transitive dependencies are resolved from the configured/default registry. |
| `--cache-dir <string>` | _(empty)_ | Writable directory for the CUE module cache (defaults to `CUE_CACHE_DIR`, the OS cache, then a temp dir). |
| `-o`, `--output <string>` | _(empty)_ | Write the generated XRD to this file instead of stdout. |

**Output.** XRD YAML to stdout, or to `--output`. Exits non-zero on a load
failure or when the module's `#Spec`/`#Status` contains a type-crossing
disjunction (a `DisjunctionError` naming the field). See the
[module contract](module-contract.md#authoring-guardrails).

---

## test

```
cuefn test [flags]
```

Runs the module's declarative test cases: one txtar file per case under the
module directory's `tests/` subdirectory, discovered as `tests/*.txtar` and run
in filename order. Each case supplies render inputs through named sections that
mirror `render`'s flags (`xr.yaml`, `environment.yaml`, `required.yaml`,
`observed.yaml`) and declares expectations: `want.cue` (partial CUE unified
with the normalized result), `want.yaml` (an exact machine-maintained golden),
or `error.txt` (substrings a failing render must report). Numbered step
sections (`1/observed.yaml`, `1/want.cue`, ...) replay readiness sequences
against successive observed snapshots. The section vocabulary is closed —
unknown section names are errors. See the
[test case format](test-cases.md) for the full contract and
[How to test a module](../how-to/test-a-module.md) for the authoring guide.

| Flag | Default | Description |
|------|---------|-------------|
| `--dir <string>` | `.` | Module directory; cases are discovered in its `tests/` subdirectory. The module is served from this directory (dependencies resolve from the configured/default registry). |
| `--run <string>` | _(empty)_ | Run only cases whose name (filename without `.txtar`) matches this regular expression. |
| `--update` | `false` | Rewrite drifted `want.yaml` goldens from the rendered output, then re-run so remaining failures still fail. Never touches `want.cue` or `error.txt`. Refused in CI mode. |
| `--fail-fast` | `false` | Stop after the first failing case. |
| `--ci` | `false` | CI mode: golden seeding and `--update` are refused, so a case without expectations or with golden drift fails. Auto-enabled when the `CI` environment variable is set (not `0`/`false`). |
| `--cache-dir <string>` | _(empty)_ | Writable directory for the CUE module cache (defaults to `CUE_CACHE_DIR`, the OS cache, then a temp dir). |

**Output.** One line per case — `PASS`, `FAIL`, `SEED` (a case with no
expectations had its rendered output written to a `want.yaml` section for
review), or `UPDATE` (`--update` rewrote a drifted golden) — with failure
details indented beneath (the case description, then each failure:
path-qualified CUE conflicts for `want.cue`, a line diff for `want.yaml`, the
render error for `error.txt` mismatches). A summary line follows:
`N passed, N failed, N seeded, N updated`.

**Exit.** Zero only when every selected case passes with no seeding. Failures,
freshly seeded goldens, an empty `tests/` directory, and a `--run` pattern
matching nothing all exit non-zero.

---

## check

```
cuefn check [flags]
```

Runs the module's static health checks — the instance-free counterpart to
[`test`](#test). Three units run in order and all are always reported:

- **fmt** — every `.cue` file under the module directory (including
  `cue.mod/module.cue`; hidden directories and the legacy
  `cue.mod/pkg`/`gen`/`usr` vendoring directories are skipped) is in the
  canonical form bare `cue fmt` produces. Unparseable files are errors, never
  skipped.
- **vet** — the module evaluates cleanly without requiring concreteness: the
  equivalent of `cue vet -c=false ./...`, the correct author-side vet (bare
  `cue vet` fails on required fields without defaults). A module that fails to
  load reports the load error under this unit.
- **xrd** — the structural XRD generates from `#API`/`#Spec`/`#Status`, with
  the same rejections as [`generate`](#generate) (type-crossing disjunctions,
  non-structural schemas). With `--xrd <file>`, the generation must also match
  that golden file.

The golden comparison ignores line endings and leading full-line `#` comments,
so a headerless file previously written by `cuefn generate --output` passes
unchanged. Goldens written by `check` itself (seeding, or `--update`) are
machine-owned and begin with a `# Generated by cuefn check; DO NOT EDIT.`
header naming the re-bless command.

| Flag | Default | Description |
|------|---------|-------------|
| `--dir <string>` | `.` | Module directory to check. The module is served from this directory (dependencies resolve from the configured/default registry). |
| `--xrd <string>` | _(empty)_ | Compare the generated XRD against this golden file. A missing file is seeded with the current generation (refused in CI mode). |
| `--update` | `false` | Rewrite a drifted XRD golden from the current generation. Refused in CI mode. |
| `--ci` | `false` | CI mode: golden seeding and `--update` are refused, so a missing or drifted golden fails. Auto-enabled when the `CI` environment variable is set (not `0`/`false`). |
| `--cache-dir <string>` | _(empty)_ | Writable directory for the CUE module cache (defaults to `CUE_CACHE_DIR`, the OS cache, then a temp dir). |

**Output.** One line per unit — `PASS`, `FAIL`, `SEED` (the golden was written
for review), or `UPDATE` (`--update` rewrote a drifted golden) — with failure
details indented beneath (the unformatted file list for `fmt`, the CUE error
for `vet`, the generation error or golden line diff for `xrd`). A summary line
follows: `N passed, N failed, N seeded, N updated`.

**Exit.** Zero only when all three units pass. Failures and a freshly seeded
golden exit non-zero.

---

## validate

```
cuefn validate <xr> [flags]
```

Reads an XR YAML file and checks its `spec` against the target module's `#Spec`
using the same CUE evaluation the runtime engine uses, applying `#Spec` defaults.

**Argument.** `<xr>` — path to the XR YAML file to check.

| Flag | Default | Description |
|------|---------|-------------|
| `--module <string>` | _(empty)_ | Module reference (`path@version`) to validate against when fetching over OCI. |
| `--dir <string>` | _(empty)_ | Serve the module from this local directory instead of fetching it over OCI. Transitive dependencies are resolved from the configured/default registry. |
| `--cache-dir <string>` | _(empty)_ | Writable directory for the CUE module cache (defaults to `CUE_CACHE_DIR`, the OS cache, then a temp dir). |

**Output.** On a valid (or defaulted-but-omitted) XR, prints `<path>: valid` to
stderr and exits zero. On the first violation (out-of-bounds, wrong enum,
missing required, failed pattern) it returns the error — naming the offending
field path — and exits non-zero.

---

## publish

```
cuefn publish <module-ref> --package <oci-ref> [flags]
```

The generate → package → push flow. It loads the module, generates
its XRD, resolves the module's live OCI manifest digest, builds a pipeline
Composition (the `cuefn` step, plus a leading `function-environment-configs` step
when `--environment-config` or `--environment-config-selector` is given) recording
the module ref **and** that digest,
assembles a Crossplane **Configuration** xpkg, and pushes it. Recording the
resolved digest is the author half of the
[digest lock-step](../explanation/digest-lockstep.md).

With `--publish-module`, cuefn first prepares the canonical CUE module artifact
from `--dir`, uses its exact digest in the Composition, publishes that immutable
module version, then pushes the Configuration. All validation and both artifact
builds finish before the first registry mutation.

**Argument.** `<module-ref>` — `path@version`.

| Flag | Default | Description |
|------|---------|-------------|
| `--package <string>` | _(required)_ | Destination OCI reference for the Configuration package. |
| `--dir <string>` | _(empty)_ | Build the XRD/Composition from this local module directory instead of fetching it over OCI. Without `--publish-module`, the digest is still resolved from the registry. |
| `--publish-module` | `false` | Publish the local `--dir` module version before pushing the Configuration. Requires `--dir`; an identical existing version is reused and a different digest is rejected. |
| `--metadata <key=value>` | _(none)_ | Add OCI metadata (repeatable). Labels the Configuration image config; with `--publish-module`, the same map also annotates the CUE module manifest. Splits on the first `=`, rejects duplicates and empty fields, and requires `org.opencontainers.image.source` to be an absolute HTTP(S) URL. Do not put secrets in metadata. |
| `--cache-dir <string>` | _(empty)_ | Writable directory for the CUE module cache (defaults to `CUE_CACHE_DIR`, the OS cache, then a temp dir). |
| `--function-ref <string>` | `ghcr.io/meigma/function-cuefn` | cuefn Function package OCI ref recorded in the Configuration's `dependsOn`. |
| `--function-name <string>` | _(the name Crossplane derives for the `--function-ref` dependency)_ | In-cluster Function resource name the Composition's `cuefn` step references. The default matches the name Crossplane gives the auto-installed `dependsOn` Function, so a single Configuration install resolves. |
| `--function-version <string>` | `>=v0.0.0` | Semver constraint for the cuefn Function dependency. |
| `--name <string>` | `<xrd-plural>-configuration` | Configuration package `metadata.name`. |
| `--crossplane-constraint <string>` | _(empty)_ | Optional semver constraint on the supported Crossplane version. |
| `--environment-config <string>` | _(none)_ | Name of an EnvironmentConfig the Composition merges into the pipeline context (repeatable). Each is referenced by name so its values reach the module under `out.input.environment`. Supplying any adds the `function-environment-configs` step and a second `dependsOn` entry. |
| `--environment-config-selector <string>` | _(none)_ | Label matchers selecting exactly one EnvironmentConfig per composite, as comma-separated `labelKey=compositeFieldPath` pairs (e.g. `a.io/name=metadata.name,a.io/namespace=metadata.namespace`). Repeatable; each occurrence adds one Single-mode `Selector` source after the `--environment-config` references, so its data merges over theirs. Single mode fails the render when zero or multiple EnvironmentConfigs match. |
| `--environment-config-function-ref <string>` | `xpkg.crossplane.io/crossplane-contrib/function-environment-configs` | Package ref recorded in `dependsOn` for the env-config function (only when an `--environment-config*` source is used). |
| `--environment-config-function-version <string>` | `>=v0.7.2` | Semver constraint for the function-environment-configs dependency. |
| `--insecure` | `false` | Push over plain HTTP (development only; for a non-loopback throwaway registry). |

**Output.** Prints `pushed <ref>` to stdout on success. With
`--publish-module`, it first prints `published module <module-ref>@<digest>` (or
`reused module ...` on an identical retry). Exits non-zero if the
module cannot be loaded, the XRD cannot be generated, the digest cannot be
resolved from the registry, or the push fails.

!!! warning "`--dir` footgun"
    With `--dir` but without `--publish-module`, the XRD and Composition are built
    from the **local** directory while `ExpectedDigest` is resolved from the
    **registry**. Publish the module first, or use `--publish-module` to bind both
    artifacts to the same prepared bytes.

Run `cue mod tidy --check` in the module directory before publishing. cuefn
validates the module archive through CUE's public APIs, but CUE's tidy checker is
not a public library API.

The Configuration package needs an **HTTPS** destination registry — Crossplane's
package manager is HTTPS-only. (The CUE module registry may be any OCI registry,
including a plain-HTTP local one.) See
[one module, two outputs](../explanation/one-module-two-outputs.md#two-registries).

---

## publish-function

```
cuefn publish-function --runtime-image <base> (--package <oci-ref> | --output <file>) [flags]
```

Assembles the cuefn **Function** xpkg — the package metadata plus the embedded
`Input` CRD — over one or more apko-built runtime image bases and pushes it (or
writes it locally). The package image **is** the runtime image plus a
`package.yaml` layer, so it both installs as a Crossplane Function and serves
`cuefn function`. Assembly preserves the runtime layers while moving the base's
default command into the package entrypoint and clearing `Cmd` for Crossplane's
runtime flags. A single `--runtime-image` produces a single-arch image; several
produce a multi-arch index (a Function package image must run on every node
arch). This command takes no positional arguments.

| Flag | Default | Description |
|------|---------|-------------|
| `--runtime-image <string>` | _(required, repeatable)_ | Runtime image base: a local OCI/docker tarball path or a registry ref. Repeat for a multi-arch index. |
| `--package <string>` | _(empty)_ | Destination OCI reference for the Function package. Required unless `--output` is set. |
| `--output <string>` | _(empty)_ | Write the assembled single-arch package to this local `.xpkg` file instead of pushing (e.g. for `crossplane xpkg extract --from-xpkg`). |
| `--name <string>` | `function-cuefn` | Function package `metadata.name`. |
| `--crossplane-constraint <string>` | _(empty)_ | Optional semver constraint on the supported Crossplane version. |
| `--capabilities <string>` | _(empty, repeatable)_ | Optional package capability strings. |
| `--insecure` | `false` | Push/pull over plain HTTP (development only; for a non-loopback throwaway registry). |

**Output.** Prints `pushed <ref>` (or `wrote <path>` for `--output`) on success.
Requires either `--package` or `--output`. `--output` writes a single-arch
package and rejects more than one `--runtime-image`.
