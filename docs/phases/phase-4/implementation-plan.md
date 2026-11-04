# Phase 4 — Library Extraction: Implementation Plan

**Design:** `docs/phases/phase-4/design.md`
**Base:** `e1c5666`

The work is split into three **planned** sub-phases. Each one is a reviewable change on its own and leaves `go test ./...` and `go vet ./...` green.

| Sub-phase | Scope | Depends on |
|---|---|---|
| 4.1 | Library move and API, import boundary, client-go bump, LICENSE | — |
| 4.2 | `-o json` and golden contract files | 4.1 |
| 4.3 | Test corpus: OSS recordings on v2.4.7 and v2.5.0, enterprise corpus moved | 4.1 (4.2 golden files regenerate at the end) |

Phase close: cross-model phase review, tag `v0.1.0`, update the program file, send cross-repo notices.

---

## 4.1 — Library move and API

1. **Capture the baseline.** Build the base binary and write `krp dump --file <dump>` text output for every scenario to a scratch directory. Do the same with `--route/--route-ns` for the scenarios the filter e2e tests use. Afterwards, the same commands on the new binary must produce byte-identical output.
2. **Move the packages with `git mv`.** `internal/model` → `pkg/envoy/model`, `internal/parser` → `pkg/envoy/parser`, `internal/filter` → `pkg/envoy/filter`. Update imports and the package docs (`internal/model` mentions).
3. **Parser API.** `Parse(io.Reader) (Result, error)`, and rename `ParseResult` to `Result`. `Parse` uses `io.ReadAll` followed by `json.Unmarshal`, so trailing data stays an error. Tests read their fixtures through `os.Open`. Add tests for a reader that errors mid-stream and for trailing data after the document. Write the ownership contract (design § Ownership) into the `parser` and `filter` package docs.
4. **Filter API.** Rename `FilterOptions` to `Options` and add `const AllRules = -1`. The CLI's `--rule` default uses `filter.AllRules`.
5. **Source package.** `pkg/envoy/source` gets the `Source` interface plus `File` and `Admin`. `Admin` uses `http.NewRequestWithContext`, so it honours ctx, and the reader it returns is the response body. Move `client_test.go`'s httptest cases here and add a cancelled-context case.
6. **Kube source.** `internal/envoy/portforward.go` → `pkg/envoy/source/kube`. `Gateway` and `Deployment` return a `source.Source`. `ConfigDump` opens the tunnel and returns a reader whose `Close` closes both the body and the tunnel. `Options.RESTConfig` takes precedence over `KubeContext`. Every error after the tunnel opens closes the tunnel before `ConfigDump` returns: request failure, non-200, and cancelled ctx. Keep the pod-finder tests. Add tests through a fake forwarder seam: closing the reader stops the tunnel, and each of the three failure paths stops it too.
7. **CLI wiring.** `cmd/krp` picks a `source.Source` from the flags, then runs `ConfigDump`, `parser.Parse`, `filter.Filter` and the renderer. Flag validation and messages don't change. Delete `internal/envoy`.
8. **Import-boundary test.** `pkg/envoy/boundary_test.go` runs `go list -deps` on `model`, `parser`, `filter` and `source`. It fails if any dependency is outside the standard library and `github.com/DuncanDoyle/krp/pkg/envoy/...`.
9. **Dependencies.** `go get k8s.io/client-go@v0.36.2 k8s.io/api@v0.36.2 k8s.io/apimachinery@v0.36.2`, set `go 1.26.0`, then `go mod tidy`.
10. **LICENSE.** Add the Apache-2.0 text, with copyright "The krp Authors". Link it from the README.
11. **External consumer check.** In a scratch module outside the repo, with `replace github.com/DuncanDoyle/krp => <repo>`, import `parser`, `filter` and `source` and parse one dump. It has to build and run. The check isn't committed; its output goes in the sub-phase record.
12. **Verify.** `go test ./...`, `go vet ./...`, the step-1 byte comparison, and the README architecture section updated.

## 4.2 — `-o json`

1. Add the `-o/--output` flag with values `text` (default) and `json`. Reject unknown values, and reject `json` together with `--interactive`.
2. Put the JSON envelope type and its encoder in `cmd/krp`. The envelope is CLI output, not a library type, so the library's contract stays the model. `schemaVersion: "v1alpha1"`, and `warnings` is always an array.
3. Add a golden test in `cmd/krp`. For every dump under `testdata/`, run the dump command in-process with `-o json`, then compare against `testdata/golden/<corpus>/<scenario>[/<version>].json`. An `-update` flag rewrites the golden files. Add one filtered case with `--route` and one with `--rule`.
4. Add a test that rejects `-o json --interactive` and `-o yaml`.
5. Update the README usage, and the `cmd/krp` doc comment.

## 4.3 — Test corpus

1. **Move the enterprise corpus.** Run `git mv` on all current `testdata/scenarios/*/envoy/config_dump.json` and the 02_1/02_6/02_7/02_8 `k8s/` manifests into `testdata/enterprise/scenarios/<s>/`. Add `testdata/enterprise/README.md`, which says the dumps came from the enterprise distribution and are removed before the move into kgateway (E6).
2. **Parameterise the scripts.** The scenario `setup.sh` and `teardown.sh`, and the 02_1 TLS secret helpers, take a kubeconfig path and a context. They pass `--kubeconfig` and `--context` to every `kubectl` call. `teardown.sh` deletes everything `setup.sh` creates, including policies. Fix the README collection snippets the same way (the 00-simple snippet tears down by calling `setup.sh`).
3. **Rewrite five scenarios for OSS.** 02_1 is a transformation `TrafficPolicy` on HTTPS, 02_6 a CORS `TrafficPolicy`, 02_7 ext auth (`GatewayExtension` plus a dummy Service, and a `TrafficPolicy`), and 02_8 global rate limit (`GatewayExtension` plus a dummy Service, and a `TrafficPolicy`). 06 gets new manifests: two ext_proc `GatewayExtension`s with dummy Services, and the `TrafficPolicy`s that use them. If OSS can't express part of a scenario, write it down in that scenario's README.
4. **Recording script.** `hack/record-dumps.sh <kgateway-version>` does the following:
   1. Creates a kind cluster with `--kubeconfig` set to a temp file.
   2. Installs the Gateway API CRDs for the minor that version is built against, read from that tag's `go.mod`.
   3. Installs the kgateway CRDs and kgateway with Helm (`--kube-context`).
   4. For each scenario that has `k8s/setup.sh`, in isolation:
      1. Run setup.
      2. Wait for the HTTPRoute and its policies to report `Accepted`.
      3. Poll `/config_dump` until it holds a route for the scenario's HTTPRoute and two reads in a row are equal.
      4. Write `envoy/kgateway-<version>/config_dump.json`.
      5. Run teardown and wait for the scenario's namespaces and policies to be gone.
   5. Deletes the cluster.

   Every `kubectl`/`helm` call gets `--kubeconfig <temp file>` and `--context`/`--kube-context`.

   It also records Envoy's `/server_info` version next to each dump.
5. **Record.** Run the script for `v2.4.7` and `v2.5.0`. Scenarios 03, 04 and 05 get recorded if their setup works; otherwise they stay without dumps (#17 is Phase 5).
6. **Update the tests.** The parser and filter e2e tests iterate over every dump in both corpora. Each scenario gets a table of expected Envoy route names, derived from its manifests (rules × matches × backends). The filter tests assert:
   1. The exact set of routes kept for the scenario's HTTPRoute.
   2. The exact subset kept for each `--rule`.
   3. That a route of another HTTPRoute is dropped, in scenarios that have more than one.

   A naming-convention break on a version is a test failure, not a skip.
7. **Regenerate the golden files** from 4.2 for the new corpus, and review the diff.
8. **Update the roadmap Test Scenarios table**, with the corpus and version columns.

## Phase close

1. Run a cross-model phase review on `git diff e1c5666..HEAD` → `reviews/phase-codex.md`, then triage it.
2. Write `phase-4-complete.md`, and update the roadmap and `phase-status.md`.
3. Propose the `v0.1.0` tag to the owner, and push it once approved.
4. In the program file: the contracts row becomes delivered `v0.1.0`, and E1, E4 and E6 (license) are updated.
5. File `cross-repo` issues in kgwctl (E1) and kgateway-ui (E4) that name the edge and the version. Ask the owner before filing.
