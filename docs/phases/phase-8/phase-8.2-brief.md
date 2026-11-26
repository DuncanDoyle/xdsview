# Sub-phase 8.2 — Downstream corpus out, OSS coverage kept

Kind: planned
Base: da82d0d
Issues: #32 (part) | Plan steps: implementation-plan.md §8.2 | Edge: E6 | Decisions: D-013, D-018

## Problem / goal
The module ships downstream dumps, goldens and vendor names (P-002). They leave the repository (git history stays, D-018), without losing any regression coverage they provide.

## Expected behaviour

| # | Input / action | Expected |
|---|---|---|
| 1 | `testdata/` tree | No downstream corpus directory and no downstream goldens |
| 2 | Neutral hand-built dump `testdata/fixtures/upstream-extproc` | `krp dump -o json` golden holds the router `upstreamFilters` subtree (typed config, origin) and a route's `ExtProcPerRoute` `disabled` per-filter config |
| 3 | Hand-built `accuracy` fixture, text render | Line `router upstream filters (as configured): …` |
| 4 | Parser accuracy tests (simple HTTP, HTTPS/SNI, ext auth, header modifiers, mirror, rewrite, CORS, rate limit) | Same assertions on OSS dumps (00, 02_x); downstream-only values replaced by OSS ones |
| 5 | Narrowing, parse-all, matcher tests | Run on OSS dumps only; no skips for downstream paths |
| 6 | A case-insensitive scan for edition names over Go code in `pkg cmd internal` | Only `origin/kgateway.go`'s transformation type, `chain/sides.go`'s entry for it, and their tests (8.3 removes them) |
| 7 | Same scan over `testdata/` | Only recorded OSS manifests (image `solo-public/docs/hey`) |
| 8 | OSS goldens | Unchanged |

## Root cause (if known)
n/a (planned removal).

## Acceptance tests
- Cases 2 and 3 are replacement tests. They pass **before** the deletion.
- `TestDumpJSON_Golden` must still find every OSS dump.
- Test counts per package are reported before and after, naming each dropped test and why.

## Scope
In: `testdata/`, the parser, filter, render and CLI tests, the hand-built fixtures named in plan step 5, README corpus section, and a pointer from D-003.
Out: `origin/kgateway.go` and `chain/sides.go` logic (8.3).

## Plan / design impact
None.

## Definition of done
- [ ] replacement tests pass before the deletion; full suite passes after
- [ ] `go test ./...`, `go vet ./...` pass; test counts before and after reported
- [ ] greps as in cases 6 and 7
- [ ] phase-8.2-complete.md; phase-status updated
