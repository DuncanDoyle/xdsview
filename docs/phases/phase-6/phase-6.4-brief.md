# Sub-phase 6.4 — Phase review fixes

Kind: stabilization
Base: 02e635f
Issues: reviews/phase-codex.md #1–#12, #14 | queue Q8–Q20

## Problem / goal
The Phase 6 review found seven correctness bugs, three design mismatches, two test gaps and one fragile copy. This sub-phase fixes them, each with a test at the boundary where the bug shows.

## Expected behaviour

| # | Input | Expected |
|---|---|---|
| 1 | HCM filter `ExtensionWithMatcher` → `Composite`, delegating to RBAC (decoder only) | Entry `Type` is the RBAC type, and `Request` and `Response` are both `yes` (the composite's sides) |
| 2 | Route with policy `extAuth`, two ext_authz entries that both have a per-filter level | Neither entry gets the `extAuth` policy contribution. With one such entry, it does. |
| 3 | `Conditional` chain where no filter runs on the baseline | The output still has `filters (request order, baseline before backend selection):` and the `backend overrides:` line. With `ResponseOrder`, the response heading also says `baseline before backend selection`. |
| 4 | Two HCM filters with the same name, `AllFilters` | `Style` receives `FilterIdx` 0 and 1, and `Extra` shows each one's own config |
| 5 | Listener error details with ESC; a request header value ending in a tab | No `\x1b` byte and no trailing whitespace in the plain output; the ESC prints as `\x1b` |
| 6 | `krp dump -i` on a snapshot with only a rejected listener | Prints the tree, including the error, and exits 0 |
| 7 | Per-filter entry `@type: …/acme.FilterConfig` with `disabled: true`; a `FilterConfig` with `"config": null` | The first is a filter-specific config (`enabled`/`on`). The second is `unknown`. |
| 8 | Route whose origin has `NarrowedByKnown` | Text ends `(chosen from known HTTPRoutes)` |
| 9 | Listener `:::80`; a chain with ten rows; a `?` row | Text shows `[::]:80`. The state column aligns across rows 1–10 and `?` rows. JSON `address` is unchanged. |
| 10 | — | D-009 in `decisions.md` |
| 11 | TUI navigation | Asserted on `View()`: the cursor row is visible, and the expanded config is directly under it |
| 12 | A `core` package importing a `pkg/envoy` package outside `core` | Boundary test fails |
| 14 | Edit a nested per-filter config value on one HCM's copy | The other HCM's copy is unchanged |

## Root cause (if known)
See `reviews/phase-codex.md`. Item 1: the parser replaces a composite's `Type` with its delegate, and `chain` reads sides from `Type`. Item 4: `render.rows` looks the index up by name.

## Acceptance tests
One test per row, at the boundary named: `chain` (1, 2, 7), `render` (3, 4, 5, 8, 9), `cmd/krp` or `tui` (6), `tui` `View()` (11), the boundary test with a mutation run by hand (12), `parser` (14). Goldens regenerated where the wording or alignment changes, with the diff reviewed.

## Scope
In: `pkg/envoy/{chain,render,parser,model}`, `internal/{renderer,tui}`, `cmd/krp`, the boundary test, `decisions.md`, README wording.
Out: #13 (Phase 7). No new features.

## Plan / design impact
The design text `(inferred from cluster routes)` becomes `(chosen from known HTTPRoutes)` (#8). D-009 records #10.

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] `go test ./...` and `go vet ./...` pass
- [ ] cross-model review of `02e635f..HEAD` (stabilization), triaged
- [ ] phase-6.4-complete.md written; phase-status.md updated; kgwctl told about #8
