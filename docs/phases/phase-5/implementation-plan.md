# Phase 5 — Data-Plane Accuracy and Mapping: Implementation Plan

**Design:** `docs/phases/phase-5/design.md`
**Base:** `badf0ab`

The work is split into four planned sub-phases. Each one leaves `go test ./...` and `go vet ./...` green.

| Sub-phase | Scope | Depends on |
|---|---|---|
| 5.1 | #25 cancellation | — |
| 5.2 | Model accuracy (#11, #13, #15, `HTTPFilter.Type`) and corpus (#17, 07, 08, `route_source`) | — |
| 5.3 | `pkg/envoy/origin` and the filter changes (#16) | 5.2 corpus |
| 5.4 | CLI: annotate, render origins, `--rule <name>`, golden files, README | 5.3 |

Phase close: cross-model phase review, tag `v0.2.0`, program file (E5 available, E4 unblocked), `cross-repo` notices to kgwctl and kgateway-ui.

---

## 5.1 — Cancellation (#25)

1. **Failing test first.** In `pkg/envoy/source/kube`, start a local TLS listener that accepts the connection and never answers the upgrade. Build a `rest.Config` pointing at it, and run the real `portForward` path with a fake pod finder. Then cancel ctx after the connection is accepted. Assert three things, each within 2s of `ConfigDump` returning:
   1. `ConfigDump` returns `context.Canceled`.
   2. The server sees the connection closed.
   3. `ForwardPorts` has returned, observed through a test hook on the goroutine's exit and not through goroutine counts.

   Run it before the fix and record that it fails.
2. **Fix.** Make the connect honour ctx. Prefer an upgrade request built with ctx (`NewRequestWithContext` through a small dialer that implements `httpstream.Dialer`). If client-go makes that impractical, close the underlying connection on `ctx.Done()`. Whichever approach is used, write down the reason in the sub-phase record.
3. Keep the Phase 4 fake-tunnel tests green. Add the pod-finder seam only if step 1 needs it.

## 5.2 — Model accuracy and corpus

1. **Parser fields, test first.** Write synthetic fixtures for each field, then implement:
   - `prefix_rewrite` → `Route.Rewrite.Prefix`. `Rewrite` is non-nil when either rewrite is set.
   - `weighted_clusters.clusters[]` → `Route.WeightedClusters`. `Cluster` stays empty for split routes.
   - router `upstream_http_filters` → `HCMConfig.UpstreamFilters`, parsed like `http_filters`.
   - `typed_config.@type` → `HTTPFilter.Type`. For `ExtensionWithMatcher`, use the inner `extension_config.typed_config.@type`.
2. **Renderer.** Show the prefix rewrite, the weighted backends as `name (weight)`, and an "upstream filters" block after the router. Add renderer unit tests for each.
3. **Scenarios.** Write the manifests and README for 03, 04, 05, 07 and 08, with expected route names in the README derived from the manifests:
   - 03: three TrafficPolicies on one HTTPRoute.
   - 04: three rules, where rule 1 has two matches, backed by two Services.
   - 05: HTTP and HTTPS listeners, with the HTTPRoute attached to both through two parentRefs or no sectionName.
   - 07: one rule with weights 80/20.
   - 08: rules named `a` and `a-b`.
4. **Recording script.** Add `--values <file>` to `hack/record-dumps.sh`, passed to the kgateway Helm install, and `--suffix <s>` for the output directory (`kgateway-<version><suffix>`). Record:
   1. all new scenarios on v2.4.7 and v2.5.0;
   2. 04 again with `enableRouteSourceMetadata: true` into `kgateway-<version>-route-source`.

   If a scenario can't be expressed in OSS, write that in its README and skip it.
5. **Filter e2e.** Extend `scenarioRoutes` with literal names for the new scenarios, including per-rule expectations for 04 and the multi-listener case in 05. `TestFilter_E2E_OSSCorpusComplete` already requires them.
6. **Parser e2e.** It covers the new dumps automatically. Add assertions that 07 yields `WeightedClusters` and that the policy filters have the expected `Type`.
7. Fix the "backend index" wording in `filter`'s package doc and `routeMatches` to "match index".
8. Regenerate the golden files. The diff should contain only additions: `type` on filters, plus the new scenarios.

## 5.3 — `pkg/envoy/origin`

1. **Types and interface** as in the design. Doc comments cover how certainty is decided and which versions are verified.
2. **Kgateway mapper**, test first, against literal per-version tables:
   - `Listener`: `listener~<port>` gives the port and the Gateway from options.
   - `VirtualHost`: split on the last `~`, map `_` back to `.`, and treat the prefix as the chain or section name.
   - `Route`: `route_source` metadata first. Otherwise locate `-httproute-` after the vhost prefix and `-matcher-<n>` at the end, then enumerate every split of the middle into `<name>-<ns>-<ruleIdx>-<matchIdx>[-<ruleName>]` that gives valid DNS-1123 names. One split is Inferred. Several splits are resolved against `Known`, and stay Ambiguous otherwise.
   - `Cluster`: `kube_<ns>_<svc>_<port>` gives a Service. Other prefixes return Unknown in this phase.
   - `HTTPFilter`: the kind from `Type`, the extension from the `<kind>/<ns>/<name>` suffix, and `global_disable/*` as a helper.
   - `Policies`: every `merge.*` key, with entries parsed as `<group>/<Kind>/<ns>/<name>`.
3. `Chain`, `Annotate` (copy-on-write, following the filter package's ownership rule) and `ObjectForName`.
4. **Model origin fields.** They are pointer or slice fields with `omitempty`, and only `Annotate` sets them.
5. **Tests:**
   - per-version e2e tables over the OSS corpus;
   - Inferred equals Exact on the `route_source` recordings;
   - synthetic ambiguity cases;
   - the plugin-seam test on the enterprise corpus;
   - `origin` added to the boundary test.
6. **Filter.** Add `Options.RuleName`. It matches on the route origin's `RuleName` when the route has `route_source` metadata, and on the exact `-<ruleIdx>-<matchIdx>-<ruleName>-matcher-` segment otherwise. Setting `RuleIndex` and `RuleName` together is the caller's error, and `Filter` documents it. E2e on 08 checks that `a` doesn't select `a-b`.

## 5.4 — CLI

1. `krp dump` runs `origin.Annotate` with `origin.Kgateway`. With `--gateway`, it sets `KgatewayOptions.Gateway`.
2. **Text renderer.** Add a dim origin line under each route (`HTTPRoute ns/name · rule N[ name] · match M`), each policy filter (`GatewayExtension ns/name`) and each route's policies (`TrafficPolicy ns/name (cors)`). Ambiguous and Unknown origins print nothing. Add renderer tests.
3. **`--rule`.** Accept an integer or a name; a value that parses as an integer is an index. Update the help text and README.
4. Regenerate the golden files, then review that the diff only adds fields.
5. Update the README (origins, `--rule <name>`, library usage of `origin`) and the roadmap.

## Phase close

1. Cross-model phase review of `git diff badf0ab..HEAD` → `reviews/phase-codex.md`, then triage.
2. `phase-5-complete.md`, roadmap and `phase-status.md` reset to Phase 6.
3. Tag `v0.2.0` after owner approval.
4. Program file: E5 available at `v0.2.0`, and E4's `source/kube` caveat removed. Send `cross-repo` notices to kgwctl and kgateway-ui.
