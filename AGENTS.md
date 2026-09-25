# Repository guidance

## Work and verification

Track work, dependencies, and acceptance evidence in GitHub Issues. Keep implementation and its focused tests in separate work units when planning project milestones; run ordinary verification for each change. Follow the [issue tracker conventions](docs/agents/issue-tracker.md) and [status labels](docs/agents/triage-labels.md).

## Source and generated files

Treat `proto/` as protobuf source and `gen/` as generated output. Regenerate Go files with the project script when changing protobuf definitions; do not edit generated files by hand. Keep architectural decisions in `docs/adr/` when a consequential trade-off warrants an ADR, and use the [domain glossary](CONTEXT.md) for shared terms.

## Configuration and hooks

Configure the service through environment variables. Add a secret-free `.env.example` alongside configuration work. Use Lefthook if repository hooks are added.
