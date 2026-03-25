# codecli-channels

Turn chat channels into remote coding workbenches for Codex and Claude Code.

[English](README.md) · [简体中文](README.zh-CN.md)

`codecli-channels` is a self-hosted local bridge that connects chat channels to local coding CLIs. It keeps each conversation mapped to a local project, a bridge session, and a backend thread, so you can continue real engineering work from QQ, Feishu, or personal Weixin without losing execution context.

This project is not a general chatbot wrapper. It is designed for ongoing development tasks: project switching, session continuity, approval forwarding, controlled write access, task history, and interruption are first-class concerns.

## Why It Exists

- Keep a persistent mapping from channel conversation -> local project -> bridge session -> Codex or Claude thread.
- Let plain chat messages become the default work entrypoint instead of forcing slash-command-heavy workflows.
- Forward backend approvals back into chat so risky operations stay reviewable.
- Support multiple channel drivers behind one bridge core instead of coupling the project to a single platform.
- Keep onboarding details such as personal Weixin token setup outside the runtime bridge path.

## Current Status

| Area | Status | Notes |
| --- | --- | --- |
| QQ | usable | Uses the official QQ bot API and gateway, no OneBot dependency |
| Feishu | text MVP | Text-first driver with group/session isolation options |
| Personal Weixin | text MVP | Built-in QR/token onboarding, text send/reply path in place |
| Codex backend | primary path | Uses `codex app-server` over stdio JSON-RPC |
| Claude backend | supported | Uses `claude -p` headless mode |

Current gaps worth stating clearly:

- Feishu and Weixin are still text-focused drivers.
- Weixin media messages are not executed as media workflows yet.
- Interactive backend follow-up prompts such as `request_user_input` and MCP elicitation are not bridged yet.

## How It Works

```text
Channel Driver
  -> normalized channel.Message
  -> bridge service
  -> local project + persisted session
  -> Codex / Claude runner
  -> reply + approval loop back to chat
```

The key idea is that the bridge owns conversation routing and persistence, while channel drivers stay responsible for platform-specific transport details.

## Quick Start

### Requirements

- Go `1.25+`
- A working `codex` CLI installation if you want to use `bridge.backend=codex`
- Completed Codex login on the host machine if you use the Codex backend
- A working `claude` CLI installation if you want to use `bridge.backend=claude`
- Completed Claude Code login on the host machine if you use the Claude backend
- Credentials for at least one channel you want to enable
- Access to the local project directories you want to expose through the bridge

### 1. Copy the example config

```bash
cp config/codecli-channels.example.json config/codecli-channels.json
```

### 2. Fill in credentials, scopes, and project paths

Minimal QQ-oriented example:

```json
{
  "defaultAccountId": "default",
  "defaultTimezone": "Asia/Shanghai",
  "channels": {
    "default": {
      "type": "qq",
      "enabled": true,
      "options": {
        "appId": "YOUR_APP_ID",
        "clientSecret": "YOUR_APP_SECRET"
      }
    }
  },
  "bridge": {
    "enabled": true,
    "backend": "codex",
    "channelIds": ["default"],
    "allowAllTargets": false,
    "allowedScopes": [
      "default:user:YOUR_OPENID"
    ],
    "requireCommandPrefix": true,
    "projects": {
      "codecli-channels": {
        "path": "/path/to/codecli-channels",
        "description": "channels bridge"
      }
    },
    "defaultProject": "codecli-channels",
    "maxReplyChars": 1500,
    "readOnlyCodexSandbox": "read-only",
    "writeCodexSandbox": "workspace-write",
    "defaultRunMode": "write",
    "implicitMessageMode": "write",
    "claudeBinary": "claude",
    "claudeModel": null,
    "dataDir": "/path/to/codecli-channels/data",
    "stateFile": "/path/to/codecli-channels/data/state.json",
    "auditFile": "/path/to/codecli-channels/data/bridge-audit.jsonl"
  }
}
```

Notes:

- Prefer `channels`, `channelIds`, and `allowedScopes` over the legacy `accounts`, `accountIds`, and `allowedTargets` fields.
- Set `dataDir`, `stateFile`, and `auditFile` explicitly so runtime state ends up where you expect.
- `bridge.backend` sets the default backend, but you can switch per conversation with `/backend use codex` or `/backend use claude`.
- If `config/codecli-channels.json` is missing, the binary still falls back to `config/qqbot.json`.

### 3. Run the bridge

```bash
go run ./cmd/codecli-channels -config ./config/codecli-channels.json
```

Or build first:

```bash
go build -o ./bin/codecli-channels ./cmd/codecli-channels
./bin/codecli-channels -config ./config/codecli-channels.json
```

### 4. Verify the first conversation

Recommended order:

1. Confirm the log shows `QQ 网关 READY` if you started with QQ.
2. Send `/help`.
3. Send a plain message such as `Explain what this repository does`.
4. After the first task completes, send `/history`.

### Optional: onboard personal Weixin

The repository includes built-in personal Weixin onboarding commands:

```bash
go run ./cmd/codecli-channels weixin setup -config ./config/codecli-channels.json
```

This flow prints a QR code in the terminal, waits for confirmation, and writes the resulting token plus the related `bridge.channelIds` and `bridge.allowedScopes` entries back into your config.

If you already have a token:

```bash
go run ./cmd/codecli-channels weixin bind -config ./config/codecli-channels.json -token '<your-token>'
```

## Daily Usage

Plain messages are the default work entrypoint. The bridge keeps using the current project, session, and backend unless you switch them.

### General

| Command | Description |
| --- | --- |
| `/help` | Show environment-control commands |
| `/stop` | Interrupt the active task |
| `/history` | Show recent task records for the current project |
| plain message | Send work directly to the current backend |

### Project

| Command | Description |
| --- | --- |
| `/project list` | List configured projects |
| `/project current` | Show the active project |
| `/project use <alias>` | Switch project |

### Session

| Command | Description |
| --- | --- |
| `/session current` | Show the current session |
| `/session list` | List sessions for the current project |
| `/session new [name]` | Start a fresh session |
| `/session switch <id>` | Switch to an existing session |

### Backend

| Command | Description |
| --- | --- |
| `/backend current` | Show the active backend |
| `/backend list` | List available backends |
| `/backend use <codex|claude>` | Switch backend |

### Approval

| Command | Description |
| --- | --- |
| `/approve` | Approve the current pending request |
| `/approve session` | Approve and remember it for the current session |
| `/deny` | Deny the current pending request |

## Approval Model

There are two approval layers:

1. Bridge-level confirmation for obviously dangerous tasks such as `rm -rf`, `git reset --hard`, or `drop database`.
2. Native backend approval forwarded from Codex while a turn is running.

This keeps the bridge opinionated about high-risk requests without replacing the backend's own approval flow.

## Repository Layout

- `cmd/codecli-channels`: CLI entrypoint
- `internal/app`: top-level command routing and `weixin setup` / `weixin bind`
- `internal/bridge`: orchestration, command handling, approvals, history, reply behavior
- `internal/channel`: channel abstractions plus QQ, Feishu, and Weixin drivers
- `internal/codex`: Codex runners, including the `app-server` transport
- `internal/claude`: Claude Code headless runner
- `internal/config`: config loading, normalization, and compatibility helpers
- `internal/store`: persistent mapping for project, backend, session, and thread state
- `docs/`: public docs plus dated design and plan notes

## Limitations

- Feishu and Weixin are still driver-specific MVPs, not full parity with QQ.
- Personal Weixin support currently targets the iLink-style API used by this repository, not a generic public WeChat bot platform.
- Weixin replies depend on stored `context_token` values from earlier inbound messages.
- Backend interactive prompts that expect structured follow-up answers are not fully bridged yet.
- This is a self-hosted bridge and assumes the same machine can access local repositories plus the required CLIs.

## Documentation

Start here:

- [`docs/project-overview.md`](docs/project-overview.md)
- [`docs/architecture.md`](docs/architecture.md)
- [`docs/configuration.md`](docs/configuration.md)
- [`docs/troubleshooting.md`](docs/troubleshooting.md)
- [`docs/README.md`](docs/README.md)

Chinese docs:

- [`docs/project-overview.zh-CN.md`](docs/project-overview.zh-CN.md)
- [`docs/architecture.zh-CN.md`](docs/architecture.zh-CN.md)
- [`docs/configuration.zh-CN.md`](docs/configuration.zh-CN.md)
- [`docs/troubleshooting.zh-CN.md`](docs/troubleshooting.zh-CN.md)
- [`docs/README.zh-CN.md`](docs/README.zh-CN.md)

## Development

Run tests:

```bash
go test ./...
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution guidelines.

## Roadmap

- Better handling for interactive backend follow-up flows
- Richer Feishu and Weixin message capabilities
- Clearer startup diagnostics and config validation
- Deployment examples for Docker / launchd / systemd
- Continued GitHub open-source polish

## License

This repository is licensed under the [MIT License](LICENSE).
