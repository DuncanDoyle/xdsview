# Phase 4.2 — `-o json` (Complete)

**Completed:** 2026-11-04
**Range:** 43b4bab..1ef9633

## Summary
`krp dump -o json` prints the (filtered) snapshot as `{schemaVersion: "v1alpha1", snapshot, warnings}`. Golden files in `testdata/golden/` pin the shape for every test dump; consumers test against them.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Text output unchanged | The 4.1 capture (160 files) matches the base, except the usage text of 8 invalid-flag cases, each of which gains the `-o` line. A second capture with `--route-ns kgateway-system` (114 files, 57 of them non-empty) also matches. |
| `-o json` envelope | `TestDumpJSON_Golden`: 19 dumps, with the envelope checked independently of the golden file (`schemaVersion`, `snapshot`, `warnings` array) |
| Filtered `-o json` | `TestDumpJSON_Filtered`: `--route` and `--rule 0` on 01_1. Each golden file holds exactly the `...httproute-api-example-com-kgateway-system-0-0-matcher-0` route. |
| Warnings in JSON and on stderr | `TestDumpJSON_Warnings` |
| `-o json --interactive`, `-o yaml` rejected | `TestDumpOutput_Rejected` |
| Fail before | `go test ./cmd/krp` failed to build before the change (`undefined: newRootCmd`); without golden files it fails reading them |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |

Files: `cmd/krp/output.go` (envelope), `cmd/krp/main_test.go`, `testdata/golden/` (21 files, 96K), README § JSON output. The CLI now writes through cobra's `Out`/`Err` writers, which default to stdout and stderr.

## Notes for later phases
- A filter that matches nothing yields `"listeners": null`, not `[]`. That comes from the model's tags (no `omitempty`, nil slice). Normalising it would change the model contract, so it's left for the phase review to decide.
- The 4.1 byte comparison filtered on `--route-ns default`, which only 00-simple matches; every other scenario's HTTPRoute is in `kgateway-system`. The `kgateway-system` capture above closes that gap.
- 4.3 moves the dumps. `goldenPath` maps any `<corpus>/<scenario>/envoy/[<version>/]config_dump.json`, so after the move the golden files only need `-update` and a review.

## Deferred / open
none
