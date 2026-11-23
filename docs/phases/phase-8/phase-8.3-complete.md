# Sub-phase 8.3 complete — mapper vendor-neutral

**Date:** 2026-11-23 | **Kind:** planned | **Base:** 251a451 | **Edge:** E6 (part) | **Decisions:** D-012, D-013, D-018

## Outcome
The core kgateway mapper links a policy to a filter only for `gateway.kgateway.dev/TrafficPolicy`. Other kinds and groups stay listed on `Route.Policies` (exact), unlinked. The enterprise transformation type left `origin` and the sides table. A plugin in `origin.Chain` adds such a link back.

| # | Case (composed path) | Result |
|---|---|---|
| 1 | TrafficPolicy `extAuth` | Linked to `ext_authz`, inferred |
| 2 | `example.com/TrafficPolicy` | Listed, not linked (failed before) |
| 3 | `gateway.kgateway.dev/AcmePolicy` | Listed, not linked (failed before) |
| 4 | Vendor transformation filter | No contribution, sides unknown (failed before) |
| 5 | Case 4 with a plugin claiming the type | Linked, inferred |

Tests: `TestVendorNeutral_PolicyKind`, `TestVendorNeutral_Transformation` (`pkg/envoy/chain/vendorneutral_test.go`). `TestKgateway_PolicyFilter` passes full TrafficPolicy refs; its vendor-type case moved to the plugin test.

## Gates
- `go test ./...` and `go vet ./...` pass. No golden changed.
- `FilterTransformations` in Go: only the plugin test at close; after the phase review, nowhere (synthetic type).
- `enterprise|solo|gloo|_ee` in Go under `pkg cmd internal`: no match.

## Reviews
Planned sub-phase: reviewed in the phase review before `v0.6.0`.

## Deferred
None.
