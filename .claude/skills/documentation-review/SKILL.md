---
name: documentation-review
description: Review documentation for clarity, structural consistency, accuracy, and contributor onboarding quality
user-invocable: true
---

# Documentation Review

## Task

Review project documentation for clarity, accuracy, structural consistency, and contributor accessibility.

## Instructions

1. Identify the target: specific doc file, README, docs/ directory, or full audit
2. Check against each area below
3. Provide specific rewrites for any failing sections

## Review Areas

### Readability

- No jargon without immediate definition in the same paragraph
- Real-world analogies for abstract concepts
- Sentences are short and direct — target a reader unfamiliar with the codebase
- Examples use small, concrete inputs rather than abstract descriptions
- Technical terms defined on first use

### Documentation Structure

- README.md follows the structure defined in `.claude/rules/docs.md`
- `docs/` files follow their defined sections (architecture, testing, deployment, contributing)
- Internal links between docs are not broken
- Scripts/commands table in README matches actual project manifest (e.g., `package.json`, `Makefile`, `pyproject.toml`)

### Contributor Onboarding

- A new contributor can complete their first task by following the guide alone
- Core APIs and abstractions documented with examples
- Troubleshooting section covers common issues
- Prerequisites and setup steps are current and accurate

### Consistency

- Same terminology used across all docs (no synonym drift between files)
- Same formatting conventions (headings, code blocks, tables)
- No contradictions between docs and `.claude/rules/`

### Comment Quality

- **Accuracy**: Comments accurately describe what the code does — no stale comments that contradict the code
- **Comment rot**: Identify comments that were accurate when written but no longer match the current implementation
- **Maintainability**: Comments explain "why" not "what" — code should be self-documenting for the "what"
- **Completeness**: Exported functions and types have documentation comments (JSDoc, docstrings, or equivalent)

## Rules

- Accuracy trumps readability — never simplify to the point of being wrong
- Follow the update trigger table in `.claude/rules/docs.md`

## Output Format

- CLEAR: [section] - meets readability and accuracy standards
- REVISE: [section] - specific issue + suggested rewrite
- MISSING: [section] - required content not present
