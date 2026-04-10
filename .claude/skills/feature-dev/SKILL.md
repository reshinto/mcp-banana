---
name: feature-dev
description: Guided feature development following a 7-step workflow covering product evaluation, design, architecture, implementation, review, QA, and documentation
user-invocable: true
---

# Feature Development

## Task

Guide the development of a new feature through a structured 7-step workflow.

## Workflow

### Step 1: Product Evaluation

Evaluate the feature for alignment with project goals:

- Does it deliver clear value to the target user?
- Does it follow established engagement or UX patterns for this product?
- Does it serve functional outcomes, not just aesthetics?

### Step 2: UI/UX Designer Consultation

For any visual changes:

- Theme compliance: follows the project's established color and spacing system
- Responsive layout: works across all supported breakpoints
- Accessibility: WCAG 2.1 AA, keyboard navigation, reduced-motion support

### Step 3: Architecture Review

For structural changes:

- Follows the project's established patterns — see `.claude/rules/` for specifics
- Module boundaries and state ownership are respected
- Build optimization is considered (code splitting, lazy loading)
- Security-by-design (no dynamic code execution, no raw HTML injection, no unsafe input handling)

### Step 4: Implementation

Follow the project's established directory and file conventions — see `.claude/rules/` for the canonical structure.

Use the project's existing patterns for:
- Module registration or entry points
- State management integration
- Test and story file placement

### Step 5: Senior Engineer Review

Code review checklist:

- Naming: no single-character variables, descriptive names throughout
- Types: no unsafe type escape hatches, correct use of variant types and strict type patterns
- DRY: reused strings and metadata centralized in constants
- Architecture: follows the project's established patterns

### Step 6: QA Validation

- Unit tests: correctness and integration coverage
- Coverage: per `.claude/rules/` testing thresholds
- E2E: follows project-specific spec conventions in `.claude/rules/`
- Security: no unsafe patterns, dependency audit clean

### Step 7: Documentation Review

- Feature documentation updated alongside code changes
- Explanations are accessible to the intended audience
- README, docs, and index files updated in the same pass as code changes
