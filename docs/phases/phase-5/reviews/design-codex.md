Local file links and vendor names redacted; reviewer text otherwise unchanged.

1. **P1 — Proposed package dependencies form an import cycle.** `docs/phases/phase-5/design.md:73`, § Model. Origin types live in `origin`, and `Mapper` imports `model`; adding those types to `model.Route.Origin`, etc. requires `model` to import `origin`. The specified API cannot compile. Put annotation value types in `model` or an independent package, or return annotations separately.

2. **P1 — Rule-name matching still admits false positives.** `docs/phases/phase-5/design.md:151`, plan §5.3 step 6. A valid rule named `a-matcher-b` produces `…-0-0-a-matcher-b-matcher-0`. Selecting `a` matches the proposed `-0-0-a-matcher-` segment. Anchor matching to the final `-matcher-<matchIdx>` suffix; testing only `a` versus `a-b` misses this.

3. **P2 — The swappable mapper cannot express the UI’s route-config lookup.** `docs/phases/phase-5/design.md:95`, § UI adapter. `ObjectForName` promises route-config resolution, but `Mapper` has no route-config or bare-name method. Existing HTTPS route-config names such as `https-api` encode neither a hostname nor a Gateway identity. The plan must specify how contextual Gateway information and replacement naming implementations reach this lookup without hardcoding kgateway rules outside the seam.

4. **P2 — Unconditional “Inferred equals Exact” contradicts the ambiguity contract.** `docs/phases/phase-5/design.md:180`, plan §5.3 step 5. The existing `api-example-com-kgateway-system-0-0` suffix admits several valid identities, including `api-example-com`/`kgateway-system` and `api-example`/`com-kgateway-system`. Without `Known`, name parsing must return multiple candidates; metadata returns one. Specify candidate containment without `Known`, and equality after supplying independently declared known refs. Otherwise this acceptance test pressures implementation into guessing.

5. **P2 — Extending OSS expectations also creates nonexistent downstream expectations.** `docs/phases/phase-5/implementation-plan.md:49`. The downstream route-list helper copies every entry from `scenarioRoutes`, excluding only scenario 06. Adding 03/04/05/07/08 as instructed makes downstream tests open recordings that the plan never creates. Separate corpus expectations or explicitly restrict downstream membership.

6. **P2 — New weighted-cluster slices are omitted from the parser’s ownership work.** `docs/phases/phase-5/implementation-plan.md:34`. `cloneRouteConfig` copies routes by value and then explicitly clones existing slices. Simply adding `WeightedClusters` leaves it shared between HCMs using the same RDS config, violating the parser’s stated ownership guarantee. Add the clone update and a public parser test proving mutations remain isolated.

7. **P3 — Cancellation testing does not bound cancellation latency.** `docs/phases/phase-5/implementation-plan.md:21`. All deadlines begin *after* `ConfigDump` returns. An implementation that blocks indefinitely after cancellation never reaches those assertions; one returning with a lingering connection can pass after two seconds, despite the design requiring cleanup before return. Bound return latency from cancellation, and verify cleanup at the promised boundary.

8. **P3 — Upstream-filter coverage ignores an available real fixture.** `docs/phases/phase-5/design.md:172`, plan §5.2 step 6. Although OSS does not emit this configuration, the downstream 06-multi-extproc dump (former downstream corpus directory) contains router upstream filters in order: `late_ext_proc`, then `upstream_codec`. Add explicit parsed and rendered expectations for both. The existing “parses without errors” test passes when the field is discarded entirely.

9. **P3 — Plugin testing proves dispatch rather than delivered behaviour.** `docs/phases/phase-5/design.md:133`. Checking that `Chain` prefers a test mapper does not establish that `Annotate` preserves its result or that consumers receive it in JSON/rendering. Assert an observable custom origin through annotation, alongside an OSS fallback. Include a self-contained fixture: D-003 requires removal of the downstream corpus before the kgateway move.

10. **P4 — Derived annotations need an explicit Phase 6 comparison boundary.** `docs/phases/phase-5/design.md:145`, roadmap § Phase 6. Origins depend on caller-supplied `Gateway`, `Known`, mapper composition and metadata availability, rather than solely on Envoy behaviour. Embedding them in the snapshot without defining their exclusion from comparison lets equivalent configurations appear changed. State that Phase 6 compares configuration independently of origin annotations and certainty.
## Triage

| # | Finding | Source | Decision | Note |
|---|---|---|---|---|
| 1 | Origin types in `origin` plus origin fields on `model` form an import cycle | reviewer | accept | Value types move to `model`, and `origin` keeps the `Mapper` and implementations (design § Package layout, API). |
| 2 | Rule-name segment match is fooled by a rule named `a-matcher-b` | reviewer | accept | Verified: rule names allow `-matcher-`. Matching is now anchored to the end with the same match index; 08 adds rule `a-matcher-b`. |
| 3 | `ObjectForName` promises route-config lookup that `Mapper` can't express | reviewer | accept | Added `Mapper.RouteConfig(name)`. `ObjectForName` dispatches only through the mapper, and the Gateway comes from `KgatewayOptions.Gateway` (Unknown without it). |
| 4 | "Inferred equals Exact" contradicts returning ambiguous candidates | reviewer | accept | Now: candidates contain Exact without `Known`, and equal it with `Known` taken from a literal manifest list. |
| 5 | The downstream route-list helper would inherit the new OSS scenarios | reviewer | accept | Verified at `pkg/envoy/filter/e2e_test.go:68`. Changed to an explicit downstream list. |
| 6 | `WeightedClusters` is not deep-copied by `cloneRouteConfig` | reviewer | accept | Verified at `pkg/envoy/parser/parser.go:474`. The plan adds the clone and an isolation test through `Parse`. |
| 7 | Cancellation test doesn't bound return latency | reviewer | accept | Deadlines are now measured from the cancel, `ConfigDump` runs in a goroutine, and cleanup is asserted at return. |
| 8 | Real upstream-filter fixture is ignored | reviewer | accept | Verified: downstream 06 has `late_ext_proc` then `upstream_codec`. Explicit parse and render expectations added; the synthetic fixture stays for after E6. |
| 9 | Plugin test proves dispatch, not delivered behaviour | reviewer | accept | Checked through `Annotate` and a CLI golden on a synthetic dump, independent of the downstream corpus. |
| 10 | Phase 6 comparison must exclude origin annotations | reviewer | accept | Added design § Phase 6 boundary. |
