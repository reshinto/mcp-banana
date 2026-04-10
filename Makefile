.PHONY: build test lint fmt fmt-check vet run-stdio run-http clean rotate-token quality-gate

BINARY=mcp-banana
BUILD_FLAGS=-ldflags="-s -w" -trimpath

build:
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY) ./cmd/mcp-banana/

test:
	go test -coverprofile=coverage.out -race ./... -v

lint:
	golangci-lint run

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

vet:
	go vet ./...

run-stdio: build
	./$(BINARY) --transport stdio

run-http: build
	./$(BINARY) --transport http --addr 0.0.0.0:8847

clean:
	rm -f $(BINARY) coverage.out

rotate-token:
	@NEW_TOKEN=$$(openssl rand -hex 32); \
	echo "New token: $$NEW_TOKEN"; \
	echo ""; \
	echo "Step 1: Update MCP_AUTH_TOKEN on the server:"; \
	echo "  SSH into the server, edit /opt/mcp-banana/.env, set MCP_AUTH_TOKEN=$$NEW_TOKEN"; \
	echo "  Then restart: docker compose restart"; \
	echo ""; \
	echo "Step 2: Update your Claude Code config with the new token:"; \
	echo "  claude mcp add-json --scope user banana '{\"type\":\"http\",\"url\":\"http://localhost:8847/mcp\",\"headers\":{\"Authorization\":\"Bearer $$NEW_TOKEN\"}}'"

quality-gate: lint fmt-check vet test
	@echo "All checks passed"
