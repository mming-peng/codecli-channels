# codecli-channels

Turn chat channels into remote coding workbenches for Codex and Claude Code.

[English](README.md) · [简体中文](README.zh-CN.md)

## Start Here

If you are setting this project up for the first time, skip the long-form details and go straight to the channel quickstart that matches your first integration:

- [QQ quickstart](quickstart/qq-quickstart.zh-CN.md)
- [Feishu quickstart](quickstart/feishu-quickstart.zh-CN.md)
- [Personal Weixin quickstart](quickstart/weixin-quickstart.zh-CN.md)

These onboarding guides are currently written in Simplified Chinese and cover the real first steps: where to get credentials, how to fill the minimal config, and how to verify the first message flow.

## What It Is

`codecli-channels` is a self-hosted local bridge that maps each chat conversation to:

- a local project
- a persisted bridge session
- a Codex or Claude backend thread

It is built for ongoing engineering work rather than generic chatbot wrapping. Project switching, session continuity, approval forwarding, and controlled local execution are the core use cases.

## Current Status

| Area | Status | Notes |
| --- | --- | --- |
| QQ | usable | Official QQ bot API and gateway |
| Feishu | text MVP | Text-first driver |
| Personal Weixin | text MVP | Built-in token onboarding flow |
| Codex backend | primary path | `codex app-server` over stdio JSON-RPC |
| Claude backend | supported | `claude -p` headless mode |

## Minimal Run

1. Copy the example config:

   ```bash
   cp config/codecli-channels.example.json config/codecli-channels.json
   ```

2. Follow one quickstart from `quickstart/` to finish channel credentials and bridge config.

3. Start the bridge:

   ```bash
   go run ./cmd/codecli-channels -config ./config/codecli-channels.json
   ```

4. In chat, send `/help`, then send a plain task message.

## Common Commands

| Command                       | Description |
|-------------------------------| --- |
| `/help`                       | Show available control commands |
| `/project use <alias>`        | Switch project |
| `/session new [name]`         | Start a new session |
| `/backend use codex` / `/backend use claude` | Switch backend |
| `/history`                    | Show recent task history |
| `/stop`                       | Interrupt the active task |

## Development

Run tests with:

```bash
go test ./...
```

This repository is licensed under the [MIT License](LICENSE).
