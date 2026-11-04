# Sub-phase 4.1 — Library move and API

Kind: planned
Base: 012c61e16ca3a136c1a5532b53274f13d504cbd5
Issues: n/a | Plan steps: implementation-plan.md § 4.1 steps 1–12

## Problem / goal
krp's model, parser, filter and Envoy transport live under `internal/`, so kgwctl and kgateway-ui can't import them. Moving them to `pkg/envoy/` with the API in design § API makes them importable, while krp's CLI behaves exactly as before.

## Expected behaviour

| Input / action | Expected |
|---|---|
| `krp dump --file <dump>` for every scenario dump | stdout byte-identical to the base binary |
| `krp dump --file <dump> --route api-example-com --route-ns default [--rule 0]` | stdout byte-identical to the base binary |
| Invalid flag combinations (no source, `--route` without `--route-ns`, …) | same error text and non-zero exit as the base |
| `parser.Parse` on a reader that errors mid-stream | error, no snapshot |
| `parser.Parse` on valid JSON followed by trailing data | error (same as base `json.Unmarshal`) |
| `source.Admin(url).ConfigDump(ctx)` with a cancelled ctx | error |
| `source.Admin` against a server returning non-200 | error naming the status |
| `kube` source: reader closed | tunnel stopped |
| `kube` source: request error, non-200, or cancelled ctx after the tunnel opens | error returned and tunnel stopped |
| `go list -deps` of `pkg/envoy/{model,parser,filter,source}` | only stdlib and `github.com/DuncanDoyle/krp/pkg/envoy/...` |
| A separate module importing `pkg/envoy/{parser,filter,source}` via `replace` | builds and parses a dump |

## Root cause (if known)
n/a (planned work)

## Acceptance tests
- CLI byte comparison against the base binary for every scenario dump, plus the filtered cases. The script lives in scratch and isn't committed; its output goes in the complete record.
- Unit tests in `pkg/envoy/parser`, `pkg/envoy/source` and `pkg/envoy/source/kube` for the table rows above.
- `pkg/envoy/boundary_test.go` for the import boundary.
- External module build, with output in the complete record.

## Scope
In: `internal/{model,parser,filter,envoy}` → `pkg/envoy/...`, `cmd/krp`, `go.mod`/`go.sum`, `LICENSE`, README architecture/usage.
Out: `-o json` (4.2), test corpus and scripts (4.3), parser accuracy issues #11/#13/#15 (Phase 5), renderer/TUI changes beyond import paths.

## Plan / design impact
none

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] full test suite passes
- [ ] cross-model review of `git diff <base>..HEAD` + triage (planned: at the phase review)
- [ ] phase-4.1-complete.md written; phase-status.md updated
