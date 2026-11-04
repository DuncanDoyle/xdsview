# Sub-phase 5.1 — Cancellation (#25)

Kind: planned
Base: d1dfc47
Issues: #25 | Plan steps: implementation-plan.md § 5.1

## Problem / goal
Suppose `kube.Gateway(...).ConfigDump(ctx)` is cancelled while the port-forward is still connecting, for example against an API server that accepts the connection but never answers the SPDY upgrade. The call returns, but the forwarder goroutine and its connection stay alive until the server answers or TCP gives up.

## Expected behaviour

| Input / action | Expected |
|---|---|
| Cancel ctx while the API server holds the upgrade unanswered | `ConfigDump` returns `context.Canceled` within 2s of the cancel. By then the forwarder goroutine has exited, and the server sees the connection close (allowed 1s for the close to reach it). |
| Successful forward, reader closed | Unchanged: the tunnel stops once. |
| Request error, non-200 response, ctx cancelled after the tunnel is up | Unchanged: the tunnel stops before `ConfigDump` returns. |

## Root cause (if known)
The SPDY dialer builds its upgrade request without a context, and its `http.Client` has no timeout (`pkg/envoy/source/kube/portforward.go:137`). Even with a context, client-go reads the upgrade response with no deadline or cancel hook. On cancel, `portForward` closes `stopCh` and returns. `ForwardPorts` only checks `stopCh` after the dial.

## Acceptance tests
`TestConfigDump_CancelDuringUpgrade` in `pkg/envoy/source/kube`. It uses a local TLS API server whose upgrade handler never answers, runs the real `portForward` path with a stub pod finder, and cancels after the server has received the request.

## Scope
In: `pkg/envoy/source/kube`.
Out: other sources, the CLI, and fixing the same gap when an HTTP or SOCKS proxy is configured. That path keeps client-go's dialer, so the connect and TLS handshake honour ctx, but the response read doesn't. That's recorded as a known limit.

## Plan / design impact
None.

## Definition of done
- [ ] acceptance test fails before, passes after (report both)
- [ ] full test suite passes
- [ ] cross-model review at the phase review (planned sub-phase)
- [ ] phase-5.1-complete.md written; phase-status.md updated; #25 closed with root cause + commit
