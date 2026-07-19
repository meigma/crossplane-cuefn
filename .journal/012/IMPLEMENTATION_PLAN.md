# First-class publication metadata and module publishing — implementation plan

Status: complete; PR open for review

Session: 012

Baseline: `master` at `4577625107450c6cbdc9fb350698fe0b95bea1bb` (`v0.1.7`)

This is an agile delivery plan, not a fixed design. The prototype and CLI review
gates are intentionally early; findings at either gate should revise the later
slices instead of forcing an unsuitable design through implementation.

## Outcome

Give module authors an opt-in cuefn publishing path that can:

1. prepare and publish the local CUE module with explicit repeatable OCI
   metadata on its manifest;
2. build the Crossplane Configuration from those same local module bytes;
3. record the exact annotated module manifest digest in the Composition;
4. put the same metadata on the Configuration image as config labels; and
5. push both artifacts with retry-safe, clearly reported partial-failure
   semantics.

The primary documented metadata pair is
`org.opencontainers.image.source=https://github.com/<owner>/<repo>`, but the CLI
contract is deliberately not limited to that one standard key.

The existing v0.1.7 `cuefn publish` behavior must remain unchanged unless the
author explicitly opts into new behavior.

## Baseline and constraints

- `cuefn publish <module-ref> --package <oci-ref>` currently loads a local or
  remote module, generates the XRD, resolves an already-published module digest,
  assembles the Configuration, and pushes only the Configuration.
- `--dir` currently has a known split-source hazard: schema/Composition bytes
  come from disk while `ExpectedDigest` comes from the registry.
- The Configuration builder already reads and rewrites the OCI image config,
  preserves existing labels, and appends the Crossplane package layer.
- Module publication test infrastructure already uses a disposable
  `registry:2`, public `modzip`/`modregistry` APIs, and `render.OCILoader`.
- The full packaging command tree is already behind `//go:build !noxpkg`; the
  runtime image's lean `noxpkg` build must not gain the publishing dependencies.
- Registry routing and credentials remain CUE/Docker-native (`CUE_REGISTRY`,
  the existing resolver, and standard keychains). No credential flags.
- Publication metadata is explicit. Never infer source metadata from Git
  remotes.
- Metadata values are public OCI artifact metadata and command-line visible;
  they are not a credential or secret-input channel.
- Do not mutate `.claude/` or `xr.yaml` in the main checkout.
- Delivery ends with an open reviewable PR. Do not merge it and do not cut a
  release.

## Feasibility findings already established

The pinned dependency sources provide a viable public-only seam:

- `modzip.CreateFromDir` / `modzip.Create` create and validate the module zip.
- `modregistry.Client.PutModule` validates the zip/module file and builds the
  standard CUE artifact: config media type
  `application/vnd.cue.module.v1+json`, then the `application/zip` and
  `application/vnd.cue.modulefile.v1` layers.
- The final manifest is written through `ociregistry.PushManifest`.
  `ociregistry.Funcs` is the supported external wrapper mechanism, so a small
  adapter can decode the OCI manifest, merge user metadata, marshal it,
  and delegate the first real manifest push.
- The Catalyst spike proved GHCR repository linkage and clean CUE v0.16.1
  consumption for exactly this annotated manifest shape.
- `modregistry.Metadata` cannot carry arbitrary annotations; it only supports
  CUE's VCS type, revision, and commit-time keys. User metadata therefore belongs
  in the manifest adapter, merged with—not replacing—CUE metadata.
- CUE marks `modregistry` experimental and `modzip.Create` deprecated. Keep this
  dependency surface behind one small adapter with contract tests so a future CUE
  upgrade changes one boundary rather than the CLI transaction.

Two constraints must be proved rather than assumed:

1. **Canonical preparation before mutation.** Prepare the annotated artifact in
   memory (or another local public-API registry) and know its digest before any
   remote write. Promote those exact bytes; do not rebuild after the digest has
   been put in the Composition.
2. **`source.kind: "git"` fidelity.** Catalyst's Phoenix, PostgreSQL, and
   Typesense modules use Git source mode. CUE's CLI implements tracked-file
   selection and clean-tree checks in an internal package that cuefn cannot
   import. The product must reproduce the relevant behavior with a pure-Go Git
   seam: linked-worktree support, tracked files only, dirty-state refusal, HEAD
   revision/time metadata, and the repository-root `LICENSE` fallback. A
   `CreateFromDir`-only implementation is not acceptable for Git-source modules.

CUE's tidy checker is also internal. cuefn can and should retain its existing
load/codegen validation plus public `modfile`, `modzip`, and `modregistry`
validation, but it must not claim to replace `cue mod tidy --check`. Keep that
author preflight documented.

## Agreed metadata interface and remaining CLI gate

The agreed public metadata interface is one repeatable common flag. The leading
command shape remains an additive extension of the existing `publish` command:

```text
cuefn publish <module-ref> \
  --dir <module-dir> \
  --publish-module \
  --metadata org.opencontainers.image.source=https://github.com/example/repo \
  --metadata org.opencontainers.image.licenses=Apache-2.0 \
  --package <configuration-ref>
```

Metadata rules:

- No new flags: byte-for-byte behavioral and output compatibility with v0.1.7.
- Implement `--metadata <key=value>` as a repeatable Cobra `StringArray`, not a
  comma-splitting slice.
- Split each pair on the first `=` only so values may contain additional `=`
  characters; preserve the validated value exactly.
- Reject empty keys, empty values, duplicate input keys, and collisions with
  metadata owned or preserved by CUE or the xpkg builder. Do not introduce
  last-wins precedence.
- Normalize the parsed pairs to a deterministic map so flag order does not
  change either artifact digest.
- In combined mode, apply the same map as CUE module manifest annotations and
  Configuration image-config labels.
- `--metadata` without `--publish-module` labels only the Configuration while
  retaining the existing already-published-module flow; help and docs must state
  that cuefn cannot change metadata on an existing module it does not publish.
- `--publish-module` requires `--dir`, then owns both artifact publications and
  removes the `--dir` digest mismatch for this explicit mode. Metadata itself is
  optional.
- `--dir` alone: retain the existing two-step behavior and warning for backward
  compatibility.
- Apply semantic validation to known standard keys where it prevents a clear
  mistake: `org.opencontainers.image.source`, when supplied, must be an absolute
  HTTP(S) URL. Other keys receive structural key/value validation without cuefn
  inventing meaning for them.
- Do not add a `--source` alias. One spelling avoids precedence and collision
  rules for two paths to the same metadata.

The generic metadata decision is settled. The early prototype still validates
whether the existing `publish` command remains the smallest command placement:

| Candidate | Advantage | Main concern |
|---|---|---|
| Add `--publish-module` and `--metadata` to `publish` (leading) | One transaction owns the exact digest and both outputs; existing scripts remain valid | The command gains another mode, so flag validation and help must be crisp |
| Add `publish-module` | Focused verb and reusable module publisher | A second user-visible command does not by itself guarantee the Configuration uses the digest from that same publication |
| Make `publish --dir` publish automatically | Fewest flags | Adds a hidden network mutation to an existing flag and breaks compatibility |

After Slice 1, record the prototype result and final CLI choice in session
`012/NOTES.md`. Preserve the agreed repeatable metadata contract even if evidence
changes the command placement; revise this document before implementation if the
first candidate is no longer the least surprising.

## Delivery slices

### Slice 0 — isolate the work and re-confirm the base

- Fetch `origin/master` and verify it still represents the intended release
  baseline.
- Create a Worktrunk branch/worktree from fetched master, provisionally
  `feat/publish-metadata`.
- Confirm the implementation worktree has no tracked `.journal/` files and that
  the main checkout's `.claude/` and `xr.yaml` remain untouched.
- Record the chosen branch and exact base SHA in the session notes.

Gate: stop if master or the release state materially changed since this plan.

### Slice 1 — smallest annotated-module prototype

Build a deliberately small prototype around the pinned public APIs:

- Parse a full `path@vX.Y.Z` module version.
- Package a `source.kind: "self"` fixture through `modzip`.
- Run `modregistry` against a local recording/in-memory OCI registry.
- Wrap only the final manifest write, verify it is the expected CUE manifest,
  merge a small metadata map including `org.opencontainers.image.source`, and
  capture the canonical descriptor.
- Promote the exact prepared blobs and manifest to the disposable registry.
- Pull the remote manifest back and prove:
  - exact source and second generic metadata annotations;
  - expected config media type;
  - exactly two layers with unchanged media types;
  - prepared digest equals pushed and resolved digest; and
  - `render.OCILoader` fetches and evaluates it normally.

Keep useful code/tests; delete throwaway scaffolding. Do not design the full CLI
until this proof is green.

Gate: if public APIs cannot produce and promote one canonical annotated artifact
without a post-publication rewrite, stop and revisit the approach.

### Slice 2 — faithful local module preparation and immutable publication

Turn the prototype into a narrow publishing component behind `!noxpkg`.

Preparation is pure/local and returns a value containing at least the canonical
module version, manifest descriptor/digest, manifest bytes, and referenced blobs.
It performs all available checks before a registry mutation:

- parse `cue.mod/module.cue` with public `modfile` APIs;
- require the operand's module path/major to match the module file;
- package `source.kind: "self"` with `modzip.CreateFromDir`;
- package `source.kind: "git"` from the tracked file set with `modzip.Create`,
  rejecting a dirty repository and preserving CUE's root-LICENSE and VCS metadata
  behavior;
- preserve existing CUE annotations while adding the explicit metadata map;
- use a pure-Go Git implementation that works in ordinary and linked Worktrunk
  worktrees. Treat a suitable Go Git library as the leading experiment, not a
  committed dependency until parity is proven.

Before promoting:

1. resolve the destination tag through the standard CUE resolver;
2. if absent, push the exact prepared blobs and manifest;
3. if the existing digest equals the prepared digest, return a `reused` result
   without rewriting the tag;
4. if it differs, fail before any tag mutation and report both digests; and
5. after a push, re-resolve and require the registry digest to equal the prepared
   digest.

Sequential same-content retries must be safe. Different metadata is part of
artifact identity and therefore counts as different content. Generic OCI
distribution has no portable compare-and-swap tag operation; document that the
preflight/postflight guard protects normal retries but cannot make two racing
publishers atomic across all registries.

Focused tests:

- self-source package;
- Git-source tracked/untracked/dirty/linked-worktree behavior;
- multiple metadata pairs merged with CUE VCS metadata;
- pair parsing with additional `=`, order independence, duplicate rejection,
  known-source URL validation, and generated-metadata collision rejection;
- same-content reuse;
- different-content and different-metadata rejection;
- malformed module ref, module-path mismatch, invalid metadata, and registry
  errors.

Gate: compare the Git-source fixture's file set and metadata with pinned CUE
behavior. If parity cannot be achieved without command internals or subprocesses,
pause rather than silently changing published module contents.

### Slice 3 — Configuration metadata and end-to-end transaction

Add the smallest image-builder option needed to merge a metadata map into config
labels while preserving existing labels and source-image immutability. Reject
collisions instead of overwriting preserved/generated labels. The no-option path
must produce the same Configuration and Function xpkg behavior as today.

Wire the chosen CLI mode as a transaction:

1. validate all flags and both destination references;
2. load the local module and generate the XRD;
3. prepare the annotated module locally and obtain its canonical digest;
4. derive function names and package metadata, generate the Composition with
   that prepared digest, and assemble the metadata-labelled Configuration image;
5. only after both artifacts are locally valid, publish/reuse the module;
6. require the returned module digest to equal the prepared digest; and
7. push the Configuration.

Existing non-module-publishing mode continues to resolve the live module digest
as it does in v0.1.7.

Failure/output contract:

- Existing mode retains its current `pushed <configuration@digest>` success
  output.
- Combined mode reports whether the module was `published` or `reused`, always
  including its exact digest, followed by the Configuration digest on success.
- If validation or Configuration assembly fails, neither artifact is pushed.
- If module publication succeeds but Configuration push fails, return a non-zero
  error that names the module ref, exact digest, failed Configuration ref, and
  says that rerunning the same command is safe.
- Never print credentials or infer repository state.

Keep the orchestration thin: module preparation/publication and xpkg assembly
remain independently testable; CLI code owns flag validation, sequencing, and
human-facing diagnostics.

### Slice 4 — acceptance integration and regression coverage

Extend the disposable-registry harness and the existing publish integration test
instead of creating a parallel test stack. Prove in one combined-path test:

- exact module manifest source annotation and a second generic metadata pair;
- unchanged CUE config and layer media types/shape;
- cuefn-reported digest equals the registry descriptor;
- `render.OCILoader` loads/evaluates the annotated module;
- the exact same Configuration config labels;
- `xpkg.ExtractPackageYAML` (and the existing Crossplane extraction acceptance)
  still reads the package;
- the packaged Composition contains the digest produced by that same
  transaction;
- same-content retry reuses the module;
- metadata flag ordering yields the same digest;
- duplicate/colliding metadata fails before publication;
- different-content version reuse is rejected and leaves the old tag intact;
- an injected/refusing Configuration destination produces the required partial
  result with the module digest; and
- the old publish path stays green without new flags.

Keep Docker-backed cases under the existing `CUEFN_INTEGRATION` gate and update
the `publish-test` task selector only if new test names require it.

### Slice 5 — user-facing contract and documentation

Update only documentation made inaccurate by the new opt-in flow:

- `docs/docs/reference/cli.md` — flags, validation matrix, output, retry and
  partial-failure semantics, and the manifest-annotation/config-label targeting
  contract;
- `docs/docs/how-to/publish-configuration.md` — native combined flow first,
  legacy two-step flow retained where useful, plus the tidy preflight;
- `docs/docs/explanation/digest-lockstep.md` — the prepared metadata-bearing
  digest is known before either remote mutation and carried into the
  Composition;
- `docs/docs/explanation/one-module-two-outputs.md` — cuefn can publish both
  artifacts and place common metadata on each using their correct OCI location;
- `README.md` and quickstart examples where the old prerequisite/order warning
  would otherwise remain the primary story.

Do not add separate low-level manifest-annotation/config-label flags,
artifact-specific overrides, GHCR-specific behavior, release changes, or
unrelated publishing features. `--metadata` is common publication metadata, not
an unrestricted OCI-layout mutation interface.

### Slice 6 — full verification and review handoff

Run focused checks while iterating, then the required final sequence:

```text
mise exec -- go test ./internal/pkg ./internal/cli
moon run root:oci-test
moon run root:publish-test
golangci-lint cache clean
moon run root:check
moon ci
git diff --check
```

Also verify:

- `go build -tags noxpkg -o bin/cuefn-noxpkg ./cmd/cuefn` remains green;
- `git ls-files .journal` prints nothing in the implementation worktree;
- the main checkout's developer-owned untracked paths are unchanged;
- no scratch artifact or registry container remains; and
- the final diff contains no release/version changes.

Use conventional commits at the proof and vertical-slice boundaries. Push the
branch, open a PR with a conventional title and concise testing evidence, then
confirm hosted CI/integration on the exact PR head. Stop with the PR open for
human review—no merge and no release.

## Likely code surface (not a commitment)

- `internal/cli/publish.go` and tests — opt-in flags, validation, transaction,
  output/error contract.
- `internal/pkg/` or a small sibling publishing package — prepared CUE artifact,
  OCI manifest adapter, promotion, immutable retry logic.
- `internal/pkg/image.go` / `configuration.go` and tests — additive image-config
  label option.
- `internal/test/common/registry.go` — inspection and failure-test helpers.
- `internal/test/integration/publish_test.go` — combined acceptance proof.
- `go.mod` / `go.sum` — only if the Git-source prototype justifies a pure-Go Git
  dependency.
- The documentation files listed in Slice 5.

Favor the smallest arrangement that preserves the existing hexagonal seams.
File/package names may change after the prototype; the behavioral boundaries and
acceptance evidence are the durable part of this plan.

## Done

The work is complete when every acceptance item from the prompt is proven on the
review branch, the public CLI/docs make the retry and partial-failure model clear,
both local final gates and hosted checks pass on the exact PR head, and the PR is
open but unmerged with no release created.
