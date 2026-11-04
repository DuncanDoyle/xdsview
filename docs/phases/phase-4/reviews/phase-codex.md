Cross-model: YES (author: claude-opus-5-5, reviewer: Codex, command: codex exec --sandbox read-only)
Scope: phase 4
Diff: e1c5666..ee2030e
Timing: on time

1. **P1 — Recording cleanup can delete an existing cluster.** `hack/record-dumps.sh:21`: the EXIT trap unconditionally deletes the fixed name `krp-record`, even before creation succeeds. If that cluster already exists, `kind create` fails and cleanup deletes it; concurrent recordings can delete each other’s cluster. Use a unique name and track ownership before cleanup.

2. **P1 — Cancellation leaves an in-flight port-forward upgrade running.** `pkg/envoy/source/kube/portforward.go:137`: the SPDY dialer uses an HTTP client without a timeout; client-go’s `Dial` creates its request without the supplied context. Cancellation at line 153 closes `stopCh` and returns, but `ForwardPorts` only observes that channel after dialing completes. A stalled upgrade therefore retains its goroutine/connection after `ConfigDump` returns. This inherited behavior now affects repeated calls through the public library; the fake opener tests bypass it.

3. **P3 — “Exact route sets” are still counts plus implementation-shaped assertions.** `pkg/envoy/filter/e2e_test.go:106`: the test checks count, the same substring marker used by `Filter`, and membership in the parsed input. Duplicating one matching route while losing another can pass; per-rule checks at line 122 compare only counts. This leaves the accepted design-review finding unresolved and weakens Phase 5 naming validation. Compare independently specified route-name multisets for each selection.

4. **P3 — Filter coverage does not span both corpora as promised.** `pkg/envoy/filter/e2e_test.go:167`: enterprise filtering tests only `00-simple`; the scenario/version loop covers OSS exclusively. Design § Test corpus and plan 4.3 step 6 require filtering every dump in both corpora. Enterprise TLS and multi-extproc variants therefore lack the promised selection checks.

Author test results accepted as supplied. No tests run or files edited.
## Triage
| # | Finding | Source | Decision | Note |
|---|---|---|---|---|
| 1 | Recording cleanup can delete an existing `krp-record` cluster | reviewer | accept | Fixed: per-run name `krp-record-$$`, refuse if it exists, delete only when this run created it. Smoke-recorded 00-simple on v2.5.0 afterwards: exit 0, cluster gone, `portal-gate5` untouched. |
| 2 | Cancelled ctx leaves an in-flight SPDY dial running | reviewer | defer: Q9 (P2), GitHub issue to file | Pre-existing (the base CLI had the same `select`). client-go's SPDY dialer takes no context, so a fix needs a dial timeout or a context-aware transport; that's design work for Phase 5. `ConfigDump` still returns promptly, and the goroutine ends when the dial completes or fails. |
| 3 | Filter e2e checks counts plus the filter's own marker, not exact names | reviewer | accept | Fixed: literal expected route names per HTTPRoute and per rule, compared as sorted lists; the marker and membership checks are removed. Mutating `filter.go` to drop the namespace fails 02_1, 02_3, … on both versions. |
| 4 | Filter e2e covers only OSS, plus enterprise 00-simple | reviewer | accept, except 06 | Fixed: `TestFilter_E2E_EnterpriseScenarios` covers every enterprise scenario except 06. That one is a Gloo Gateway v1 dump (`gloo-system_api-example-com-route-0-matcher-0`) with no HTTPRoute identity in its route names, so there is nothing to filter. |
