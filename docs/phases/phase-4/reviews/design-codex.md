Cross-model: YES (author: claude-opus-5-5, reviewer: Codex, command: codex exec --sandbox read-only)
Scope: design
Diff: n/a (design review)
Timing: on time

No P1 found. Ranked findings:

1. **P2 — Recording script does not specify how clients find its kubeconfig.** [implementation-plan.md:47](docs/phases/phase-4/implementation-plan.md#L47) gives kind a temporary `--kubeconfig`, but subsequent commands only receive context names. Those contexts are unavailable through default kubeconfig discovery unless the script propagates `KUBECONFIG` or `--kubeconfig`. Also propagate context into the TLS helper scripts: both currently invoke bare `kubectl` (e.g. `testdata/scenarios/02_1-single-policy/k8s/create-tls-cert-secret-api-example-com.sh:11`).

2. **P2 — Recording lacks scenario isolation and convergence criteria.** [implementation-plan.md:50](docs/phases/phase-4/implementation-plan.md#L50) says to record every setup script, without teardown between scenarios or waiting for the expected configuration to reach Envoy. Scenarios reuse `kgateway-system/gw` and `api-example-com`, while policy resources have different names. Sequential setup alone can retain previous policies; immediate collection can capture stale configuration. Specify cleanup and observable readiness before recording.

3. **P2 — Scenario 06 cannot enter the promised OSS corpus.** [implementation-plan.md:50](docs/phases/phase-4/implementation-plan.md#L50) records only scenarios with `k8s/setup.sh`, but `06-multi-extproc` contains only an existing dump. Moving that dump leaves no OSS replacement. This contradicts D-003’s “re-record all scenarios”; add reproducible manifests or explicitly decide its exception.

4. **P2 — Tunnel cleanup on acquisition failure is unspecified.** [implementation-plan.md:25](docs/phases/phase-4/implementation-plan.md#L25) assigns cleanup to the returned reader’s `Close`. If tunnel creation succeeds but the HTTP request fails, returns non-200, or is cancelled before returning a reader, that cleanup path is unavailable. The current CLI covers fetch failure with `defer pf.Stop()` (`cmd/krp/main.go:130`). Require equivalent cleanup on every error after tunnel creation, with failure-path tests.

5. **P3 — Naming-convention tests can pass despite dropping expected routes.** [implementation-plan.md:55](docs/phases/phase-4/implementation-plan.md#L55) carries forward content-dependent filter assertions. The existing e2e test checks only a nonempty listener list and the same substring used by the implementation (`internal/filter/e2e_test.go:43–55`). Losing some expected routes still passes. Assert the expected retained route set from scenario identities, including rule selection and exclusion of other HTTPRoutes, for each version.

6. **P3 — Reader migration has no tests for its new failure boundary.** [implementation-plan.md:22](docs/phases/phase-4/implementation-plan.md#L22) only changes fixtures to `os.Open`. It does not test reader errors or trailing input. The existing `json.Unmarshal` rejects trailing garbage/multiple documents; a single `json.Decoder.Decode` would silently accept them. Add observable tests for those cases and an interrupted reader.

7. **P4 — Public snapshots have no ownership/mutation contract.** [design.md:49](docs/phases/phase-4/design.md#L49) exports `Filter` without defining whether results share mutable data with their input. The current filter copies route structs while sharing nested maps, slices, and pointers (`internal/filter/filter.go:115`); parser cloning also shares nested map values (`internal/parser/parser.go:486–495`). Phase 5 mapping/provenance annotations could consequently alter cached or sibling views. Specify read-only sharing or independent ownership before consumers depend on the API.

Author-reported tests/vet at `e1c5666` were accepted as supplied; I ran no tests and edited no files.
## Triage
| # | Finding | Source | Decision | Note |
|---|---|---|---|---|
| 1 | Recording script kubeconfig not propagated; TLS helpers use bare kubectl | reviewer | accept | Plan 4.3 step 2, 4: `--kubeconfig` + `--context` on every call, TLS helpers included |
| 2 | No scenario isolation or convergence before recording | reviewer | accept | Plan 4.3 step 4: Accepted wait, stable `/config_dump`, teardown between scenarios; teardown.sh deletes policies |
| 3 | Scenario 06 has no manifests, so no OSS replacement | reviewer | accept | New OSS manifests for 06; D-003 amended (uncommitted) |
| 4 | Tunnel cleanup on failure after tunnel creation unspecified | reviewer | accept | Design § API; plan 4.1 step 6 with failure-path tests |
| 5 | Filter e2e checks only substring, can pass with dropped routes | reviewer | accept | Per-scenario expected route sets; exact set, per-rule subset, other-route exclusion |
| 6 | Reader migration lacks failure-boundary tests | reviewer | accept | `io.ReadAll` + `json.Unmarshal` keeps trailing-data error; tests for reader error and trailing data |
| 7 | No ownership/mutation contract for snapshots | reviewer | accept | Design § Ownership: read-only by contract; derived data as separate structures |
