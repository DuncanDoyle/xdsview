1. **P1 correctness — Hook release makes rejected-upgrade connections leak permanently.** [portforward.go:164](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/source/kube/portforward.go:164). A keep-alive `403` with an empty body makes `NewConnection` fail. The dependency closes the response body, but does not close its raw `s.conn`; `ForwardPorts` installs its connection-close defer only after a successful dial. `release()` then unregisters the remaining close-on-cancel hook without closing the connection. Previously, cancelling the parent context could reclaim it; now it cannot. Release must follow actual connection closure.

2. **P3 test gap — Cancellation test does not force the corrected error branch.** [portforward_test.go:199](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/source/kube/portforward_test.go:199). Cancelling makes `ctx.Done()` ready before the forwarder necessarily publishes its error. The existing test can therefore pass without the new `ctx.Err()` check. The supplied race runs do not establish coverage of that selection.

3. **P3 test gap — Hook test bypasses production cleanup.** [portforward_test.go:271](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/source/kube/portforward_test.go:271). The test calls `hooks.release()` directly. Removing every production `release()` call would leave it passing. It verifies the helper’s effect, but neither successful reader-close integration nor error-path connection cleanup.

Accepted-finding assessment:

| Original # | Does the fix resolve it? | Would the regression test fail without the fix? |
|---|---|---|
| **1 — TLS verification** | Yes, for the reported direct HTTPS/system-roots case. | **Yes.** The supplied before/after result confirms it. |
| **2 — Cancellation error identity** | Yes: the `errCh` branch checks `ctx.Err()` before returning the transport error. | **Not reliably.** No new test forces that branch; see finding 2. |
| **3 — Dynamic composite delegate** | Yes. An action without a static type permanently marks the classification mixed. | **Yes.** The added static-plus-dynamic case would previously report ExternalProcessor. |
| **5 — Metadata-backed filtering** | Yes for the accepted metadata scope. `r.Origin` and replacement mappers remain unsupported, as triaged. | **Yes.** The custom route names cannot satisfy the previous name-only selection. |
| **6 — Legacy metadata indices** | Yes. Both indices are decoded independently of the Envoy route name. | **Yes, the v2.4 case.** The v2.5 cases already worked. |
| **7 — Ambiguity contract** | Yes, the test now asserts certainty and complete candidate sets. | **No.** It passes the previous implementation; this closes a coverage gap. |
| **8 — Blocking timeout diagnostic** | Yes, the blocking channel read is removed. | **No new regression test.** The normal cancellation path would pass before; only the timeout path exposed the hang. |
| **9 — Retained cancellation callbacks** | Yes for successful tunnel cleanup, but error cleanup introduces finding 1. | **Helper-level only.** A no-op release would fail; missing production release calls would not. |

Two checked concerns do **not** establish additional defects:

- The empty TLS config does not force TLS for plain-HTTP hosts: the dependency’s `http` branch uses the raw dialer.
- Mixed snapshots choose metadata or name matching per route. Name-only rows retain the documented dash ambiguity; the added test lacks mixed-snapshot coverage, but that ambiguity predates this diff.

Static review only. Author-supplied test results were not rerun. No files edited.
## Triage

| # | Finding | Source | Decision | Note |
|---|---|---|---|---|
| 1 | Releasing hooks leaks a rejected upgrade's connection | reviewer | accept | Verified: `TestConfigDump_RejectedUpgradeClosesConnection` (keep-alive 403) failed. `release` now closes the tunnel's connections as well as unregistering the hooks. |
| 2 | No test forces the dial-error branch of the cancel fix | reviewer | accept | `TestConfigDump_CancelledDialReportsCanceled` uses a context whose `Err` is set but whose `Done` never fires, so only the dial-error branch runs. It fails without the `ctx.Err()` check. |
| 3 | Hook test bypasses production cleanup | reviewer | accept, partly | The rejected-upgrade test covers the production error path end to end. The success path (reader close) is covered by the live kind check, not a unit test: that needs a real SPDY server. |
