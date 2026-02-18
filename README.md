# furiwake (振分) — AI Agent Routing Proxy

[日本語](README.ja.md)

A lightweight routing proxy that sits between [Claude Code](https://docs.anthropic.com/en/docs/claude-code) / [Codex CLI](https://github.com/openai/codex) and multiple AI providers. Automatically routes requests to different backends based on markers in system prompts. Works with both team agents and standalone usage.

```
                         furiwake.yaml
                              |
  Claude Code            furiwake (:52860)             Providers
 +-----------+     +-------------------------+
 |           |     |  @route:anthropic        | -----> Anthropic API  (passthrough)
 | Leader    | --> |  @route:codex            | -----> ChatGPT/Codex  (Responses API)
 | Coder     | --> |  @route:openai           | -----> OpenAI         (Chat Completions)
 | Researcher| --> |  @route:ollama           | -----> Ollama         (local)
 |           |     |  @route:openrouter       | -----> OpenRouter     (Chat Completions)
 +-----------+     +-------------------------+
    Anthropic           API translation            OpenAI-compatible
    Messages API        + SSE streaming            APIs
```

## Why?

Claude Code agents all talk to Anthropic by default. But you might want different agents using different providers:

- Leader agent → Anthropic (direct)
- Coder agent → Codex or OpenAI
- Researcher → free model via OpenRouter

Add a `@route:` marker to each agent file. furiwake translates between Anthropic Messages API and OpenAI-compatible APIs automatically — no code changes, no complex config.

### What makes furiwake different?

- **Marker-in-prompt routing** — `@route:` / `@model:` / `@reasoning:` markers go in the agent file, not a separate config. Add an agent, add a marker, done.
- **Single binary** — One Go binary, one YAML config. No Python, no Docker, no database.
- **Codex-native** — First-class ChatGPT Responses API (Codex) support, not just OpenAI Chat Completions.

## Quick Start

### 1. Install

```bash
curl -fsSL https://github.com/HoshimuraYuto/furiwake/releases/latest/download/install.sh | bash
```

Or build from source:

```bash
go build -o furiwake .
cp furiwake.yaml.example furiwake.yaml
```

### 2. Configure

Edit `furiwake.yaml` to set your providers and API keys.

### 3. Run

```bash
./furiwake
```

### 4. Connect Claude Code

```bash
export ANTHROPIC_BASE_URL=http://localhost:52860
claude
```

To persist, append to `~/.bashrc` (or `~/.zshrc`):

```bash
echo 'export ANTHROPIC_BASE_URL=http://localhost:52860' >> ~/.bashrc
```

If using the OpenAI or OpenRouter providers, add their API keys too:

```bash
echo 'export OPENAI_API_KEY="sk-proj-..."' >> ~/.bashrc
echo 'export OPENROUTER_API_KEY="sk-or-..."' >> ~/.bashrc
```

If using the Codex provider, log in once with the Codex CLI (this creates `~/.codex/auth.json`):

```bash
codex
```

Or set everything at once:

```bash
cat >> ~/.bashrc << 'EOF'
export ANTHROPIC_BASE_URL=http://localhost:52860
export OPENAI_API_KEY="sk-proj-..."
export OPENROUTER_API_KEY="sk-or-..."
EOF

source ~/.bashrc
```

Then reload: `source ~/.bashrc`

## Features

**Routing & Translation**
| Feature | Description |
|---------|-------------|
| Marker-based routing | `@route:<provider>` in system prompts determines the backend |
| Per-agent model override | `@model:<model>` overrides provider default model per request |
| Reasoning control | `@reasoning:<level>` overrides reasoning effort (Codex/Responses) |
| API translation | Anthropic Messages API <-> OpenAI Chat Completions / ChatGPT Responses API |
| Streaming | Full SSE stream translation in both directions |

**Operations**
| Feature | Description |
|---------|-------------|
| Multiple auth methods | Bearer token, Codex (`~/.codex/auth.json`), or none |
| Retry with backoff | Automatic exponential backoff on 429 responses, up to 5 retries |
| Configurable timeout | `timeout_seconds` in config for long-running requests |
| Audit logging | `[HTTP-OUT]` logs with actual HTTP request URL for every upstream call |
| Debug file logging | All levels to `furiwake-debug.log`; console shows INFO+ |

**Distribution**
| Feature | Description |
|---------|-------------|
| Single binary | Go with one external dependency (`gopkg.in/yaml.v3`) |
| Cross-platform | Linux, macOS, Windows (amd64/arm64) |
| Release assets | Semantic Release publishes multi-platform binaries + `install.sh` |

## How Routing Works

Add markers in an agent's system prompt to control routing:

```markdown
<!-- .claude/agents/implementer.md -->
<!-- @route:codex @model:gpt-5.3-codex @reasoning:high -->

You are a code implementation specialist...
```

A working sample is included at `.claude/agents/coder.md`.

### Markers

| Marker               | Effect                                     | Example             |
| -------------------- | ------------------------------------------ | ------------------- |
| `@route:<name>`      | Route to a specific provider               | `@route:codex`      |
| `@model:<name>`      | Override provider's default model          | `@model:gpt-5-mini` |
| `@reasoning:<level>` | Override reasoning effort (`chatgpt` only) | `@reasoning:high`   |

### Resolution Rules

```
                    @route marker found?
                     /            \
                   yes             no
                   /                \
          providers[name]     default_provider (anthropic)
                 |
         @model marker found?
          /            \
        yes             no
        /                \
  use specified      providers[name].model
     model           from furiwake.yaml
```

- `@reasoning` allowed values: `none | minimal | low | medium | high | xhigh`
- If `@reasoning` is missing, `providers.<name>.reasoning_effort` from config is used

### Request Flow

```
  Claude Code                   furiwake                          Provider
      |                            |                                 |
      |   POST /v1/messages        |                                 |
      |  (Anthropic format)        |                                 |
      |--------------------------->|                                 |
      |                            |  1. Parse JSON body             |
      |                            |  2. Detect @route / @model /    |
      |                            |     @reasoning markers          |
      |                            |  3. Resolve provider & model    |
      |                            |                                 |
      |                            |  [passthrough]                  |
      |                            |--- relay as-is ---------------->| Anthropic
      |                            |                                 |
      |                            |  [openai / chatgpt]             |
      |                            |--- translate request ---------->| OpenAI / Codex
      |                            |<-- translate SSE response ------| / Ollama / OpenRouter
      |                            |                                 |
      |   SSE stream               |                                 |
      |  (Anthropic format)        |                                 |
      |<---------------------------|                                 |
```

## Configuration

All configuration lives in `furiwake.yaml` (see [furiwake.yaml.example](furiwake.yaml.example)). All top-level fields are **required**:

| Field              | Description                        | Example               |
| ------------------ | ---------------------------------- | --------------------- |
| `listen`           | Bind address                       | `":52860"`            |
| `spoof_model`      | Model name reported to Claude Code | `"claude-sonnet-4-6"` |
| `default_provider` | Fallback when no `@route` marker   | `"anthropic"`         |
| `timeout_seconds`  | HTTP client timeout                | `300`                 |
| `providers`        | Provider definitions               | (see below)           |

### Providers

```yaml
providers:
  anthropic:
    type: passthrough
    url: "https://api.anthropic.com"
    auth:
      type: none

  codex:
    type: chatgpt
    url: "https://chatgpt.com/backend-api/codex/responses"
    model: "gpt-5.3-codex"
    reasoning_effort: "medium"
    auth:
      type: codex

  openai:
    type: openai
    url: "https://api.openai.com/v1/chat/completions"
    model: "gpt-5-mini"
    auth:
      type: bearer
      token_env: "OPENAI_API_KEY"

  ollama:
    type: openai
    url: "http://localhost:11434/v1/chat/completions"
    model: "qwen2.5-coder:32b"
    auth:
      type: none

  openrouter:
    type: openai
    url: "https://openrouter.ai/api/v1/chat/completions"
    model: "z-ai/glm-4.5-air:free"
    auth:
      type: bearer
      token_env: "OPENROUTER_API_KEY"
```

### Provider Types

| Type          | Description                                                       |
| ------------- | ----------------------------------------------------------------- |
| `passthrough` | Relays requests directly to the Anthropic API without translation |
| `openai`      | Translates to OpenAI Chat Completions API format                  |
| `chatgpt`     | Translates to ChatGPT Responses API format                        |

### Auth Types

| Type     | Description                                                                             |
| -------- | --------------------------------------------------------------------------------------- |
| `none`   | No authentication                                                                       |
| `bearer` | Bearer token from environment variable (set via `token_env`)                            |
| `codex`  | Reads token and account ID from `~/.codex/auth.json`, sends `Chatgpt-Account-Id` header |

## Endpoints

| Endpoint                    | Method | Description                              |
| --------------------------- | ------ | ---------------------------------------- |
| `/health`                   | GET    | Health check                             |
| `/v1/messages`              | POST   | Anthropic Messages API (main endpoint)   |
| `/v1/messages/count_tokens` | POST   | Token counting (passthrough or estimate) |

## Verification

After starting furiwake, verify from another terminal:

```bash
# Health check
curl -s http://localhost:52860/health

# Codex (ChatGPT Responses API)
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:codex @model:gpt-5.1-codex-mini @reasoning:low",
    "messages":[{"role":"user","content":"hi"}]
  }'

# OpenAI (Chat Completions API)
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:openai @model:gpt-5-mini",
    "messages":[{"role":"user","content":"hi"}]
  }'

# Ollama (local)
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:ollama @model:qwen2.5-coder:32b",
    "messages":[{"role":"user","content":"hi"}]
  }'

# OpenRouter
curl -s http://localhost:52860/v1/messages \
  -H "content-type: application/json" \
  -d '{
    "model":"claude",
    "stream": true,
    "system":"@route:openrouter @model:z-ai/glm-4.5-air:free",
    "messages":[{"role":"user","content":"hi"}]
  }'
```

## Installer Options

```bash
# Specific version
curl -fsSL https://github.com/HoshimuraYuto/furiwake/releases/download/v1.2.3/install.sh | \
  bash -s -- --version v1.2.3

# Custom install/config locations
curl -fsSL https://github.com/HoshimuraYuto/furiwake/releases/latest/download/install.sh | \
  bash -s -- --bin-dir "$HOME/.local/bin" --config-dir "$HOME/.config/furiwake"
```

The installer can set up a systemd user service automatically on Linux.

## Logging

furiwake outputs logs at two levels:

- **Console** — INFO, WARN, ERROR (color-coded)
- **File** (`furiwake-debug.log`) — All levels including DEBUG

```
[HTTP-OUT] req=req_1739980000000 route=codex model=gpt-5.3-codex reasoning=high POST https://chatgpt.com/backend-api/codex/responses
```

For ChatGPT/Codex providers, DEBUG-level logs include outgoing request payloads (`[CODEX-REQ]`) and incoming SSE events (`[CODEX-SSE]`).

## Development

The development environment uses [Dev Containers](https://code.visualstudio.com/docs/devcontainers/containers). Open the repository in VS Code or GitHub Codespaces and it will automatically set up Go, Node.js, and all required tools.

```bash
make build            # Build
make run              # Run
make test             # Test
make fmt              # Format code (gofmt + prettier)
make cross            # Cross-compile for all platforms
make release-assets   # Build release artifacts (cross binaries + checksums)
```

### Background Mode

```bash
./furiwake --config furiwake.yaml >/tmp/furiwake.log 2>&1 &
echo $!    # PID
kill <PID>
```

### Daemon Mode (systemd user service)

When installed via `install.sh` on Linux:

```bash
systemctl --user status furiwake
systemctl --user restart furiwake
systemctl --user stop furiwake
journalctl --user -u furiwake -f
```

## Project Structure

```
furiwake/
├── main.go                 # Entry point, signal handling
├── config.go               # YAML config loading
├── server.go               # HTTP server, endpoint routing, token estimation
├── router.go               # @route:<name> detection, provider resolution
├── passthrough.go          # Anthropic passthrough relay
├── translate_request.go    # Anthropic -> OpenAI request translation
├── translate_messages.go   # Message/content block translation
├── translate_stream.go     # OpenAI SSE -> Anthropic SSE translation
├── translate_chatgpt.go    # ChatGPT Responses API translation + SSE
├── sse.go                  # SSE event parser
├── types.go                # All struct definitions
├── auth.go                 # Auth + retry with exponential backoff
├── logger.go               # Console + file logger
├── install.sh              # Release installer (binary + config + systemd user service)
├── Makefile
└── furiwake.yaml.example
```

## Acknowledgments

furiwake builds on ideas from the broader Claude Code proxy ecosystem. We appreciate the pioneering work of these projects:

- [claude-code-proxy](https://github.com/1rgs/claude-code-proxy) / [claude-code-proxy](https://github.com/fuergaosi233/claude-code-proxy) — Early implementations of Anthropic-to-OpenAI translation
- [claude-code-mux](https://github.com/9j/claude-code-mux) — High-performance Rust proxy with multi-provider failover
- [CCProxy](https://github.com/starbased-co/ccproxy) — Intelligent routing with LiteLLM integration
- [LiteLLM](https://github.com/BerriAI/litellm) — The universal LLM gateway that proved the value of unified API access
- [Bifrost](https://github.com/maximhq/bifrost) — Demonstrating what's possible with Go-native gateway performance
- [HydraTeams](https://github.com/Pickle-Pixel/HydraTeams) — Model-agnostic Agent Teams proxy with full Claude Code tooling support

## License

MIT
