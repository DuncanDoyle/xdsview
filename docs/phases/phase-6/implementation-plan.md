# Phase 6 — Route View for Consumers: Implementation Plan

**Design:** `docs/phases/phase-6/design.md`
**Base:** `40a5863`

There are three planned sub-phases. Each one leaves `go test ./...` and `go vet ./...` passing.

| Sub-phase | Scope | Depends on |
|---|---|---|
| 6.1 | Model and parser: listener state, per-filter config on vhost, route config and weighted cluster | — |
| 6.2 | `pkg/envoy/chain`, `origin.PolicyFilters`, narrowing on origin, non-active listeners kept | 6.1 |
| 6.3 | `pkg/envoy/render`; `krp` and the TUI render through it; golden `.txt` files; boundary test; README | 6.2 |

When the phase closes, run the cross-model phase review and tag `v0.4.0`. Then:
1. Update the program file: E11 is available, and add the E4 note for Snellius Phase 12.
2. Close #27.
3. Post `cross-repo` notices to kgwctl and Snellius.

---

## 6.1 — Model and parser

1. Add the `model` types from the design:
   - `ListenerState`, `ListenerError`, `ListenerStatus`;
   - `TypedPerFilterConfig` on `RouteConfig`, `VirtualHost` and `WeightedCluster`;
   - `EffectiveChain`, `ChainEntry`, `Contribution`, `FilterState`, `ConfigLevel`, `Side`, `ContributionKind`;
   - `RequestOrder` and `ResponseOrder`.
2. Write the tests first, on synthetic `dynamic_listeners`. Cover each case and assert the expected `State` and filter chains:
   - warming only;
   - active plus warming;
   - active plus draining;
   - error only;
   - active plus error;
   - an unnamed LDS error.

   Then change the parser.
3. Write tests first for per-filter config on a vhost, a route config and a weighted cluster. Assert them on the joined snapshot after the per-HCM RDS clone. Then change the parser, including `cloneRouteConfig`.
4. Regenerate the JSON goldens. Check with jq that the diff only adds fields.

## 6.2 — `pkg/envoy/chain`, attribution and narrowing

1. Write a table test with one case per row of the semantics table:
   - the invalid-`FilterConfig` case (`unknown`);
   - the three levels, with route taking precedence over vhost, and vhost over route config;
   - no route config;
   - `Conditional`.

   Assert literal values for `Runs`, `State`, `Level`, `PerFilterType`, `Request` and `Response`, and for both orders.
2. Build the side table. List each filter type's decoder and encoder interfaces, read from Envoy source at the version kgateway v2.5 ships (`/server_info` on the kind cluster), and cite file and line in the code. Unknown types get `unknown`.
3. Implement `Effective`. Add corpus checks for `03-multi-policy` (v2.4.7 and v2.5.0) and `00-simple`, with literal expected entries.
4. Add `origin.PolicyFilters`, the `Kgateway` field-to-kind table, extension contributions, and the pass-through in `Chain`. Tests:
   - `03` attributes `extAuth`, `rateLimit.global` and `cors`, plus the GatewayExtension;
   - an unknown field attributes nothing;
   - a mapper without the interface attributes nothing;
   - a plugin in the chain adds its own field.
5. Implement `Annotate`, which copies like `origin.Annotate`. Test that the input is unchanged and that every route, in every HCM, gets a chain.
6. Narrowing. Write tests first:
   - the colliding pair, with and without `Known`;
   - `filter.Ambiguous`;
   - warming and rejected listeners survive.

   Then change `filter.Filter`. Existing tests on un-annotated snapshots stay unchanged.
7. Add `chain` to the boundary test's `core`.

## 6.3 — `pkg/envoy/render` and krp

1. Add `render.Text` with `Options`, `Class` and `FilterRef`, in the design's text shape. Add golden `.txt` files per corpus dump, with an `-update` flag. Add cases:
   - `Header`, `ResponseOrder`, `AllFilters`;
   - a non-active listener and an LDS error;
   - a `Conditional` chain.

   Also add a determinism test (render twice and compare bytes), and a test that the output has no `\x1b` and no trailing whitespace.
2. Rebuild `internal/renderer` as a `Style` mapping to lipgloss over `render.Text`. The TUI uses `Style` for its cursor and `Extra` for expanded config, with `AllFilters: true`. Add the navigation test from the design. List each renderer test that changes because of the layout in the sub-phase record.
3. `krp dump` annotates chains with `chain.Annotate`, using the same mapper as for origins, for both text and `-o json`. Regenerate the JSON goldens and check that they contain only additions.
4. Boundary test:
   - add `render` to `core`;
   - assert no `github.com/charmbracelet/` dependency;
   - mutation check: a lipgloss import in `render` fails the test.
5. Do one manual TUI run. Do one live kind check on `03`, using the ext_authz stats oracle from the design (enabled route against a control route). Record both in the sub-phase record.
6. README: the new packages, the semantics table, an example of the text output, and the narrowing change.

## Phase close

These are the exit gates from the SDLC skill:
1. The cross-model phase review, `reviews/phase-codex.md`, is triaged.
2. `phase-6-complete.md` is written.
3. `v0.4.0` is tagged, and the program file is updated.
4. #27 is closed with a comment naming the release.
5. The notices are posted: `cross-repo` issues to kgwctl and Snellius for E11 and E4.
