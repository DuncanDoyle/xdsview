# Sub-phase 4.2 — `-o json`

Kind: planned
Base: 59cac51b1770fdcb4a66dcad13ef5cc7c096b52e
Issues: n/a | Plan steps: implementation-plan.md § 4.2 steps 1–5

## Problem / goal
Consumers (kgwctl, kgateway-ui, scripts) need krp's view as data. `krp dump` prints only styled text today.

## Expected behaviour

| Input / action | Expected |
|---|---|
| `krp dump --file <dump>` (no `-o`) | unchanged text output (byte-identical to base, apart from `--help`, which lists the new flag) |
| `krp dump --file <dump> -o json` | exit 0; stdout is one JSON document `{"schemaVersion":"v1alpha1","snapshot":{...},"warnings":[...]}`; `warnings` is `[]` when empty |
| `... -o json --route <r> --route-ns <ns> [--rule N]` | `snapshot` holds only the filtered listeners |
| `... -o json` on a dump with skipped sections | each warning appears in `warnings` and on stderr |
| `-o json --interactive` | error, non-zero exit, no stdout |
| `-o yaml` | error naming the allowed values, non-zero exit |

## Root cause (if known)
n/a (planned work)

## Acceptance tests
- A golden test in `cmd/krp` runs the dump command in-process with `-o json` for every dump under `testdata/`, plus one `--route` case and one `--rule` case, and compares against `testdata/golden/`. The `-update` flag rewrites the files.
- Flag-rejection tests for `-o json --interactive` and `-o yaml`.
- The 4.1 byte-comparison script still matches the base for every text case.

## Scope
In: `cmd/krp`, `testdata/golden/`, README usage.
Out: library packages (the JSON shape is the model's existing tags), test corpus (4.3).

## Plan / design impact
none

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] full test suite passes
- [ ] cross-model review (planned: at the phase review)
- [ ] phase-4.2-complete.md written; phase-status.md updated
