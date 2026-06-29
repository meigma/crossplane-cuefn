# Reserved-key projection

The example `#Spec` is a **closed** CUE struct: it rejects any field the author
did not declare. That strictness is the whole point — it is what makes the schema
authoritative. But a closed schema collides with reality, because the spec the
runtime observes carries fields the author never wrote. This page explains how
cuefn reconciles the two.

## The conflict

In Crossplane v2, an observed XR spec can contain a reserved `crossplane` block
that nests composition machinery (`compositionRef`, `resourceRefs`, and so on),
plus legacy v1 machinery keys that may still appear. None of these are part of
the author's API; they are Crossplane-internal plumbing.

If the engine unified that raw observed spec against a closed `#Spec`, the
machinery keys would be fields the schema never declared, and the unification
would fail — `resources` would never go concrete in-cluster. (The original
reference spike avoided this only because its `input.spec` was an **open** struct,
which provides defaults but enforces no schema.)

## The projection

Rather than weaken the schema, the engine narrows the input. Before unifying, it
**projects** the observed spec — returning a shallow copy with the reserved keys
removed:

```
crossplane
compositionRef
compositionSelector
compositionRevisionRef
compositionRevisionSelector
compositionUpdatePolicy
claimRef
resourceRef
resourceRefs
writeConnectionSecretToRef
publishConnectionDetailsTo
environmentConfigRefs
```

What reaches `#Spec` is only the author-declared fields, so the closed schema
unifies cleanly. The original spec is never mutated, and a `nil` spec projects to
`nil`.

## Why this lives in the engine

The stripping is a property of the **engine**, not the schema. Keeping it out of
`#Spec` means authors write a clean, closed schema that describes only their API —
they never have to declare or anticipate Crossplane's machinery fields. The
engine owns the knowledge of which keys are reserved, in one place
(`ProjectSpec`), and applies it uniformly at render, in `validate`, and in the
served function. The schema stays closed; the engine makes the closed schema
usable against a real observed spec.
