---
id: 009
title: Resolve Function xpkg render compatibility
started: 2026-07-16
---

## 2026-07-16 19:01 — Kickoff
Goal for the session: Resolve the Function xpkg image-config compatibility defect blocking Catalyst Infra X0, land a narrowly scoped review-ready implementation PR, and pause before any merge or publication without explicit human approval.
Current state of the world: `master` is at `8eebd929b75346d5a5672a5c858546c562b5b560`; published product v0.1.4 works in-cluster but its Function xpkg keeps `function` in `Cmd`, so Crossplane CLI 2.3.3 replaces it with `--insecure` and the container exits; developer-owned untracked `.claude/` and `xr.yaml` are present and must remain untouched.
Plan: Read the Catalyst implementation proposal and current GitHub state, create an isolated Worktrunk branch, first reproduce the defect with a focused failing test, implement the smallest package-only normalization, expand package and standard Docker render coverage, run the requested gates, publish a review-ready PR, and stop for approval.
