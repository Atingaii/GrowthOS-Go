# Contributing

GrowthOS-Go treats documentation as part of the implementation, not as a
postponed deliverable.

Before opening or merging a change:

1. Read the [documentation governance rules](docs/standards/documentation-governance.md).
2. Update the owning product, architecture, contract, operations, or course document.
3. Add or supersede an [ADR](docs/decisions/README.md) for a significant decision.
4. Record verification evidence under [docs/qa](docs/qa/README.md).
5. Run `make verify`.

Commits should be small enough that code, documentation, tests, and migrations
describe one coherent change.
