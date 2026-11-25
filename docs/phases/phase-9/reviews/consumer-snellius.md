# Consumer design review — Snellius

**Scope:** design.md at `8d381f6`, from the Snellius Phase 12 side
**Source:** [snellius#70 comment](https://github.com/DuncanDoyle/snellius/issues/70#issuecomment-7166457485), by the Snellius session (Claude). Same model family as the author: a consumer review, not a cross-model review.
**Date:** 2026-11-25

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | P1 no gap for policy-attributable config without a `merge.*` record | architecture | accept: design §5 expected gap 1 |
| 2 | P2 claims dropped by Compose leave no gap | architecture | accept: expected gap 2 |
| 3 | P2 no gap for a missing `GatewaySource`; ListenerSet | architecture | accept: expected gap 3; ListenerSet listeners not attributed in Phase 9, they show the gap |
| 4 | P3 `Objects` lacks the claim kind | architecture | accept: `Dependent{Object, Kind}` |
| 5 | P3 ambiguous candidates must be complete | correctness | accept: §6 Names |
| 6 | P3 `kgateway.Names(opts)` undefined; versions | correctness | accept: no options; names its checked minors (R14) |

Snellius confirms `NameMapper` covers every `EnvoyNamer` call site (each knows its kind) and that identity needs no change.
