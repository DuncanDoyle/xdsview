# Sub-phase 9.1 complete — snapshot and occurrence IDs

**Date:** 2026-11-25 | **Kind:** planned | **Base:** 8d381f6 | **Commits:** 5fa200d, e496ad6, 1be3d88, 4c1f12e

## What changed
- `model`: `EnvoySnapshot` is `Snapshot` with `Capture`, `RouteConfigs` and typed `Warnings`. `ID` with `Within`, `RouteConfigSource`, `RouteConfigUse` (on `HCMConfig`), `Prune`.
- `parser.Parse(r, parser.Options)` returns `*model.Snapshot`. IDs follow design §2. Static and dynamic route configs and inline HCM configs are sources. RDS names resolve against dynamic sources only; one name with two dynamic sources is `ambiguous`, not joined, with an `rds-ambiguous` warning on the use.
- `source.Source.Capture()`: `File` unverified, `Admin` and `kube` complete. krp passes it to the parser.
- `origin`, `filter`, `chain.Annotate` keep the new snapshot fields. Text output names an ambiguous route config.

## Tests
Acceptance tests: `parser/snapshot_test.go`, `model/id_test.go`, `model/prune_test.go`, `source/capture_test.go`, `source/kube/capture_test.go`, `cmd/krp` `TestDump_DuplicateRDSName`.

Before the implementation (skeleton API only), these failed for the expected reason. Output: a local capture, not published.
- `TestRouteConfigs_DuplicateName`: no sources, and the use joined the later `dup` (vhost `second`).
- `TestRouteConfigs_SharedSource`, `_SourcesAndUses`, `_Scenario00`: no sources, no uses.
- `TestIDs_UniqueInEveryDump`, `TestIDs_Within`: empty IDs.
- `TestWarnings_Typed`: no code, no subject.
- `TestPrune_*`: nothing pruned.

`TestParse_Deterministic` and the capture tests passed before. The capture methods are trivial and were written with the skeleton. `TestDump_DuplicateRDSName` was written after the parser change.

After: `go test ./...` and `go vet ./...` pass.

## Golden changes
- Text (`render` and krp): none. No corpus dump has a duplicate RDS name.
- JSON (all 56 krp goldens): only additions. They gain `capture`, `routeConfigs`, an `id` on every occurrence, and `routeConfigUse`. No corpus dump has a parse warning, so `snapshot.warnings` is absent. Filtered goldens keep every source; `filter` does not prune them.

## Deviations
1. **Scenario 00 `main_route`.** Its inline config sits in a static listener. Static listeners are parsed in 9.4, so `main_route` and `prometheus_route` stay `RS` sources with no use. The inline-source rule (`<HCM>/RCI` records `Reported`) is implemented and tested on a hand-built dump with a dynamic listener. 9.4 must turn `TestRouteConfigs_Scenario00` into the brief's check.
2. **Inline HCM `route_config` on dynamic listeners** is parsed now, since the inline source needs it. No corpus dump has one, so no golden changed.
3. **Not-found RDS is `unknown`**, never `missing`. `missing` needs the 9.4 capture rules.
4. **Network filters.** Only the HCM has an ID (`.../N<i>`, its position in the chain). Other network filters are listed in 9.4. `hcm-skipped` warnings name the filter chain, since a skipped HCM has no occurrence.
5. **Sources carry no virtual hosts.** Each use holds its own copy, with use-scoped IDs. `RouteConfigSource` has ID, kind, name and `Reported`.
6. **krp JSON stays `v1alpha1`.** The changes are additive (D-005). Top-level `warnings` stay strings; the typed ones are in `snapshot.warnings`.

## For 9.2
- `chain.Effective` and `chain.Annotate` still take `HCMConfig`; slot IDs are on `HTTPFilter.ID`.
- Render prints `route config <name> ambiguous: ...` from `RouteConfigUse.State`. The hooks still key on indices.
- `model.Prune` exists beside `filter.Filter`. krp still uses `filter`.
