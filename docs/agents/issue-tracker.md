# Issue tracker: GitHub

Issues and specs live in [GitHub Issues](https://github.com/danielcaze/distributed-rate-limiter/issues). Use `gh` from this repository, or pass `-R danielcaze/distributed-rate-limiter` elsewhere.

## Conventions

- Create: `gh issue create --title "..." --body-file <utf8-file>`.
- Read: `gh issue view <number> --comments`; fetch labels with `--json labels` when needed.
- List: `gh issue list --state open --json number,title,body,labels,assignees` with relevant filters.
- Comment: `gh issue comment <number> --body-file <utf8-file>`.
- Add or remove a label: `gh issue edit <number> --add-label <name>` or `--remove-label <name>`; preserve other labels.
- Close with a multiline explanation: comment with `--body-file`, then `gh issue close <number>`.

Record the scope, dependencies, acceptance criteria, and verification evidence on the relevant issue.

## Dependencies

Use GitHub sub-issues and issue dependencies where supported. Otherwise, link related issues and write `Blocked by: #<number>` in an issue body. Update the dependency statement when a blocker is resolved.
