Current Phase: Phase 4 — Library Extraction (NOT STARTED)

Phase 3 and sub-phases 3.1–3.5 are complete (`docs/plans/phase-3-complete.md`, `docs/plans/phase-3.5-complete.md`). The roadmap was restructured on 2026-11-04: krp is the Envoy data-plane library of the kgateway tooling program (`docs/plans/krp-roadmap.md`).

## Program
- Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling/program.md`)
- Waiting on: nothing
- Consumers: kgateway CLI (edge E1), kgateway-ui Phase 9 (E4), Envoy → Gateway API mapping (E5), move into kgateway (E6)

## Issues

### Carry-over (open issues, planned for Phase 5)
- **#11** (P2) — `prefix_rewrite` route action not captured
- **#13** (P3) — `weighted_clusters` not captured (traffic splitting)
- **#15** — Support HTTPRoute filters
- **#16** — RFE: `--rule <name>` when HTTPRoute rule name promoted to standard channel
- **#17** — Collect multi-route config dumps for E2E filter tests
