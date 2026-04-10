## Token Efficiency Rules

### General

- Prefer the most token-efficient path that does not reduce quality
- Do not repeat requirements back to the user unnecessarily
- Use subagents only when genuinely useful — keep them tightly scoped to a single concern
- Centralize reused strings and metadata to avoid duplication across files

### Code Generation

- Avoid generating boilerplate that can be inferred from context or existing patterns
- Use barrel exports to reduce import verbosity
- Prefer composition over inheritance to minimize class hierarchies
- Keep component and module files focused — one primary export per file

### Communication

- Be concise in commit messages and PR descriptions
- Skip filler words and unnecessary transitions ("As you can see...", "Certainly!", etc.)
- Lead with the answer or action, not the reasoning
- When showing command output, show raw output first — then summarize if needed
