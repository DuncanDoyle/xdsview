# Sub-phase 8.1 complete — certainty on attribution (`v0.5.0`)

**Date:** 2026-11-22 | **Kind:** planned | **Base:** d39f5db | **Edge:** E13 | **Commits:** 04c5e07, a3cfd77, 355068a

## Outcome
Every attribution states its certainty. In JSON, `certainty` appears on each contribution and policy ref, and `indexCertainty` on a route origin whose indices partly came from the Envoy name. In text, the labels are ` (inferred)` and `(partly from name)`, and any other non-exact certainty is printed by name. A policy is linked to a filter only when exactly one filter has per-filter config for the route and matches the field. That counts filters configured at any level and on weighted backends. The link is no stronger than its policy ref, and at most `inferred`.

## Brief cases
All 14 cases pass. Case 14 was corrected (D-017): dump 09 keeps the `public-no-extauth` link, now `(extAuth, inferred)`.

## Golden diff
51 text and 20 JSON goldens changed. They differ only by certainty fields and suffixes. **No contribution was dropped** on any recorded dump: 28 policy and 24 extension contributions, and 32 policy refs, before and after.

## Reviews
- `reviews/8.1-codex.md`: 7 findings, all accepted and fixed.
- `reviews/8.1-fixes-codex.md`: 3 findings, all accepted and fixed. The owner checked the TUI by hand on dump 09 (2026-11-22).

## Consumer impact (kgwctl)
- Go API: additive.
- `TestBuildPolicyChain` compares a whole `Contribution`, so it needs `Certainty: model.Inferred`.
- Text and JSON goldens gain the labels.

## Deferred
None.
