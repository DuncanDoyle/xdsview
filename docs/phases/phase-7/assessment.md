# Phase 7 assessment — intended vs accepted configuration

**Date:** 2026-11-22
**Question:** Should xdsview diff kgateway's controller snapshot (`/snapshots/xds`) against Envoy's `/config_dump`?
**Recommendation:** C, defer. No xdsview phase until a concrete operational use case names evidence that only xdsview can provide. Convergence diagnostics (B) are a program-level investigation. Most of their evidence lives outside this library. Decision: D-011.

Sources: kgateway `v2.5.1` (`16bc081e`, matches upstream), Envoy `v1.39.6` (`aacd0340`), Snellius `d880f13`, kgwctl `38acaa0`. Paths below are at those commits.

## 1. What the controller exposes

| Question | Finding | Evidence |
|---|---|---|
| What `/snapshots/xds` returns | Every cache key mapped to its full go-control-plane `Snapshot`: listeners, routes, clusters, endpoints, secrets (secrets reduced to names). Encoded with Go `encoding/json`, not protojson: `Any` and field names don't match `config_dump`. | `pkg/kgateway/admin/server.go:47`, `admin/xds_snapshot.go:27-94` |
| Access | Port 9095, bound to localhost, plain HTTP, no auth. Port-forward or exec only. | `wellknown/ports.go:18`, `admin/server.go:29-31,95-99` |
| Snapshot key | Node `metadata.role`, rewritten to `role~hash(podLabels)~namespace`. Replicas with equal pod labels share one snapshot. A rollout (`pod-template-hash`) splits them into separate keys. | `xds/utils.go:71-80`, `pluginsdk/ir/model.go:83-100` |
| Version strings | Content hashes per resource, XOR-folded per type. Carried-over resources get `<v>-carry-<hash>`. | `proxy_syncer/proxy_syncer.go:152-167`, `kube_gw_translator_syncer.go:315,321` |
| Publish order | Per type, SDS → CDS → EDS → LDS → RDS. Not atomic across types. | `setup/controlplane.go:185-218` |
| ACK/NACK state | A NACK is logged and counted: `kgateway_envoy_xds_rejects_total`, `kgateway_envoy_xds_rejects_active` (gateway, type_url). Repeated NACKs are parked and resent with backoff. **No record of the version each proxy ACKed, and no endpoint listing connected proxies.** | `setup/envoy_error.go:29-41,109-151`, `setup/cache_watch_policy.go:41-120` |
| Gateway status | No condition reflects a NACK. | grep of NACK/ErrorDetail at `v2.5.1` |

## 2. What Envoy can prove

| Question | Finding | Evidence |
|---|---|---|
| Accepted versions | `version_info` and `last_updated` per listener state, route config, cluster, secret. This is the *accepted* version only. | `listener_manager_impl.cc:79-81`, `route_config_provider_manager.cc:48-52`, `cluster_manager_impl.cc:1734-1763` |
| Rejected updates | Only LDS records one (`error_state`: details, failed config, attempt time; cleared on the next LDS update). CDS, RDS, SRDS, EDS, SDS record none. `UpdateFailureState.version_info` is not implemented. `client_status` (ACKED/NACKED/DOES_NOT_EXIST) is never set. | `listener_manager_impl.cc:493-574`, `lds_api.cc:60,80-89`, `config_dump_shared.proto:21-68`; `git grep client_status aacd0340 -- source/` finds nothing |
| Nonce, received version | Only in the outgoing gRPC request; not in `config_dump`. | `grpc_mux_impl.cc:391-459` |
| Stats | Per subscription: `update_attempt`, `update_success`, `update_rejected`, `update_failure`, `init_fetch_timeout`; `version` (hash gauge), `version_text`, `update_time` set on success only. `control_plane.connected_state`, `pending_requests`, `identifier`. No stat separates a current NACK from a past one. | `envoy/config/subscription.h:242-251`, `grpc_subscription_impl.cc:82-137`, `envoy/config/grpc_mux.h:26-29` |
| Warming | A listener waits for its RDS/SDS/ECDS first fetch; a cluster for its first init. Each bounded by `initial_fetch_timeout` (default 15s; 0 waits forever). | `listener_impl.cc:358-383`, `config_source.proto:245-247` |
| Endpoints | Not in `config_dump` unless `?include_eds`. | `config_dump_handler.cc:126,247-253` |

**`config_dump` cannot prove:** that the accepted version is the controller's latest; that a resource was never sent; a NACK outside LDS; which version was rejected; whether a replica is stale. Zero NACKs doesn't prove convergence either: a disconnected proxy, a parked watch, or a non-atomic publish all show no NACK.

## 3. What already covers convergence

Snellius (and kgwctl `check`, which runs Snellius's checks on `v0.8.2`):

| Check | Signal | Evidence |
|---|---|---|
| KGWUI-001 xDS rejects | `kgateway_envoy_xds_rejects_active`/`_total`, per-pod proxy `update_rejected`, controller logs | `pkg/health/xds.go:59-71,272-311`, `ctrlmetrics/parse.go:36-40` |
| KGWUI-024 config withheld | `kgateway_xds_snapshot_perclient_*`, `deferred_clients` | `pkg/health/withheld.go:20-22,151-156` |
| KGWUI-025 proxy config warming | proxy `warming_clusters`, `total_listeners_warming` held 60s | `pkg/health/proxystats.go:17,44-46,162-169` |
| KGWUI-026 proxy disconnected | proxy `control_plane.connected_state` held 60s | same |
| KGWUI-006 proxy not ready | Deployment readiness | `pkg/health/checks.go:693-712` |

Not covered today: per-replica version drift (`version_text` per pod across a Gateway's replicas), and a rejected update outside LDS whose counter has stopped moving. kgwctl's roadmap proposes an `update_failure` growth check (`kgwctl docs/roadmap.md:33`).

## 4. Options

**A. Keep Phase 7 (full intended-vs-accepted diff).** Rejected.
1. In a converged system the diff is empty by construction: Envoy accepted what was sent. Every non-empty diff is a convergence state (NACK, in flight, parked, disconnected, rollout split), which the signals above already report more directly.
2. It doesn't catch the case Snellius D-006 cites, a wrong translation that Envoy accepts: then intended and accepted agree.
3. The cost is high: a second encoding (Go JSON vs protojson), per-client scoping, subscribed-names-only delivery, EDS and secrets excluded, non-atomic publish, `-carry-` versions. All need normalising before a diff means anything.
4. It reads kgateway's admin API, which is kgateway-specific, unauthenticated and localhost-bound. That is not a vendor-neutral Envoy library's input.

**B. Replace it with convergence and replica-consistency diagnostics in xdsview.** Not as an xdsview phase. Most inputs are controller metrics and per-pod stats that Snellius already scrapes. The controller has no per-proxy ACK record to correlate with. What is genuinely Envoy-side and fits here is small. If the investigation needs it, these can be stabilization-sized additions:
- the accepted `version_info` per resource in the model (today only `Listener.State.LastUpdated` is kept);
- a comparison of two snapshots of the same Gateway's replicas by resource and version, which reports lifecycle states (warming, draining, error) as such, not as drift.

**C. Defer until a concrete use case and enough evidence exist.** Recommended. The program session runs the evidence-driven convergence investigation (ACK/NACK, staleness, warming and draining, replica drift, version correlation). xdsview takes a phase only when that investigation names a question that needs Envoy-config evidence, with an operator scenario.

## 5. Consequences

- Roadmap Phase 7 becomes deferred; Phase 8 (vendor-neutral core) is next.
- Snellius D-006 and its Phase 12 text ("intended configuration compared with the configuration Envoy accepted", `snellius docs/roadmap.md:185-192`) need the program's decision; E4's "+7 intended vs accepted" no longer comes from xdsview.
- kgwctl's deferred `proxy diff` (`kgwctl docs/roadmap.md:88`) has no provider.
- Retained for later: `WeightedCluster.TypedPerFilterConfig` (D-009) stays; `chain` uses it for `Conditional`.
