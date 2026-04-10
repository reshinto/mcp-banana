# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy dependency files first for cache optimization
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o mcp-banana \
    ./cmd/mcp-banana/

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/mcp-banana /usr/local/bin/mcp-banana

# Run as non-root user (distroless:nonroot provides uid 65532)
USER nonroot:nonroot

EXPOSE 8847

ENTRYPOINT ["/usr/local/bin/mcp-banana"]
CMD ["--transport", "http", "--addr", "0.0.0.0:8847"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/mcp-banana", "--healthcheck", "--addr", "127.0.0.1:8847"]
