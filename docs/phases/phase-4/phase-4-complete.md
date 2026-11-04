# Phase 4 — Library Extraction (Complete)

**Completed:** 2026-11-04
**Range:** e1c5666..v0.1.0

## Summary
krp's Envoy core is now an importable Go library that kgwctl and kgateway-ui can depend on. `pkg/envoy/{model,parser,filter,source}` uses only the standard library, and port-forwarding lives in `pkg/envoy/source/kube`. `krp dump -o json` prints the model in a versioned envelope, and golden files pin its shape. CLI text output is byte-identical to the base. The tests run against OSS kgateway v2.4.7 and v2.5.0 recordings, with the earlier enterprise recordings kept as a second corpus.

## Sub-phases (phase record only)
| Sub-phase | Kind | Issues | Description |
|---|---|---|---|
| 4.1 | planned | — | Library move to `pkg/envoy`, `Parse(io.Reader)`, `source` and `source/kube`, import-boundary test, client-go v0.36.2, Apache-2.0 LICENSE (`phase-4.1-complete.md`) |
| 4.2 | planned | — | `-o json` envelope and golden contract files (`phase-4.2-complete.md`) |
| 4.3 | planned | — | OSS corpus recorded on v2.4.7 and v2.5.0, enterprise corpus moved to `testdata/enterprise/`, recording script (`phase-4.3-complete.md`) |

## Reviews
- `reviews/design-codex.md`: 7 findings, 7 accepted
- `reviews/phase-codex.md`: 4 findings, 3 accepted and fixed (`8afe748`), 1 deferred (Q9)

## Waivers
- "Accepted P1s fixed": Codex ranked finding 2 (kube source keeps a port-forward connection open when a call is cancelled mid-connect) P1. It was deferred at P2 because only long-running library users are affected, and none exist yet. Owner, 2026-11-04: "Yes, that makes sense." It must be fixed before kgateway-ui's backend uses the kube source.

## Deliverables
- Packages: `pkg/envoy/model`, `parser`, `filter`, `source` (stdlib only, enforced by `pkg/envoy/boundary_test.go`) and `pkg/envoy/source/kube` (client-go v0.36.2).
- `krp dump -o json`: `{schemaVersion: "v1alpha1", snapshot, warnings}`. `warnings` and `snapshot.listeners` are always arrays. Golden files in `testdata/golden/` (60 files, including a no-match case).
- Test corpus: `testdata/scenarios/*/envoy/kgateway-v2.{4.7,5.0}/` (19 scenarios each, with `server_info.json`) and `testdata/enterprise/`.
- Scripts: `hack/record-dumps.sh`, `hack/scenario.sh`.
- `LICENSE` (Apache-2.0); `go 1.26.0`.
- Decisions D-003, D-004, D-005.

## Notes for later phases
- **Filter names are labels, not types.** OSS kgateway emits one filter per `GatewayExtension`, named `<kind>/<ns>/<name>` (`ext_auth/...`, `ext_proc/...`, `ratelimit/...`), plus `global_disable/<kind>` helper filters. All are disabled on the listener and enabled per route. Transformation is the dynamic module `dynamic_modules/simple_mutations`. Enterprise uses `extauth_ee/default` and `io.solo.transformation`. The Phase 5 mapping should key on `typed_config.@type` plus the name pattern, and leave vendor names to the plugin seam (P-002).
- HTTPS virtual hosts are named after the listener (`https-api~api_example_com`), not `listener~443`.
- `testdata/enterprise/` must be removed before the move into kgateway (E6).
- staticcheck wasn't run: the installed version is too old for Go 1.27. `go vet` is clean.
- The no-match warning ("no routes found for HTTPRoute ...") goes to stderr only, not into the JSON `warnings`.

## Deferred / open
- #25 (Q9, P2): kube source keeps a port-forward connection open when a call is cancelled mid-connect — Phase 5
- #11, #13, #15, #16, #17 — Phase 5, unchanged
