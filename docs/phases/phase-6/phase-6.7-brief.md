# Sub-phase 6.7 — Silent HCM skips, listeners mid-update (`v0.4.3`)

Kind: stabilization
Base: ab593d2
Issues: #31 | Source: kgwctl Phase 3 Codex review

## Problem / goal
A view can look complete while it isn't. An HCM whose config fails to decode disappears without a warning. Narrowing drops an active listener that is mid-update, so a route still waiting on that update leaves an empty tree.

## Expected behaviour

| # | Input | Expected |
|---|---|---|
| 1 | Dump with an HCM whose `typed_config` doesn't decode (e.g. `http_filters` is a string) | `Parse` succeeds; one warning names the listener and filter chain; the filter chain is kept, with no HCM |
| 1 | Dump whose HCMs all decode | No warnings (unchanged) |
| 2 | Active listener with `Updating` set, no matching route | `Filter` keeps it, state unchanged, no routes |
| 2 | Active listener with `Draining` set, no matching route | Same |
| 2 | Active listener, neither set, no error, no matching route | Dropped (unchanged) |

## Root cause (if known)
1. `parseHCM` returns nil on a decode error, and the caller skips it without a warning.
2. `filter.explains` checks `Status` and `Error`, not `Updating` or `Draining`.

## Acceptance tests
1. `parser.Parse` on a JSON dump.
2. `filter.Filter` on a snapshot.

## Scope
In: `pkg/envoy/parser`, `pkg/envoy/filter`. Out: #32 (E6), #30.

## Plan / design impact
None. The filter package doc already says listeners whose state may explain a missing route are kept.

## Definition of done
- [ ] acceptance tests fail before, pass after
- [ ] `go test ./...`, `go vet ./...` pass
- [ ] cross-model review of `ab593d2..HEAD`, triaged
- [ ] phase-6.7-complete.md; `v0.4.3` (owner OK given); #31 closed; kgwctl told
