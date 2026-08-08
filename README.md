# GrowthOS-Go

GrowthOS-Go is an AI-native marketing and growth platform built as an
evolutionary engineering course. The Go implementation is completed first;
database schemas, domain boundaries, and deployment topology evolve only when
new requirements justify the added complexity.

The repository currently contains the project skeleton and the completed
Lesson 1 product analysis. It deliberately does not contain the final schema,
microservices, or infrastructure stack.

## Current State

- Course progress: Lesson 1 of 96 complete
- Product code: not started (first HTTP service in Lesson 11)
- Database: not introduced (first connection and Migration in Lesson 13)
- Architecture: planned modular monolith, with evidence-based extraction later

See the [documentation home](docs/README.md), [course roadmap](docs/course/README.md),
and [Lesson 1](docs/course/part-01/lesson-01-why-ai-native-growth-platform.md).

## Repository Shape

```text
growth-os-go/
├── cmd/             # Executables; product entrypoints are added by the course
├── internal/        # Private domain and infrastructure modules
├── pkg/             # Deliberately small public Go packages
├── configs/         # Versioned, non-secret configuration examples
├── migrations/      # Forward-only SQL migrations, introduced in Lesson 15
├── scripts/         # Local automation
├── deploy/          # Deployment assets added as infrastructure appears
├── docs/            # Product, architecture, ADR, QA, and course evidence
└── web/             # React applications introduced in Lessons 91-92
```

Directories that are not needed yet contain only a `.gitkeep`. This preserves
the intended ownership boundaries without pretending later capabilities exist.
The React application begins in Lesson 14 and joins its first backend
integration in Lesson 15.

## Quality Gates

Requires Go 1.24 or newer.

```bash
make verify
```

`make verify` checks formatting, runs tests, validates the 96-lesson status
registry, checks ADR registration, and detects broken local Markdown links.

## Working Agreement

1. Every behavior or architecture change updates its owning document.
2. Significant decisions receive an ADR before or with the implementation.
3. A lesson becomes `completed` only when its lesson document and QA evidence
   are both registered and pass `make doc-check`.
4. Database changes are forward-only migrations once migrations are introduced.
5. Secrets and environment-specific configuration are never committed.
