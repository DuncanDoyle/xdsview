# Consumer design review — kgwctl

**Scope:** design.md §5–§7 at `8d381f6`, from the kgwctl `proxy routes` side
**Source:** [kgwctl#14 comment](https://github.com/DuncanDoyle/kgwctl/issues/14#issuecomment-7166612167), by the kgwctl session (Claude), owner-approved. Same model family as the author: a consumer review, not a cross-model review.
**Date:** 2026-11-25

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | Blocking: Known and selection semantics undefined; "unresolved" contradiction | architecture | accept: §6 selection rules 1–6, `Narrowing` fields, no `narrowed` certainty; unresolved means no valid reading only |
| 2 | Blocking: `RuleIndex int` defaults to rule 0 | correctness | accept: `*int`, combined with `RuleName` by AND |
| 3 | `Notes` can't print narrowed readings inline | correctness | accept: `provenance.Labels` (additions 1) |
| 4 | `Assertion.Kind()` and JSON encoding | architecture | accept: additions 2 |
| 5 | ID lookup; source-relative route path | architecture | accept: additions 3 |
| 6 | Claim and candidate order; numeric ID compare | correctness | accept: additions 4 |
| 7 | `Row.Subject`; warnings and coverage in render | correctness | accept: additions 5 |
| 8 | `ix.Claims()`; records at every level | architecture | accept: additions 6 |
| 9 | `Conditional` drops the weighted-backend meaning | correctness | accept: additions 7, both meanings kept |
| n1–n4 | warning code constants; `NameAnswer.Kind`; `Names(opts)`; route kinds | correctness | accept: additions 8, §6 Names, selection rule 1 |
