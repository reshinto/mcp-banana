---
name: tdd
description: TDD workflow for Go using go test with a 4-part test matrix (unit/correctness, integration, component/story, E2E)
user-invocable: true
---

# TDD

## Task

Implement features using test-driven development following a structured 4-part test matrix.

## Instructions

1. Identify the feature type: new feature, new component, state/store change, or bug fix
2. Write tests FIRST following the matrix below
3. Implement the minimum code to pass
4. Refactor while keeping tests green
5. Run the full quality gate before considering done

## Test Matrix by Feature Type

### New Feature

Write tests in this order:

1. **Correctness test**

   - Test the core logic with known inputs/outputs
   - Test edge cases: empty input, single element, boundary conditions, error states
   - Use meaningful variable names throughout

2. **Integration test**

   - Test that the feature integrates correctly with dependent modules
   - Test expected call sequences and data flow
   - Test that output state matches expected results
   - Cover all supported variations

3. **Component/story test**

   - Place in the relevant `__tests__/` directory (or project-equivalent)
   - Renders or exercises the feature with representative inputs

4. **E2E coverage** — see `.claude/rules/` for project-specific spec file conventions

### New Component or UI

1. Unit test for component logic (inputs, state, events)
2. Story or snapshot per significant state variant
3. Accessibility test where applicable: keyboard navigation, ARIA labels

### State/Store Change

1. Test state transitions for all new actions
2. Test derived or computed values
3. Test cross-module interactions if applicable

### Bug Fix

1. Write a failing test that reproduces the bug
2. Fix the bug
3. Verify the test passes

## Rules

- Tests go before implementation — no exceptions
- Coverage thresholds per `.claude/rules/` testing rules
- Use go test for unit tests and None for E2E
- Tests live in the feature's `__tests__/` directory (or project-equivalent)
