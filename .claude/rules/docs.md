## Documentation Rules

### mcp-banana Doc Requirements

- **README.md** — primary documentation root. Must include: architecture overview, threat model summary, Claude Code integration instructions, environment variable reference, and Docker deployment steps.
- **Code comments** — every exported function and type must have a Go doc comment.
- **Security annotations** — security-critical code must include a `// SECURITY:` comment explaining the invariant being enforced.

### Update Triggers

Update documentation in the same pass as the code change. Do not defer doc updates to a follow-up PR.

| Change type                        | Docs to update                              |
|------------------------------------|---------------------------------------------|
| New feature or module added        | README, relevant package doc comment        |
| Existing feature modified          | README if user-facing, package doc comment  |
| File or directory renamed/moved    | All import references, README paths         |
| New environment variable or config | README setup section, .env.example          |
| New MCP tool added                 | README tools section, tool handler doc comment |
| New model added to registry        | README, `internal/gemini/registry.go` comment |
| API surface changed                | README, migration notes if breaking         |

### Style Guidelines

- Write for a competent developer unfamiliar with this codebase.
- Use active voice and present tense.
- Code blocks for all commands, file paths, and inline code references.
- Keep headings short — they are navigation anchors, not sentences.

### References

- Architecture decisions: `.claude/rules/architecture.md`
- Workflow and branching: `.claude/rules/workflow.md`
