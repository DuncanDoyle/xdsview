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

## D-003 — Test corpus: OSS main, enterprise kept separately (2026-11-04, phase 4)
**Context:** Every existing dump came from the enterprise distribution, and four scenarios use enterprise policy APIs. krp is headed into kgateway OSS (program E6).
**Decision:** Re-record all scenarios on OSS kgateway v2.4.7 and v2.5.0 as the main corpus, rewriting the four enterprise scenarios (and adding manifests for 06, which had none) with OSS `TrafficPolicy`/`GatewayExtension`. Keep the existing enterprise dumps and manifests in `testdata/enterprise/` as extra parser coverage (owner's choice).
**Alternatives:** OSS only, removing the enterprise corpus — not chosen; OSS v2.5.0 only — rejected: 2.4 is still supported.
**Consequences:** `testdata/enterprise/` is removed before the move into kgateway. Tests run on both corpora.

## D-004 — Standard-library core; Kubernetes in `source/kube` (2026-11-04, phase 4)
**Context:** kgateway-ui's core may not import client-go (UI D-016), and kgwctl brings its own `*rest.Config`.
**Decision:** `pkg/envoy/{model,parser,filter,source}` depend only on the standard library and each other, enforced by a test. Port-forwarding lives in `pkg/envoy/source/kube` and takes an optional `*rest.Config`.
**Alternatives:** One `source` package with client-go — rejected: every consumer would pull in client-go.
**Consequences:** Adding a dependency to the core is a decision entry, not a drive-by import.

## D-005 — `-o json` envelope belongs to the CLI (2026-11-04, phase 4)
**Context:** Consumers need a stable JSON shape to test against; the library's contract is its Go types.
**Decision:** `krp dump -o json` prints `{schemaVersion: "v1alpha1", snapshot, warnings}`. The envelope lives in `cmd/krp`; golden files in `testdata/golden/` pin the shape. `schemaVersion` changes only on removals or renames.
**Alternatives:** Envelope type in `pkg/envoy/model` — rejected: library consumers don't need it.
**Consequences:** Golden diffs in a PR are contract changes and get reviewed as such.
