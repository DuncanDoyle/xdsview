# Investigation — xDS convergence (program P-009)

**Date:** 2026-11-24
**Status:** revised after cross-model review (`reviews/xds-convergence-codex.md`). Design only; no code until the owner approves.
**Question:** Does each proxy run the configuration the controller published for it? How can an operator tell, and who provides each piece (program edge E14)?
**Sources:**
- kgateway `v2.5.2`, the version captured. `main` at `1a44dc2cab` is cited only where it is identical. `api/settings/settings.go` and `proxy_syncer/publish_gate.go` differ, so those are cited at `v2.5.2`.
- Envoy: source at `ef0e49cf9e`. The captured proxies run Envoy `1.39.6` (`aacd0340`).
- Snellius: `d880f13`.
- Captures: kind with three nodes, kgateway `v2.5.2`, Gateway API `v1.7.0`. One Gateway with two proxy replicas (`q8vvd` on worker2, `wzvn6` on worker), 2026-11-24. Every proxy capture checks the pod name Envoy reports (`server_info` `service_node`). A first capture set failed this check and was discarded (review #1).

## Summary

Recommendation:
1. Answer convergence per proxy pod and per subscription (not per type).
2. Answers are `yes`, `no` or `unknown`, and each names the side that proves it.
3. Build no generic xDS diff.

Today's data answers less than it seems.
- **Proxy side.** It proves what a subscription last accepted, that rejections happened, and whether the proxy is connected. It cannot say which version a rejection was for, or whether the accepted version is the current intent.
- **Controller side.** It proves what was published per key. It cannot say which pod holds which key, or which pod rejected.

The upstream asks U7–U9 close those gaps.

xdsview's share is one vendor-neutral finding: an active HTTP connection manager whose RDS route configuration is absent from a complete dump. S5 shows that such a listener is `Programmed=True` and serves 404.

The join of controller and proxy evidence, and the verdict, belong to kgwctl. Snellius can report observed version differences between replicas, but not classify them.

## 1. Evidence

### 1.1 Starting evidence, re-verified

| # | Program claim | Verdict | Evidence |
|---|---|---|---|
| 1 | No per-proxy ACK record; NACK metrics have no pod label; the active gauge clears on stream close or an error-free request | Confirmed | `setup/cache_watch_policy.go:99-124,171-214,332-340`; `setup/envoy_error.go:30-41,129-150`. An error-free request is not an ACK: a new subscription produces one too (§1.2 S5) |
| 2 | Versions are per-type opaque hashes, sometimes composed, published per type and not atomically. The key includes the node | Confirmed | Versions: `proxy_syncer.go:167`, `perclient.go:73,138,164,327`. Install order SDS, CDS, EDS, LDS, RDS, stated as non-atomic (`setup/controlplane.go:189-218` at `v2.5.2`). Key `role~HashLabels(augmented pod labels)~ns` (`ir/model.go:112-129`, `krtcollections/pods.go:286-339`). `pod-template-hash` also splits keys per ReplicaSet. `DISABLE_POD_LOCALITY_XDS=true` gives one key per Gateway (`uniqueclients.go:113-116`) |
| 3 | The controller can hold back a newer translation | Confirmed | See the table below. Sources: `publish_gate.go:20-80`; defaults at `v2.5.2` `api/settings/settings.go:448,469` |
| 4 | `Programmed=True` is a translation result | Confirmed | Default Gateway and listener conditions at `v2.5.2` `pkg/reports/status.go:258,284`. The NACK path writes only metrics and a log. Live: listener `http2` is `Programmed=True` while serving 404 (S5). kgateway#14453 is open |
| 5 | Envoy `version_text`, `update_success`, `client_status`, `error_state`, `version_info` | Confirmed, with corrections | `version_text` is a text readout, not a gauge (`subscription.h:332-341`). The `version` gauge (xxHash64) and `update_time` change only on an accepted update (`grpc_subscription_impl.cc:121-135`). A rejection increments `update_rejected` without recording the rejected version. `client_status` is never set. Only LDS sets `error_state`. The EDS dump has no `version_info` |

How the controller holds back a newer translation:

| Mechanism | Trigger | Default duration |
|---|---|---|
| First-publish withhold | referenced clusters not yet ready | 15 s budget |
| Warm-client withhold | a client reconnects holding an older version | up to 10 min after the budget |
| Flip hold | a route moves to a new cluster that isn't ready | one budget |
| NACK parking | the proxy rejects an update on an open stream | the rejected version is resent with 1–60 s backoff |

### 1.2 Captures (identity-checked)

| Scenario | Observed | What it shows |
|---|---|---|
| S1 steady state | Two keys (`5475…`, `8735…`), one per worker, with equal versions per type. Each pod's subscriptions accept the published version: CDS `11484…`, LDS `4868…`, RDS `listener~8080` `9175…`, EDS `gw.demo` `12643…` | Equal content gives equal versions across keys. A match is an acceptance observation, not a controller-recorded ACK |
| S2 RDS NACK (route regex `[invalid-regex`), captured twice 36 s apart | Published RDS `11812…`. Both pods stay on `9175…`. `update_rejected` goes 6 → 7 on one pod and stays 6 on the other. `rejects_active=2`, gateway labels only. Route, Gateway and listener are all `Programmed=True`. The log says `"xds error" … missing ]: [invalid-regex` | A mismatch plus past rejections, but no record of which version was rejected. Counters can stand still for a whole window because of the backoff |
| S4 controller scaled to 0 | `wzvn6` reports `connected_state 0`. `cds` and `lds` `update_failure` go 0 → 17; `update_time` unchanged. The second pod's capture timed out under load | `update_failure` counts connection failures, so its growth is not a NACK signal. `update_time` is the age of the last accepted update, not a bound on how long the proxy has been stale |
| S5 new listener 8081 during the NACK | Both pods: `listener~8081` is active, its RDS subscription has accepted nothing (`version_text ""`, 6 rejections), and `RoutesConfigDump` has only `listener~8080`. `curl :8081` → 404 while `:8080` → 200. Listener `http2` is `Programmed=True` | Accepted is not serving. The listener is live with no routes, and nothing in Envoy or the status flags it |
| S5b the gauge during S5, sampled every second | `rejects_active` stays 2; `rejects_total` goes 2 → 4 within one second | The new subscription started new rejection episodes. A zero reading was not observed and may not be scrapeable |
| S6 `q8vvd`'s node frozen, route fixed; captured 30 s and 6 min later | The controller keeps both keys and publishes RDS `9421…`. `rejects_active=1` stays for 6 min. `q8vvd` is unreachable: reported missing, not substituted. `wzvn6` accepts `9421…` | The controller cannot tell a frozen proxy from a slow one, nor say which pod holds the reject. An unreachable pod's answer is `unknown` |
| S7 unpaused | Within 5 s `q8vvd` accepts `9421…` (`update_time` 15:10:19 vs `wzvn6` 14:57:54) and the gauge is 0 | Whether the stream survived the pause or reconnected is not established |

Warming (Q5) was not reproduced. The evidence for it is Envoy source (Phase 7 assessment §2, and review #7).

Access findings:
- The proxy stats listener on port 9091 serves `/ready`, `/metrics` (`/stats/prometheus?usedonly`) and `/stats*`. It does not serve `/config_dump`.
- `version_text` is readable there through `/stats` or `?text_readouts`.
- Controller `/snapshots/xds` is localhost-bound on 9095 and can contain inline credentials, because redaction only covers SDS resources (`admin/xds_snapshot.go:58`).

## 2. The questions

| Q | Operator scenario | What can be answered today | Proved by | `unknown` when | Missing |
|---|---|---|---|---|---|
| 1 | "I changed a route. Has every proxy got it?" | Per pod and per subscription (RDS per route configuration, EDS per cluster, CDS and LDS per type): accepted version = published version | proxy (accepted) and controller (published) | pod not reached; pod-to-key not proven, unless all keys for the Gateway agree; published version changed between two captures (publish race, non-atomic install); a withhold is active; a subscription is absent from the published snapshot or vice versa | pod key (U9); controller-recorded ACK (U7) |
| 2 | "Do my replicas disagree?" | Observed per-subscription difference between pods, persisting across two captures | proxy | a replica not sampled. Whether the difference is *unexpected* needs controller evidence (published versions per key differ legitimately, e.g. locality-dependent EDS) | as Q1 |
| 3 | "Is a rejection still in force?" | Two separate facts: (a) the subscription rejected at least once since its last accepted update; (b) accepted ≠ published. Whether the current published version is the rejected one stays `unknown` | proxy (a), both (b) | always, for "current version rejected", without correlated stream events | per-stream last sent, ACKed and NACKed version with time and error, per pod (U7) |
| 4 | "Is a proxy disconnected or stale, and since when?" | `connected_state` at capture time. Stale: as Q1. "Since when": a last-good and first-bad pair of observations, never `update_time` alone | proxy | single capture (onset) | connection events (U7). Onset of staleness also needs publication history |
| 5 | "Is something stuck warming or draining, or missing?" | Three outcomes. Warming listeners and clusters, by name, from `/config_dump`. Draining listeners, by name. An active HCM whose RDS configuration is absent (S5). "Stuck" needs the same occurrence in repeated captures | proxy | a dump that is incomplete or fails to parse; draining clusters, because the cluster dump does not list them | nothing upstream |
| 6 | "What does Snellius already answer?" | See §3 | — | — | — |

## 3. Snellius coverage (question 6)

| Check | Evidence it uses | Q1 | Q2 | Q3 | Q4 | Q5 |
|---|---|---|---|---|---|---|
| 001 `xds-rejected` | controller `rejects_active` per Gateway, with per-proxy `update_rejected` deltas and derived logs as evidence | — | — | partial: per Gateway, not which pod or version | — | — |
| 006 `proxy-not-ready` | Deployment readiness | — | — | — | context | — |
| 008 `signal-unavailable` | missing or stale inputs | — | — | — | — | — (supports `unknown`) |
| 024 `xds-config-withheld` | controller withheld counts per Gateway | partial | — | — | partial | — |
| 025 `proxy-config-warming` | warming counts on Ready pods | — | — | — | — | partial: counts only, no names, no draining, no missing RDS |
| 026 `proxy-disconnected` | `connected_state` on Ready pods | — | — | — | partial: no staleness; "since" is when the UI first saw it | — |

Snellius reads no versions: its stats filter excludes `version_text` and its parser accepts integers (`pkg/proxystats/parse.go:11`). By design it never reads Envoy admin, `/config_dump` or controller port 9095.

One finding for Snellius, to be raised through the program:
- The "Config accepted" wording rests on no 001 and no 024 error. It checks unavailable inputs, but not 025, 026 or versions, so a disconnected, warming or stale proxy can still read as accepted. S5 adds a case no Snellius input can see: a `Programmed=True` listener serving 404.

## 4. Recommendation

### 4.1 Who provides what (E14)

| Piece | Provider | Notes |
|---|---|---|
| Active HCM with an absent RDS route configuration, from a complete dump; `unknown` if `RoutesConfigDump` is missing or fails to parse | xdsview core | Vendor-neutral Envoy config. The only core change this investigation supports (D-011). It fits Phase 9's dump-coverage work |
| Warming and draining listeners, warming clusters, by name | xdsview core (largely exists: listener state since Phase 6) | Draining clusters are not namable |
| Join controller and proxy evidence; per-subscription verdict with `unknown` reasons; two-capture windows | kgwctl | A live diagnostic that extracts only versions per key and type from `/snapshots/xds`, never storing the snapshot. The Phase 4 bundle boundary (no controller admin) stays |
| Offline ceiling | kgwctl bundles | Proxy versions, lifecycle and connection signals, plus aggregate controller metrics. A bundle cannot answer Q1, and cannot confirm Q3 for the current version |
| Observed version differences across replicas (Q2), proxy-only | Snellius, if it wants it | Needs `version_text` (or the `*_version` gauge) in its scrape. It reports a difference, not drift |
| Per-pod ACK/NACK record, pod key, NACK in status | kgateway upstream | U7, U9, U8 |

xdsview adds no stats parser. Snellius has its own, and no second consumer is named. The kgateway snapshot reader stays out of xdsview's kgateway module, which is about provenance (D-019).

### 4.2 Upstream asks

| Ask | Refined by this investigation |
|---|---|
| U7 | Per stream and type, tied to pod identity and controller replica: last sent version and nonce, last ACKed version with time, last NACKed version with time and error, connect time. Exposed on 9095 or via CSDS. A `pod` label on `rejects_*`. An error-free request from a new subscription must stay distinguishable from an ACK (S5, S5b) |
| U8 | Unchanged: kgateway#14453 (open). S5 adds a case for it: a NACK on RDS leaves a listener `Programmed=True` that serves no routes |
| U9 | Confirmed: a proxy cannot learn its key, since Envoy shows only the bootstrap role. The key must be returned for the connection the pod holds. Alternative: `DISABLE_POD_LOCALITY_XDS` where one key per Gateway is acceptable |

### 4.3 Not recommended

- A generic diff of `/snapshots/xds` against `/config_dump` (Phase 7 assessment §4A). No scenario here needs one.
- `update_failure` growth as a rejection signal: it counts connection failures (S4).
- `update_time` as a staleness or disconnect onset (S4).
- Treating a version mismatch plus rising `update_rejected` as "the current version is rejected" (S2: one pod's counter didn't move for 36 s).

## 5. Open questions for the owner

1. Should the missing-RDS finding (§4.1, first row) go into the Phase 9 design?
2. Should the kgwctl piece (§4.1, live join) be proposed to kgwctl as a `cross-repo` issue? (Asking before filing.)
3. Should the Snellius finding in §3 go to Snellius as a `cross-repo` issue? (Asking before filing.)
4. Should the U7–U9 refinements in §4.2 be sent to the program session, which owns the upstream asks table?
5. Should the key mirror (M-3) be used to map pods to keys before U9? It needs owner approval under the program's mirror rules. This investigation did not use it.
