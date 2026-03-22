# Troubleshooting

## QQ bot does not reply at all

Check:

- the process is still running
- the log contains `QQ 网关 READY`
- the target scope is present in `allowedScopes` (or `allowedTargets` if you still use legacy config)
- the `appId` and `clientSecret` are valid

## `/ping` works but normal tasks do not

Check:

- current overall state with `/status`
- current project selection with `/project current`
- current session with `/session current`
- whether the session is bound to an unexpected thread
- whether the bridge process logged the incoming QQ message

## A task keeps running but you want to change direction

Try:

- `/status` to confirm whether the conversation is still busy
- `/stop` to interrupt the current task
- `/history` to confirm what the bridge actually finished before you retry

## Approval does not show up in QQ

Check whether the task triggered:

- bridge-level confirmation only
- or native Codex approval

Then inspect logs for proactive message sending and approval-related bridge activity.

## Session state feels confusing

Use:

- `/status` for the current project/session/mode snapshot
- `/session list` for recent task summaries per session
- `/history` for recent task records across the current project

## State files are in an unexpected location

Set these explicitly in `codecli-channels.json` (or legacy `qqbot.json`):

- `bridge.dataDir`
- `bridge.stateFile`
- `bridge.auditFile`

## Slow responses

There are two separate reasons a response may feel slow:

- model execution time
- bridge behavior waiting for the final answer

The bridge can send an early “收到，正在处理…” message for slower tasks, but that improves perceived latency rather than raw model latency.
