## Coding Standards

### Naming

- No single-character variable names (no `i`, `j`, `k`, `n`, `x`, `y`, `z`).
- No variable names composed of combined single characters (no `ij`, `xy`).
- Use meaningful names everywhere: tests, temporary variables, loop iterators.
- Examples: `elementIndex`, `outerIndex`, `currentNode`, `arrayLength`, `itemCount`.

**Allowed standard Go abbreviations** (do not expand these):
- `err` — error values
- `ctx` — context.Context
- `req` — request
- `resp` — response
- `cfg` — config/configuration struct
- `srv` — server
- `test` — `*testing.T` parameter in test functions (use `test *testing.T`, not `t *testing.T`)

All other abbreviations must use full words. Examples of banned forms:
`mw`, `msg`, `sig`, `idx`, `num`, `val`, `buf`, `tmp`, `fn`, `cb`, `ch`, `opts`, `addr`.

Use context-specific error names instead of generic `err` when multiple errors exist in scope:
`loadError`, `validationError`, `parseError`, etc.

### Go Test Convention

Test functions use `test *testing.T` (not `t *testing.T`):

```go
func TestSomething(test *testing.T) {
    // ...
}
```

### Code Quality

- DRY: extract shared logic into utilities or helper functions.
- Centralize public-facing and cross-file strings as constants. Single-package inline literals are fine when clear from context.
- Comment intent and purpose on every exported function and type.
- Security-critical code must include a `// SECURITY:` annotation explaining the invariant being enforced.
- No unsafe type escapes — avoid `interface{}` without type assertion; use typed interfaces.

### Modularity

- Max 500 lines per file. Split files that grow beyond this into focused sub-files.
- One responsibility per function.
- One objective per struct/type.

### Formatting

Run `gofmt -w .` before committing. All formatting is enforced by `gofmt` and `golangci-lint`.

The project `.golangci.yml` disables `stylecheck` and `revive` linters — do not re-enable them.

### Imports

Group imports in this order, separated by blank lines:

```go
import (
    // 1. Standard library
    "context"
    "fmt"

    // 2. External dependencies
    "github.com/mark3labs/mcp-go/mcp"
    "google.golang.org/genai"

    // 3. Internal packages
    "github.com/yourorg/mcp-banana/internal/config"
)
```
