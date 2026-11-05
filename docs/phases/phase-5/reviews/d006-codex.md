- **P1 — Optional listing can block all output.** [cmd/krp/main.go:220](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/cmd/krp/main.go:220) calls the listing with `context.Background()`; neither `HTTPRoutes` nor its REST-config builder sets a request timeout. A stalled API response prevents rendering an already-fetched dump, including with `--route`. D-006’s warn-and-continue fallback only runs once the request returns. Use a bounded context derived from `cmd.Context()`.

- **P2 — Live `--route` no longer necessarily settles the origin as documented.** [cmd/krp/main.go:97](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/cmd/krp/main.go:97) appends the requested ref to the entire cluster list. If both `b-c/a` and `c/a-b` exist, `--route a-b --route-ns c` retains both readings of `httproute-a-b-c-0-0`; the mapper returns `Ambiguous`, whereas previously the explicit ref resolved it. README:95–98 promises these inputs settle ambiguity. Define the combined behavior and align implementation/documentation. README:129 also overstates failure behavior: metadata, explicit refs, and unambiguous names can still yield origins without listing permission.

- **P3 — Error and CLI integration coverage bypass important boundaries.** [pkg/envoy/source/kube/httproutes_test.go:27](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/source/kube/httproutes_test.go:27) tests only successful listing. [cmd/krp/main_test.go:290](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/cmd/krp/main_test.go:290) replaces the whole listing function with a generic error, so it cannot catch swallowed API errors or broken context propagation. No live tests cover `--deployment`, forwarded `--context`, or listing combined with `--route`. Add Forbidden/NotFound reactors, a bounded cancellation case, and combined-mode assertions.

The cluster-wide `v1` listing produces the full refs expected by `Known`. Returned Forbidden and missing-CRD errors follow the warning-and-continue path. The stdlib-only core boundary remains intact; Kubernetes imports stay under `source/kube`.

No tests run or files edited. The author-reported Known-removal failure establishes that the positive CLI test detects missing mapper wiring; it does not cover the gaps above.
## Triage

| # | Finding | Source | Decision | Note |
|---|---|---|---|---|
| 1 | The optional listing has no timeout and can block output | reviewer | accept | 10s bound from the command's context. The stalled-listing test hangs without it (verified with `-timeout 15s`). |
| 2 | `--route` plus the cluster list can bring back ambiguity | reviewer | accept | With `--route`, the named HTTPRoute alone is `Known` and nothing is listed (`TestDumpLive_RouteSkipsListing`, fails without the skip). README wording corrected. |
| 3 | Error paths and `--deployment` untested | reviewer | accept | Forbidden reactor test for `listHTTPRoutes`; stalled-listing case; `--deployment` in the `--route` test. Forwarded `--context` is covered by the live kind check only. |
