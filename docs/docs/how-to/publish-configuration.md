# How to publish a Configuration

`cuefn publish` builds an installable Crossplane **Configuration** package from
one CUE module in a single command: it generates the XRD, builds a pipeline
Composition wired to the cuefn function, records the module's resolved digest,
and pushes the package.

## Prerequisite: publish the module first

The Composition records the module's **registry** digest, so the module must
already be published to its OCI registry before you build the Configuration:

```sh
export CUE_REGISTRY=registry.example.com
cue mod publish v0.1.0      # from the module directory
```

!!! warning "Order matters"
    Publish the module with `cue mod publish` **before** `cuefn publish`. With
    `--dir`, the XRD/Composition are built from the local directory but the
    digest is resolved from the registry — running out of order can ship a
    package whose digest does not match the local source.

## Publish the Configuration

```sh
cuefn publish cuefn.example/app@v0.1.0 \
  --package registry.example.com/xapp-configuration:v0.1.0
```

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
| `--environment-config-function-ref` / `--environment-config-function-version` | Override the env-config function package/version recorded in `dependsOn` (defaults to crossplane-contrib's `function-environment-configs`). |
| `--name` | Configuration `metadata.name` (default `<plural>-configuration`). |
| `--crossplane-constraint` | Restrict the supported Crossplane version. |
| `--dir` | Build the XRD/Composition from a local directory (digest still from the registry). |
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
