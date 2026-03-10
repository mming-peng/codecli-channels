# Architecture

## Overview

`qq-codex-go` maps an incoming QQ conversation to a persistent local execution context:

```text
QQ conversation
  -> project
  -> bridge session
  -> Codex thread
```

This allows the bridge to preserve continuity, isolate projects, and route approvals back to the right QQ context.

## Main Components

- `internal/qq/`: QQ API client and gateway handling
- `internal/bridge/`: orchestration, commands, approvals, and reply behavior
- `internal/codex/`: transport layer for Codex integration
- `internal/store/`: local state and session persistence

## Codex Transport

The primary transport uses:

```text
codex app-server --listen stdio://
```

The bridge communicates over stdio JSON-RPC, using structured methods such as:

- `thread/start`
- `thread/resume`
- `turn/start`
- `turn/interrupt`
- approval request callbacks

This is more robust than relying on terminal scraping or ad-hoc output parsing.

## Approval Flow

There are two approval layers:

1. Bridge-level confirmation for obviously dangerous tasks
2. Native Codex approval during actual execution

QQ messages such as `/approve`, `/approve session`, and `/deny` are mapped back to Codex approval decisions.

## Response Flow

For long-running tasks, the bridge can send an early “working on it” message before the final answer arrives. Once the first final answer is available, the bridge prefers sending it immediately.
