# Models

## Overview

mcp-banana exposes three model aliases. Each alias maps to an internal Gemini model ID that is never exposed to Claude Code or logged. Aliases are the only identifiers that appear in tool parameters, tool responses, and log output.

The default model when none is specified is `nano-banana-2`.

## Model Aliases

| Alias | Latency | Best For | Description |
|---|---|---|---|
| `nano-banana-2` | 5–10s | Iterative work, drafts, batch generation | Fast, high-volume image generation |
| `nano-banana-pro` | 15–45s | Final assets, photorealistic images, complex scenes | Professional quality with advanced reasoning |
| `nano-banana-original` | 3–8s | Quick previews, high-volume batch work | Speed and efficiency optimized |

All three models support both `generate` (text to image) and `edit` (image + instructions to image) operations.

## Model Details

### nano-banana-2

The default and balanced choice. Produces good results for most tasks within 5–10 seconds. Use this when you are unsure which model to choose, or when iterating on ideas.

- **Capabilities:** generate, edit
- **Typical latency:** 5–10s
- **Best for:** Iterative work, drafts, batch generation

### nano-banana-pro

The highest quality model. Applies advanced reasoning to complex scenes and photorealistic output. Expect 15–45 seconds per request. Concurrent pro requests are throttled by a semaphore (default: 3 simultaneous) to prevent API quota exhaustion. If the semaphore is full and the request context is cancelled before a slot opens, the request fails immediately.

- **Capabilities:** generate, edit
- **Typical latency:** 15–45s
- **Best for:** Final assets, photorealistic images, complex scenes

### nano-banana-original

The fastest model. Optimized for throughput at the expense of output richness. Use for high-volume batch work or rapid previews where quality is secondary.

- **Capabilities:** generate, edit
- **Typical latency:** 3–8s
- **Best for:** Quick previews, high-volume batch work

## Recommendation Logic

The `recommend_model` tool selects a model based on `priority` and `task_description`. The rules in order:

1. `priority=speed` → always `nano-banana-original`
2. `priority=quality` → always `nano-banana-pro`
3. `priority=balanced` (or empty) → keyword scan of `task_description`:
   - **Pro keywords** (first match wins): `professional`, `photorealistic`, `detailed`, `complex`, `final` → `nano-banana-pro`
   - **Speed keywords** (if no pro keyword): `quick`, `draft`, `sketch`, `iterate`, `batch`, `preview` → `nano-banana-original`
   - **No keyword match** → `nano-banana-2`

See [tools-reference.md](tools-reference.md#recommend_model) for the full parameter schema.

## GeminiID Security

The `GeminiID` field in `ModelInfo` maps a Nano Banana alias to its underlying Gemini model string. This field is **internal-only** and must never appear in any tool response, log entry, or error message.

The `AllModelsSafe()` function returns `[]SafeModelInfo`, which omits `GeminiID` entirely. All tool responses use `SafeModelInfo`. The test `TestListModelsHandler_NoGeminiID` in `internal/tools/tools_test.go` verifies that neither `gemini_id` nor `GeminiID` appears in any `list_models` response.

## Sentinel ID Verification Procedure

`internal/gemini/registry.go` is the single source of truth for model IDs. To prevent accidental deployment with an unverified mapping, any alias whose `GeminiID` is set to the sentinel value `VERIFY_MODEL_ID_BEFORE_RELEASE` will cause `ValidateRegistryAtStartup()` to return an error and the server to refuse to start.

Current verified mappings in the registry:

```
nano-banana-2        → gemini-3.1-flash-image-preview
nano-banana-pro      → gemini-3-pro-image-preview
nano-banana-original → gemini-2.5-flash-image
```

To verify or update a mapping:

1. Check the [Gemini API Models documentation](https://ai.google.dev/gemini-api/docs/models) or list models via the API:

   ```bash
   curl "https://generativelanguage.googleapis.com/v1beta/models?key=$GEMINI_API_KEY"
   ```

2. Find the model IDs for image generation models. Confirm which IDs correspond to flash and pro image generation variants.

3. Open `internal/gemini/registry.go` and update the `GeminiID` field for each alias. Replace any `VERIFY_MODEL_ID_BEFORE_RELEASE` sentinel with the confirmed ID:

   ```go
   "nano-banana-2": {
       Alias:    "nano-banana-2",
       GeminiID: "gemini-3.1-flash-image-preview", // verified
       // ...
   },
   ```

4. If a confirmed ID cannot be found for `nano-banana-original`, remove that entry from the registry map entirely.

5. Run the quality gate to confirm the server starts successfully:

   ```bash
   make quality-gate
   ```

## ValidateRegistryAtStartup

`gemini.ValidateRegistryAtStartup()` runs during the startup sequence in `cmd/mcp-banana/main.go`, before any requests are accepted. It iterates the registry and returns an error if any alias still has the sentinel `GeminiID`:

```
registry validation failed: model "nano-banana-2" has unverified GeminiID -- verify at https://ai.google.dev/gemini-api/docs/models before release
```

This is expected behavior when the registry has not been updated, not a bug.

The CD pipeline also blocks deployment if any sentinel value is present in `internal/gemini/registry.go`.

## Docker Health Implications

The container health check runs every 30 seconds:

```
/usr/local/bin/mcp-banana --healthcheck --addr 127.0.0.1:8847
```

If the server refuses to start due to unverified model IDs, the binary exits before binding to port 8847. All health checks fail, and Docker marks the container `unhealthy` after three consecutive failures. The container serves no traffic in this state. This is intentional: a container with unverified model IDs should never reach production.
