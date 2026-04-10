---
name: security-coverage-audit
description: Run security checks (OWASP client-side) and verify test coverage thresholds with E2E validation
user-invocable: true
---

# Security & Coverage Audit

## Task

Run a combined security and test coverage audit to verify the project meets quality and safety thresholds.

## Steps

### 1. Coverage Verification

- Run `go test ./...` with coverage enabled (per `rules/testing.md`)
- Verify coverage meets project-defined thresholds (see `rules/testing.md`)
- Identify any files below thresholds

### 2. E2E Test Validation

- Run `` (per `rules/testing.md`)
- See `rules/testing.md` for E2E spec conventions
- Confirm multi-viewport coverage if applicable (desktop, tablet, mobile)

### 3. Security Checks

- Run the project's dependency audit (e.g., `npm audit`, `pip audit`, `cargo audit`) and report vulnerabilities
- Scan for dynamic code execution: `eval()`, `exec()`, `Function()`, raw HTML injection
- Verify no unsafe dynamic content rendering patterns
- Check that any code editor or REPL components are read-only by default with no script execution
- Verify user inputs are sanitized before passing to any processing function
- Confirm no inline event handlers with string code

### 4. CSP Compliance

- No inline scripts in HTML entry points
- No `unsafe-eval` or `unsafe-inline` in Content Security Policy
- External resources loaded with integrity hashes where possible

### 5. Dependency Security

- Check lock file (e.g., `package-lock.json`, `poetry.lock`, `Cargo.lock`) for known vulnerabilities
- Flag any dependency with critical or high severity
- Verify no unnecessary runtime dependencies (dev deps not in production bundle)

### 6. Test Quality Analysis

- **Edge case gaps**: Identify missing edge case tests (empty inputs, boundary values, error conditions)
- **Test quality scoring**: Evaluate tests beyond coverage % — are assertions meaningful? Do tests verify behavior or just structure?
- **Critical path coverage**: Ensure the most important execution paths have thorough test coverage
- **Mutation resistance**: Would the tests catch a subtle bug (e.g., off-by-one, wrong comparison operator)?

## Rules

- Do not suppress security findings without documenting the exception
- Coverage below project-defined thresholds is a blocker — do not approve without justification
- Security findings at high/critical severity are blockers

## Output Format

- PASS: [area] - details
- FAIL: [area] - details + remediation steps
- BLOCKED: [finding] - must resolve before merge
