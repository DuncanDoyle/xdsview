# Sub-phase 6.3 — `pkg/envoy/render` and krp

Kind: planned
Base: 371297e
Issues: #27 (E11) | Plan steps: implementation-plan.md § 6.3

## Problem / goal
kgwctl and Snellius need xdsview's route view as plain text, not a copy of `krp`'s lipgloss renderer. `render.Text` writes the design's text shape using the standard library only. `krp` and its TUI style that same output through the `Style` and `Extra` hooks, so there is one tree. The owner chose two things on 2026-11-21: the default lists only the filters that run plus one "not run:" line, and `krp` drops its bordered listener panel.

## Expected behaviour

| Input | Expected output |
|---|---|
| Corpus dump, annotated with `origin.Kgateway` and `chain.Annotate` | Matches the golden `.txt` file byte for byte. No `\x1b`, no trailing spaces, ends with LF. Rendering twice gives the same bytes. |
| Default options | Each route lists the running filters in request order, then one `not run:` line naming the rest with their state |
| `AllFilters: true` | Every HCM filter is listed inline at its HCM position, with its state, and there is no `not run:` line |
| `ResponseOrder: true` | Each route also lists `filters (response order):` |
| A filter whose type is not in the side table | Listed in both orders, marked `?` |
| `Conditional` chain | The header says `baseline before backend selection`, and a line names each backend that overrides and the filters it sets |
| Warming, draining or rejected listener, active with an error, LDS error with no name | The listener line shows the state, `updating`/`draining`, and `error: <details> (<time>)`. A listener without a name shows as `LDS error`. |
| Route origin settled by `Known` | The origin is followed by `(inferred from cluster routes)` |
| `Header` set | Printed as the first line |
| Snapshot without chains | `Text` computes them with `chain.Effective` and no mapper |
| `krp dump` (text) | The same tree as `render.Text`, coloured, with no bordered panel |
| `krp dump -o json` | Each route has `effectiveChain`. The goldens change by additions only. |
| `krp -i` | Every HCM filter of every route is a cursor row (`AllFilters`). The cursor highlights the selected row, and Enter shows that filter's config under it. A filter that a vhost override enables is navigable like the others. |

## Root cause (if known)
n/a (new behaviour).

## Acceptance tests
- `pkg/envoy/render`: golden `.txt` per corpus dump (`-update`). Option cases: `AllFilters`, `ResponseOrder`, `Header`, an unknown side, `Conditional`, listener states and an LDS error, `NarrowedByKnown`, and an un-annotated snapshot. Plus the determinism, no-ANSI and no-trailing-space checks.
- `pkg/envoy`: the boundary test adds `render` to `core` and asserts no `github.com/charmbracelet/` dependency. A mutation check (a lipgloss import in `render`) is run once by hand and recorded.
- `internal/renderer` and `internal/tui`: existing tests kept where the information is the same, and updated where the layout changed. Each changed test is listed in the record. One navigation test covers an enabled, disabled, enabled sequence with a vhost override.
- `cmd/krp`: JSON goldens regenerated; the diff checked with jq for additions only.
- One manual TUI run, recorded.
- A live kind check on `03` with the ext_authz stats oracle, recorded. If Docker is not available, it is deferred and noted as an open gate.

## Scope
In: `pkg/envoy/render` (new), `internal/renderer`, `internal/tui`, `cmd/krp`, the boundary test, README.
Out: changes to model, chain or filter semantics.

## Plan / design impact
None.

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] `go test ./...` and `go vet ./...` pass
- [ ] cross-model review at the phase review (planned sub-phase)
- [ ] phase-6.3-complete.md written; phase-status.md updated; kgwctl messaged
