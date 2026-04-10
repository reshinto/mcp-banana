## Docker / CI-CD Rules

### Dockerfile Conventions

- Multi-stage build: builder stage compiles the Go binary; runner stage uses distroless base image.
- Pin base image versions explicitly — never use `:latest` in production Dockerfiles.
- Run as a non-root user in the final (distroless) stage.
- Copy only the compiled binary into the runner stage — no source, no test files, no secrets.
- Layer order: copy go.mod/go.sum and download deps before copying source (maximizes cache reuse).
- Bind to `0.0.0.0` inside the container (not `127.0.0.1`) so Docker port mapping works.
- Set `stop_grace_period: 120s` in docker-compose to allow in-flight requests to complete.

Example stage structure:
```
Stage 1 (builder): golang base image, go mod download, go build
Stage 2 (runner):  gcr.io/distroless/static-debian12, copy binary, set USER nonroot
```

### CI Pipeline

GitHub Actions runs on feature branch pushes and PRs:

1. **Lint** — `golangci-lint run`
2. **Format check** — `gofmt -w .`
3. **Type check** — `go vet ./...`
4. **Unit tests** — `go test -race -coverprofile=coverage.out ./...`
5. **Build** — `go build ./cmd/mcp-banana/`
6. **Security scan** — dependency audit + secret detection

### CD Pipeline

GitHub Actions runs on pushes to `main`:

1. Build and tag Docker image with full git SHA: `mcp-banana:<git-sha>`
2. Push image to registry
3. Deploy to DigitalOcean via SSH
4. Health check — if the health check fails, auto-rollback to previous image
5. Never overwrite an existing SHA-tagged image

### Deploy Targets

| Branch         | Environment  | Trigger              |
|----------------|--------------|----------------------|
| `main`         | Production   | On push              |
| Feature branch | CI only      | On PR open/update    |

### Secrets and Configuration

- Never commit secrets, tokens, or credentials to the repository.
- Use environment variables for all environment-specific configuration.
- Provide a `.env.example` file listing all required variables without values.
- Secrets in CI/CD must be stored in GitHub Actions secrets — not in repository files.
- Required env vars: `GEMINI_API_KEY`, `MCP_AUTH_TOKEN` (HTTP transport), `PORT` (HTTP transport).

### Image Tagging

- Tag production images with the full git SHA: `mcp-banana:<git-sha>`
- Never overwrite an existing SHA-tagged image.
