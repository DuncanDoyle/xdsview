# Phase 6.7 — Silent HCM skips, listeners mid-update (Complete)

**Completed:** 2026-11-21
**Range:** ab593d2..v0.4.3

## Summary
`Parse` now warns when an HCM's config doesn't decode, or is missing or null. The warning names the listener and the filter chain, by index if the chain has no name. The filter chain stays, without an HCM. Narrowing keeps an active listener with a version warming or draining beside it, so a route waiting on that update doesn't leave an empty tree (#31, from kgwctl's Phase 3 review).

## Sub-phases (phase record only)
n/a

## Reviews
- `reviews/6.7-codex.md`: Codex, cross-model. 5 findings (1 P1, 2 P2, 1 P3, 1 P4): 4 accepted and fixed, 1 deferred to Phase 7.

## Waivers
none

## Deliverables

| Item | Test |
|---|---|
| Undecodable HCM in the shown version warns; chain kept, no HCM | `TestParse_UndecodableHCMWarns` (failed before) |
| Null config warns; unnamed chain labelled `#0`; a bad version that isn't shown doesn't warn | same test, review cases (null case failed before the review fix) |
| Active listener with `Updating` or `Draining` survives narrowing, state unchanged | `TestFilter_ListenersThatExplain` (failed before) |
| Clean dumps give no warnings | existing parser tests and e2e corpus |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |

## Notes for later phases
- Phase 7: a skipped HCM looks like no HCM in the snapshot. Compare with the parser's warnings in hand (roadmap Phase 7).

## Deferred / open
- #32 (E6 cleanup) is Phase 8.
- #30 (remaining policy fields) is open, waiting for a dump that uses them.
