# Decision log

Append-only. Newest at the bottom. Supersede, don't edit.

## D-001 — Adopt the agentic-engineering-sdlc workflow (2026-11-04, phase 4)
**Context:** krp, kgateway-ui and kgwctl are developed in parallel. krp used project-local phase commands and a `docs/plans/` layout; the other two use the agentic-engineering-sdlc skill.
**Decision:** krp uses the skill from Phase 4 on. Docs moved: `docs/plans/krp-roadmap.md` → `docs/roadmap.md`; phase design, plan and `-complete` records → `docs/phases/phase-<N>/` (`design.md`, `implementation-plan.md`); the 2026-03-06 design and plan → `phase-1/initial-design.md`, `initial-implementation-plan.md`; the 2026-03-07 improvement plan → `phase-1/reviews/initial-design-claude.md`; `possible-future-phases.md` and `TODO.md` → the roadmap's Deferred section.
**Alternatives:** Keep the krp commands — rejected: two workflows across one program, and they lack the cross-repo program step.
**Consequences:** Phases 1–3 records are not backfilled to the templates. Phase 4 starts with a recorded base SHA and a cross-model design review.

## D-002 — krp is the Envoy data-plane library (2026-11-04, phase 4)
**Context:** kgateway-ui (Phases 2–5) and kgwctl cover the Gateway API resource graph, policies and status that krp's old Phases 4–7 planned. Both need krp's Envoy view as a library.
**Decision:** Phases 4–6 are library extraction, data-plane accuracy and mapping, and intended vs accepted config. krp owns the Envoy name → Gateway API ref mapping (program P-001). Vendor policy kinds plug in from outside the core (program P-002).
**Alternatives:** Build the Gateway API graph in krp — rejected: duplicates the UI and the CLI.
**Consequences:** The old Gateway API track is in the roadmap's Deferred section. Exported packages are a contract tracked in the program file.
