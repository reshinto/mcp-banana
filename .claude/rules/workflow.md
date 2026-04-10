## Workflow Rules

### Branch Strategy (MANDATORY — cannot be skipped unless the user explicitly says so)

- **Every new task MUST start on a new branch rebased from main.** This is non-negotiable.
- Before any code changes, always run: `git checkout main && git pull && git checkout -b <type>/<short-description>`
- If the branch already exists and main has new commits, rebase: `git rebase main`
- Branch names: `<type>/<short-description>` (e.g., `feat/user-auth`, `fix/modal-close`, `chore/update-deps`)
- A "new task" means any new user request that is not a direct continuation of the current in-progress PR
- Never reuse an existing feature branch for a different task
- Check existing feature branches before creating a new one

### Development Flow

1. **Branch** — create a task branch from main
2. **Implement** — write code and tests following the coding standards
3. **Test** — run the full CI sequence until all green
4. **Review** — self-review against architecture and security rules
5. **Merge** — push branch, open PR, merge after CI passes

### Git Operations

- The post-session quality gate must pass before: `git add`, `git commit`, `git push`, or PR creation
- Quality gate: `golangci-lint run` + `gofmt -w .` + `go vet ./...` + `go test -race -coverprofile=coverage.out ./...`
- Commit messages: imperative mood, present tense, no AI/assistant references, no "Co-Authored-By" attributions
- No force pushes to main
- Keep all related work on a single feature branch for large multi-phase tasks

### PR Requirements

- All CI checks must pass before opening a PR
- Create a PR immediately after pushing a feature branch — do not leave pushed branches without a PR
- PR title: under 70 characters, imperative mood
- PR body: summary bullet points + test plan checklist
