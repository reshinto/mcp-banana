---
name: debugging
description: Systematic debugging with diagnostic paths covering data processing, rendering, state machine, asset loading, form/input, and build issues
user-invocable: true
---

# Debugging

## Task

Systematically debug issues using structured diagnostic paths before proposing fixes.

## Instructions

1. Identify the symptom category below
2. Follow the diagnostic path for that category
3. Isolate the root cause with evidence
4. Propose a minimal fix

## Diagnostic Paths

### Data Processing Bugs

Symptoms: wrong output, missing items, incorrect computed state, unexpected values

1. Check that all operations are producing the expected output at each step
2. Verify the sequence of transformations matches the expected order
3. Compare output against known-good inputs (use unit tests)
4. Check that the final state matches the expected result type
5. Verify any mappings or lookups reference accurate source data
6. Test edge cases: empty input, single element, boundary conditions, worst case

### Rendering Bugs

Symptoms: wrong styles, missing elements, animation glitches, layout issues

1. Check the data driving the render — is the correct variant/type being passed?
2. Verify the right component is selected for the given data shape
3. Check animation configuration — timing, exit transitions, spring parameters
4. Verify reduced-motion fallback works for users with motion preferences
5. Check that design tokens are used consistently, not raw hardcoded values
6. Test across relevant breakpoints (desktop, tablet, mobile)

### State Machine Bugs

Symptoms: stuck state, wrong transitions, speed/timing not changing, reset not working

1. Check state slice transitions — are all actions producing the correct next state?
2. Verify the current index or cursor is within valid bounds
3. Check timing logic (intervals, debounce, throttle) for correctness
4. Look for stale selectors — are derived values memoized or recomputed correctly?
5. Verify reset action fully clears all related state and side effects

### Asset Loading Bugs

Symptoms: missing content, empty panels, wrong format displayed, misaligned content

1. Verify the asset import or fetch returns content (not an empty object or undefined)
2. Check file paths in glob patterns or import statements for correctness
3. Verify content mappings reference accurate line numbers or offsets in the source
4. Check that all required asset variants exist and are registered
5. Test dynamic switching — does the UI update correctly when the active asset changes?

### Form/Input Bugs

Symptoms: state not updating, validation gaps, values persisting across sessions or views

1. Check that input state is stored in the expected location (component state vs. store)
2. Verify no unintended persistence (localStorage, URL params, global variables)
3. Check validation rules — are all invalid states correctly rejected?
4. Check that mutually exclusive constraints are enforced (e.g., conflicting selections)
5. Verify state resets correctly on view/context switch

### Build / Import Bugs

Symptoms: module not found, circular dependencies, tree-shaking issues

1. Check path alias resolution in the project's compiler/bundler config (e.g., `tsconfig.json`, `pyproject.toml`, `go.mod`, `Cargo.toml`)
2. Verify no circular dependencies between modules
3. Check glob patterns or dynamic imports — are paths statically analyzable? (no string interpolation)
4. Run the project's type checker or compiler to surface type errors
5. Check barrel exports or package entry points for missing or incorrect re-exports

## Rules

- Diagnose before fixing — understand the root cause first
- One fix at a time — don't combine multiple changes
- Write a failing test that reproduces the bug before fixing
- Verify the fix doesn't break other tests
