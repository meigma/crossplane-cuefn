# How to publish a Configuration

`cuefn publish` builds an installable Crossplane **Configuration** package from
one CUE module in a single command: it generates the XRD, builds a pipeline
Composition wired to the cuefn function, records the module's resolved digest,
and pushes the package.

## Publish both artifacts from local source

Point CUE at the module registry, check that dependencies are tidy, then ask
cuefn to publish the local module and Configuration as one transaction:

```sh
export CUE_REGISTRY=registry.example.com
cue mod tidy --check

cuefn publish cuefn.example/app@v0.1.0 \
  --dir . \
  --publish-module \
  --metadata org.opencontainers.image.source=https://github.com/example/platform-modules \
  --package packages.example.com/xapp-configuration:v0.1.0
```

cuefn prepares and validates both artifacts before mutation. It publishes the
CUE module first, re-resolves its manifest digest, then pushes the Configuration.
An identical retry reuses the module version; if that version already points at
different bytes or metadata, cuefn rejects it instead of moving the tag.

`source.kind: "self"` includes the module directory as CUE normally does.
`source.kind: "git"` requires a clean Git worktree (including linked worktrees),
publishes tracked module files only, records the HEAD revision/time, and inherits
a tracked repository-root `LICENSE` for a nested module.

The repeatable `--metadata key=value` flag uses the first `=` as the separator.
In combined mode the same map becomes CUE module manifest annotations and
Configuration image-config labels. Metadata is public artifact data; do not put
credentials or other secrets in it.

## Use an already-published module

Omit `--publish-module` to preserve the existing workflow. The module must
already exist because cuefn resolves its live registry digest:

```sh
cuefn publish cuefn.example/app@v0.1.0 \
  --package registry.example.com/xapp-configuration:v0.1.0
```

With this form, `--metadata` labels only the Configuration. cuefn does not alter
an existing module artifact it does not own. If you also pass `--dir`, make sure
that local source is exactly what was published; otherwise prefer the combined
form above.

The destination `--package` registry must serve **HTTPS** — Crossplane's package
manager pulls Configurations over HTTPS only. (The CUE module registry can be
anything, including plain HTTP.)

On success: `pushed registry.example.com/xapp-configuration:v0.1.0`.

## Common flags

| Flag | Use |
|------|-----|
| `--function-ref` | The cuefn Function package the Configuration depends on (default `ghcr.io/meigma/function-cuefn`). |
| `--function-name` | In-cluster Function name the `cuefn` step references. Defaults to the name Crossplane derives for the `dependsOn` Function, so a single install resolves; override only if you install the Function under a different name. |
| `--function-version` | Semver constraint for that dependency (default `>=v0.0.0`). |
| `--environment-config` | Name of an EnvironmentConfig to merge (repeatable). Supplying any adds the `function-environment-configs` step and declares it in `dependsOn`. |
| `--environment-config-selector` | Comma-separated `labelKey=compositeFieldPath` pairs selecting exactly one EnvironmentConfig per composite by labels (repeatable). Each occurrence adds a Single-mode `Selector` source merged after the references. |
| `--environment-config-function-ref` / `--environment-config-function-version` | Override the env-config function package/version recorded in `dependsOn` (defaults to crossplane-contrib's `function-environment-configs`). |
| `--name` | Configuration `metadata.name` (default `<plural>-configuration`). |
| `--crossplane-constraint` | Restrict the supported Crossplane version. |
| `--dir` | Build the XRD/Composition from a local directory. |
| `--publish-module` | Publish the local `--dir` module before the Configuration, using the prepared digest in the Composition. |
| `--metadata` | Add repeatable `key=value` OCI metadata; combined publication applies it to both artifacts. |
| `--insecure` | Push over plain HTTP to a throwaway dev registry. |

## Verify the package

`crossplane xpkg extract` parses the full package stream (there is no `inspect`
or `validate` subcommand in Crossplane 2.3.3):

```sh
# --from-daemon reads from the local Docker daemon, so pull the pushed package first
docker pull registry.example.com/xapp-configuration:v0.1.0
crossplane xpkg extract --from-daemon registry.example.com/xapp-configuration:v0.1.0 -o out.gz
```

Install just the Configuration — its `dependsOn` auto-installs the function(s) —
then instantiate XRs; see the [Quickstart](../quickstart.md). To point the
in-cluster function at a non-central module registry, see
[configure the function runtime](configure-the-runtime.md). Full flag list:
[CLI reference](../reference/cli.md#publish).
