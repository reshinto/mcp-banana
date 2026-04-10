---
name: claude-system-management
description: Create, update, and audit .claude/ system configuration including agents, skills, memory, rules, hooks, and CLAUDE.md
user-invocable: true
---

# Claude System Management

## Task

Create, update, or audit `.claude/` system configuration to ensure all components are consistent, current, and aligned with the project's codebase.

## Instructions

1. Identify the target: specific component (agent, skill, rule, hook) or full system audit
2. Verify against the standards below
3. Fix or report inconsistencies

## Component Standards

### Agent Definitions (`.claude/agents/*.md`)

Frontmatter schema (all keys required):

```yaml
---
name: <lowercase-hyphenated>
description: <one-line summary>
tools: [Read, Glob, Grep] or [Bash, Read, Glob, Grep]
model: sonnet
maxTurns: <8-15>
---
```

Content structure:

1. `# Agent Name` — title
2. `## Role` — one-paragraph purpose
3. `## Review Areas` — numbered checklist of responsibilities
4. `## Required Skills` — bullet list of domain expertise
5. `## Constraints` — bullet list of rules the agent must follow
6. `## Output Format` — labeled format (PASS/FAIL, APPROVED/BLOCKED, etc.)

### Skill Definitions (`.claude/skills/<name>/SKILL.md`)

Frontmatter schema:

```yaml
---
name: <lowercase-hyphenated>
description: <one-line summary>
user-invocable: true
---
```

Content structure:

1. `# Skill Name` — title
2. `## Task` — one-paragraph purpose
3. `## Instructions` — numbered steps
4. Skill-specific sections (checklists, review areas, steps)
5. `## Output Format` — if applicable

### Memory (`.claude/projects/<path>/memory/`)

- `MEMORY.md` index stays under 200 lines
- Each memory file has frontmatter: name, description, type (user/feedback/project/reference)
- No duplicate memories — update existing before creating new
- Stale memories pruned when discovered

### Rules (`.claude/rules/*.md`)

- Each rule file covers one topic
- No contradictions between rule files
- Rules align with CLAUDE.md summary
- Project-specific conventions, not generic best practices

### CLAUDE.md

- Reflects current tech stack and key paths
- Concise — detailed rules in `.claude/rules/`, not here
- No outdated references to removed features or changed conventions

## CLAUDE.md Quality Audit

- **Quality scoring**: Rate CLAUDE.md sections for accuracy, completeness, and token efficiency
- **Template adherence**: Verify structure matches the project's established format (Tech Stack, Architecture, Key Paths, Rules, Testing, Workflow)
- **Targeted improvements**: Suggest specific edits to improve signal density without increasing length
- **Drift detection**: Compare CLAUDE.md claims against actual codebase state

## Audit Procedure

When running a full audit:

1. List all agents, verify frontmatter + content structure
2. List all skills, verify frontmatter + content structure
3. Cross-check: agents reference skills that exist, workflow references agents that exist
4. Verify PLAN.md directory listing matches actual files
5. Verify CLAUDE.md key paths exist in the filesystem
6. Check rules for contradictions with each other and with CLAUDE.md
7. Score CLAUDE.md quality and suggest improvements

## Output Format

- VALID: [component] - configuration is correct and current
- DRIFT: [component] - mismatch between config and codebase + fix
- STALE: [component] - outdated entry to update or remove
