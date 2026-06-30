# How to enforce the module contract

The module contract — the `#API` envelope and the `out` transform shape — is
published as a small CUE module of **closed** definitions:
`github.com/meigma/crossplane-cuefn/contract@v0`. Importing it and unifying your
module against it validates the shape at author time (`cue vet` and your editor),
so a misspelled or unknown field is caught before you ever render or publish.

It is optional: a module that writes the plain shape (`#API: {…}`, `out: {…}`)
works exactly the same at runtime. Importing the contract adds the author-time
guarantee.

## Import and unify

Add the import and unify the two well-known surfaces against the contract. The
canonical example module does exactly this:

```cue title="api.cue"
import "github.com/meigma/crossplane-cuefn/contract@v0"

// #API gains scope's default and key validation from the contract.
#API: contract.#API & {
	group:   "platform.meigma.io"
	version: "v1alpha1"
	kind:    "XApp"
	plural:  "xapps"
}

// #Spec and #Status are YOUR schemas — do not wrap them (see the guardrail below).
#Spec: {image: string | *"ghcr.io/stefanprodan/podinfo:6.7.0", replicas: *1 | int & >=1 & <=10}
#Status: {ready: bool, url: string}
```

```cue title="transform.cue"
import "github.com/meigma/crossplane-cuefn/contract@v0"

// out is unified against the closed #Transform: {input, resources, status?}.
out: contract.#Transform & {
	input: {spec: #Spec, metadata: {name: string | *"app", namespace?: string}, environment: {tier: string | *"unset", ...}}
	resources: {
		deployment: {ready: "Ready", object: {/* a Kubernetes object */}}
		// ...
	}
	status: #Status & {ready: true, url: "..."}
}
```

Then resolve the dependency:

```sh
cue mod tidy
```

The contract resolves from the **default central registry** with no `CUE_REGISTRY`
configuration (it lives on `registry.cue.works`). `cue mod tidy` records it in
`cue.mod/module.cue`.

## The payoff: cue vet rejects mistakes

Because `#Transform`, `#API`, `#Input`, and `#Resource` are closed definitions,
unknown or misspelled fields are rejected at author time. For example, a typo in
the `out` field set:

```cue
out: contract.#Transform & {
	resorces: {} // misspelled "resources"
}
```

fails immediately:

```
$ cue vet ./...
out.resorces: field not allowed
```

Without the contract, that typo is silently accepted and simply renders no
resources at runtime. The same applies to an invalid `ready` hint (only `"Ready"`
or `"NotReady"`) or an unknown `#API` key.

## Guardrail: do not wrap `#Spec` / `#Status`

Only the **wrapper** is contract-validated. Your `#Spec` and `#Status` are your
own schemas — leave them as plain top-level definitions. They feed the XRD codegen
(which reduces the module to its definitions), and wrapping them against an
imported definition can break generation. The contract deliberately constrains
only `#API` and the `out` transform.

## Versioning: pin `@v0`

The contract's **major version is welded to the function's major** — both are
`v0`. Pin `@v0` (as `cue mod tidy` does) and it stays the stable compatibility
anchor: within `v0`, a rule fix is a patch and a new optional field is a minor,
and it never auto-rolls to `v1`. A `v1` contract only ever accompanies a `v1`
function whose module-contract shape changed. So a module pinned to `@v0` keeps
working across every `v0.x` function release.
