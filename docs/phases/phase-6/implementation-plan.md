# Phase 6 — Route View for Consumers: Implementation Plan

**Design:** `docs/phases/phase-6/design.md`
**Base:** `40a5863`

There are three planned sub-phases. Each one leaves `go test ./...` and `go vet ./...` passing.

| Sub-phase | Scope | Depends on |
|---|---|---|
| 6.1 | Model and parser: listener state, vhost and route-config per-filter config, weighted-cluster flag | — |
| 6.2 | `pkg/envoy/chain` (`Effective`, `Annotate`), `origin.PolicyFilters` | 6.1 |
| 6.3 | `pkg/envoy/render`; `krp` and the TUI render through it; golden `.txt` files; boundary test; README | 6.2 |

When the phase closes: the cross-model phase review, then tag `v0.4.0`. After that, update the program file (E11 available; the E4 note for Snellius Phase 12), close #27, and post `cross-repo` notices to kgwctl and Snellius.

---

## 6.1 — Model and parser

1. Add the `model` types from the design: `ListenerState`, `ListenerError`, `ListenerStatus`, and `TypedPerFilterConfig` on `RouteConfig` and `VirtualHost`. Also `Route.PerBackendFilterConfig`, `EffectiveChain`, `ChainEntry`, `FilterState` and `ConfigLevel`, plus `Request` and `Response`.
2. Tests first, on synthetic `dynamic_listeners` covering five cases: warming only, active plus warming, active plus draining, error only, and active plus error. Each has the expected `State` and filter chains. Then the parser change: pick the state, parse `error_state`, and keep the existing listener fields.
3. Tests first for per-filter config on a vhost and on a route config, and for the weighted-cluster flag. Then the parser change.
4. Regenerate the JSON goldens. Check with jq that the diff only adds fields (`state` on every listener).

## 6.2 — `pkg/envoy/chain`

1. Write a table test with one case per row of the semantics table. Add the three levels with route over vhost over route config, and the case of no route config. Assert literal `Runs`, `State`, `Level` and `PerFilterType` values, and `Request` and `Response` order.
2. Implement `Effective`.
3. Add corpus checks for `03-multi-policy` (v2.4.7 and v2.5.0) and `00-simple`, with literal expected entries.
4. Add `origin.PolicyFilters`, the `Kgateway` field-to-kind table and the `chain` pass-through. Tests:
   - `03` attributes `extAuth`, `rateLimit.global` and `cors`;
   - an unknown field attributes nothing;
   - a mapper without the interface attributes nothing;
   - a plugin in the chain adds its own field.
5. Implement `Annotate`, which copies the snapshot like `origin.Annotate` and leaves the input unchanged. Test that the input is unchanged and that every route gets a chain.
6. Add `chain` to the boundary test's `core` list.

## 6.3 — `pkg/envoy/render` and krp

1. Add `render.Text` with `Options`, `Class` and `FilterRef`, following the design's text shape. Add golden `.txt` files per corpus dump, with an `-update` flag. Add cases for `Header`, `ResponseOrder`, `AllFilters` and a non-active listener. Add a determinism test (render twice, compare bytes). Add a test that the output has no `\x1b` and no trailing whitespace.
2. Rebuild `internal/renderer` as a `Style` mapping to lipgloss over `render.Text`. The TUI passes `Cursor` and `Expand`. Keep the cursor and expand tests. List each renderer test changed for the layout in the sub-phase record.
3. `krp dump` text annotates chains through `chain.Annotate` with the same mapper as the origins.
4. Boundary test:
   - add `render` to `core`;
   - assert that `render` and `chain` have no `github.com/charmbracelet/` dependency;
   - mutation check: a lipgloss import in `render` fails the test.
5. Do one manual TUI run, and one live kind check on `03`. Show that ext_authz is enabled on the route and denies a request without credentials. Record both in the sub-phase record.
6. In the README: document the new packages and the semantics table, then the text output example.

## Phase close

These are the exit gates from the SDLC skill:
1. The cross-model phase review, `reviews/phase-codex.md`, is triaged.
2. `phase-6-complete.md` is written.
3. `v0.4.0` is tagged, and the program file is updated.
4. #27 is closed with a comment naming the release.
5. The notices are posted: kgwctl and Snellius `cross-repo` issues for E11 and E4.
