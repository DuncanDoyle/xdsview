# Sub-phase 6.5 — Attribution gaps and route hook (`v0.4.1`)

Kind: stabilization (plus D-010, owner-approved)
Base: b4cf6ab
Issues: #28 | Source: kgwctl 3.2 on a kgateway v2.5.1 dump

## Problem / goal
On OSS kgateway, policy attribution misses three of the four policy-driven filters. A bad route config hides every route. kgwctl needs a route-scoped render hook (D-010).

## Expected behaviour

| # | Input | Expected |
|---|---|---|
| 1 | Route metadata `rateLimit.local`; `ratelimit/local` (LocalRateLimit) has per-route config | The entry gets the `rateLimit.local` policy contribution |
| 2 | Route metadata `transformation`; `dynamic_modules/simple_mutations` (DynamicModuleFilter, `filter_name: rustformation`) has per-route config | The entry gets the `transformation` contribution. The downstream transformation mapping stays. |
| 3 | Route metadata `extAuth`; `global_disable/ext_auth` enabled per route, ext_authz not configured for the route | The `global_disable/ext_auth` entry gets the `extAuth` contribution. Likewise `extProc` → `global_disable/ext_proc`. |
| 4 | A `RoutesConfigDump` with one malformed route config and one valid one | The valid one is joined to its HCM. One warning names the skipped route config. |
| 5 | `render.Options.RouteExtra` set | Its lines print under each route line, before the filters, at the route body indent, with the route's `RouteRef` |
| — | `03`, `02_1` corpus, and kgwctl's `policies.json` (copied in as `testdata/scenarios/09-kgwctl-policies`) | Literal expected contributions, checked through `parser.Parse` and `chain.Annotate` |

## Root cause (if known)
- #1–#3: the field table in `origin/kgateway.go` was built from the downstream corpus and the `03` scenario only.
- #4: `routesConfigDump` decodes all route configs in one `json.Unmarshal`.

## Acceptance tests
`origin` (field table), `chain` (corpus contributions), `parser` (#4), `render` (#5). Goldens are regenerated, and the diff is reviewed.

## Scope
In: `pkg/envoy/{origin,parser,render}`, tests, goldens, README, a new corpus dump.
Out: other policy fields not seen in a dump.

## Plan / design impact
D-010.

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] `go test ./...` and `go vet ./...` pass
- [ ] cross-model review of `b4cf6ab..HEAD`, triaged
- [ ] phase-6.5-complete.md; `v0.4.1` tagged (owner OK given); #28 closed; kgwctl told
