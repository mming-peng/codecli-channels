# Contributing

Thanks for your interest in contributing to `qq-codex-go`.

## Development Setup

Requirements:

- Go `1.22+`
- A working `codex` CLI installation
- Optional: QQ bot credentials if you want to run real integration tests

Clone the repository and run tests:

```bash
go test ./...
```

## Code Style

- Keep changes focused and minimal
- Follow the existing Go style in the repository
- Prefer root-cause fixes over surface-level patches
- Avoid unrelated refactors in the same change
- Keep user-facing documentation clear and practical

## Testing

Before opening a pull request, run:

```bash
go test ./...
```

If you changed bridge behavior, include a short note describing:

- what changed
- how you tested it
- whether it was verified with real QQ / Codex infrastructure or only locally

## Pull Requests

A good pull request should include:

- a clear summary of the change
- the motivation behind it
- testing notes
- any known limitations or follow-up work

Small, reviewable pull requests are preferred.

## Documentation

If you change commands, config behavior, approval flow, or transport behavior, update the relevant docs:

- `README.md`
- `README.zh-CN.md`
- files under `docs/`

## Security

Do not commit:

- real `appId` / `clientSecret`
- personal `openid` / `group_openid`
- local machine secrets or tokens
- private logs containing sensitive payloads
