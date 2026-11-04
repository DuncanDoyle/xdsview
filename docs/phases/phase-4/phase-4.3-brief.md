# Sub-phase 4.3 — Test corpus

Kind: planned
Base: 99a678f7cfe2f9f1fabb45475630249106347f35
Issues: n/a | Plan steps: implementation-plan.md § 4.3 steps 1–8

## Problem / goal
Every test dump was recorded on the enterprise distribution (and 06 on Gloo Gateway v1, `gloo-system`), so krp has never been tested against OSS kgateway, the platform it's headed for. The filter e2e tests also check only a substring, so they would pass even if routes went missing.

## Expected behaviour

| Input / action | Expected |
|---|---|
| `hack/record-dumps.sh v2.4.7` / `v2.5.0` | one kind cluster (own kubeconfig, deleted at the end); every recordable OSS scenario dumped to `envoy/kgateway-<version>/config_dump.json` with `server_info.json` beside it; `~/.kube/config` untouched |
| A scenario dump | taken only after its HTTPRoute (and policies) are Accepted and two consecutive `/config_dump` reads are equal; the previous scenario is fully torn down first |
| `go test ./...` | parser e2e over every dump in both corpora; filter e2e asserts the exact kept route set, the per-rule subset, and that other HTTPRoutes are dropped, for every OSS version |
| 02_1/02_6/02_7/02_8/06 OSS | expressed with OSS `TrafficPolicy`/`GatewayExtension`; any gap written in the scenario README |
| `testdata/enterprise/` | existing dumps and manifests, unchanged, with a README saying the folder is removed before the move into kgateway |

## Root cause (if known)
n/a (planned work)

## Acceptance tests
- Filter e2e with per-scenario expected route names, derived from the manifests, for each OSS version. To check that it bites, one expected name is altered to confirm failure, then restored.
- Parser e2e across all dumps.
- Golden files regenerated, with the diff reviewed.

## Scope
In: `testdata/`, `hack/`, e2e tests in `pkg/envoy/{parser,filter}`, `cmd/krp` golden files, roadmap Test Scenarios table, README.
Out: parser accuracy (#11/#13/#15/#17, Phase 5), library API.

## Plan / design impact
The recording lives in one generic `hack/scenario.sh up|down <dir>` instead of per-scenario setup/teardown scripts (the enterprise copies keep theirs). Scenario-specific steps, such as the TLS secrets, become `k8s/pre-*.sh` hooks that receive kubeconfig and context. This still meets design § Test corpus; it's recorded here, not as a decision.

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] full test suite passes
- [ ] cross-model review (planned: at the phase review)
- [ ] phase-4.3-complete.md written; phase-status.md updated
