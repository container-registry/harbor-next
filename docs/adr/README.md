# Architecture Decision Records

Decisions that shape Harbor Next beyond a single pull request are recorded here,
one file per decision, numbered in the order they were accepted.

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-replication-and-vulnerability-metrics.md) | Replication and vulnerability metrics for alerting | Proposed |

## Conventions

- File name: `NNNN-short-title.md`, four-digit sequence, lowercase, hyphenated.
- Sections: Status, Context, Decision, Consequences, Alternatives considered.
  Add more only when the decision needs them.
- Status is one of `Proposed`, `Accepted`, `Superseded by ADR-NNNN`, `Deprecated`.
- A superseded ADR stays in place with its status updated; history is never rewritten.
- ADRs describe *why*. Reference documentation for *how* lives next to the code
  or in `docs/`.
