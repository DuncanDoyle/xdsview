# Phase 5.1 — Cancellation (Complete)

**Completed:** 2026-11-04
**Range:** 3e7d671..a0c7cc9 (branch `duncandoyle/phase5`, pending design approval)

## Summary
Cancelling `kube.Gateway(...).ConfigDump(ctx)` while the port-forward is still connecting now stops the connection attempt. The call returns `context.Canceled`, the forwarder goroutine has exited, and the API server sees the connection close. Long-running consumers such as the kgateway-ui backend no longer leak a goroutine and a connection per cancelled call, unless an HTTP or SOCKS proxy sits in front of the API server (Q10). The phase review found and fixed three follow-ups: TLS verification, the cancel error and hook release (`reviews/phase-codex.md` #1, #2, #9).

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Cancel during an unanswered upgrade | `TestConfigDump_CancelDuringUpgrade`. Before the fix it failed ("forwarder goroutine still running when ConfigDump returned", "API server still holds the connection 1s after ConfigDump returned"). After the fix it passed 5 of 5 runs; `-race -count=3` is clean. |
| Reader close, request error, non-200, cancel after open | Phase 4 tests unchanged and passing |
| Real cluster | kind + kgateway v2.5.0 + 00-simple: 3 runs of `krp dump --gateway gw -n kgateway-system -o json` return the expected listener and route. Temp kubeconfig; the cluster was deleted afterwards. |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean; import-boundary test ok |

The changes are all in `pkg/envoy/source/kube/portforward.go`:

1. **`ctxDialer`** is client-go's dialer with the upgrade request bound to ctx.
2. **`newDialer`** uses an upgrade transport whose `DialContext` closes the connection when ctx is done. It only does so when no proxy applies.
3. **`portForward`** waits up to 1s for the forwarder to exit after a cancel.

## Notes for later phases
- **Why a transport hook and not just a context.** Even with a context on the request, client-go reads the upgrade response with `http.ReadResponse` and no deadline. Only closing the connection unblocks it. client-go's only hook for supplying the connection is `RoundTripperConfig.UpgradeTransport`, which excludes `TLS` and `Proxier`.
- **Known limit with a proxy.** When an HTTP or SOCKS proxy applies to the API server URL, krp keeps client-go's own dial path. The connect and TLS handshake honour ctx, but the response read doesn't. There's no hook to close that connection without reimplementing client-go's proxy dialing.
- `forwardExited` is a package-level test hook, the only one in the package.

## Deferred / open
- #25 to be closed when this branch reaches `main`.
