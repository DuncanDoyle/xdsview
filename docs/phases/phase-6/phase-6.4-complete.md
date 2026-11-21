# Phase 6.4 — Phase review fixes (Complete)

**Completed:** 2026-11-21
**Range:** 02e635f..7d527cf (`main`)

## Summary
Fixes for the 13 accepted findings of the Phase 6 review (`reviews/phase-codex.md` #1–#12, #14). The sub-phase's own cross-model review (`reviews/6.4-codex.md`) found one P1, two P2s and five P3s in those fixes. All of them are fixed.

## Sub-phases (phase record only)
n/a

## Reviews
- `reviews/6.4-codex.md`: Codex, cross-model. 8 findings, all accepted and fixed in `7d527cf`.

## Waivers
none

## Deliverables

| # | Fix | Test (failed before, passes after) |
|---|---|---|
| 1 | Sides come from the installed filter; a composite is yes/yes | `TestEffective_CompositeSides` (was `Response: no`) |
| 2 | A policy field is attributed only when one entry with per-filter config matches | `TestAnnotate_PolicyNeedsOneMatch` (was credited to both) |
| 3 | A conditional chain labels both orders, even when no filter runs | `TestText_ConditionalAlwaysLabelled` |
| 4 | Rows carry their HCM index | `TestText_DuplicateNames` (was `[0 0]`); `TestRenderInteractive_DuplicateNamesOwnConfig` (each config under its own row) |
| 5 | Control characters escaped; spaces inside a line kept; line ends trimmed | `TestText_UntrustedStrings`, `TestText_InnerSpacesKept` |
| 6 | `krp -i` prints the tree when nothing is selectable | `TestDumpInteractive_NothingSelectable` (stdout was empty) |
| 7 | Exact `FilterConfig` type; null `config` is unset | `TestEffective_FilterConfigExactType` |
| 8 | `(chosen from known HTTPRoutes)` | `TestText_KnownWording`; README and design updated; kgwctl told |
| 9 | IPv6 bracketed in text; number column padded; padding counts runes | `TestText_AddressesAndAlignment` (requires 10 rows, incl. `10` and `4?`), `TestText_UTF8Alignment` |
| 10 | D-009 (`WeightedCluster` not comparable); design's compatibility lines name it | docs |
| 11 | Navigation asserted on `View()`: `{` directly under the cursor row; cursor follows in a 3-line viewport | `TestNavigation_EnabledDisabledEnabled`, `TestNavigation_CursorStaysOnScreen` (mutation: making `scrollToCursor` a no-op fails all 5 steps) |
| 12 | Boundary test allows only `core` | Mutation: a stdlib-only `pkg/envoy/notcore` imported from `render` failed `TestCoreImportsOnlyStdlib` ("render depends on …/notcore"). With the old rule it would have passed. Removed afterwards. |
| 14 | Per-filter config and metadata deep-copied per HCM | `TestParse_PerFilterConfigIsolated` covers nested maps at vhost and backend, an array at route, and route config. Mutation: no longer copying arrays fails the array check. |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |  |

The text goldens changed only by the wording (#8) and bracketed IPv6 addresses (#9). That was checked by normalizing both and diffing.

## Notes for later phases
- Padding counts runes. East Asian wide characters still count as one column.
- `tui.Run` takes `out` and `errOut` writers.

## Deferred / open
- #13 (Phase 7 lifecycle) is on the Phase 7 roadmap entry.
