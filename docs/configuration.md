# Configuration

## Main File

The main config file is typically:

```text
config/codecli-channels.json
```

Start from:

```text
config/codecli-channels.example.json
```

Legacy compatibility:

```text
config/qqbot.json
config/qqbot.example.json
```

## Top-Level Fields

- `defaultAccountId`: legacy QQ compatibility field; optional when using `channels`
- `defaultTimezone`: timezone for the bridge
- `channels`: channel instances across platforms
- `accounts`: legacy QQ credential map, still accepted and normalized into `channels`
- `bridge`: bridge behavior, routing, project list, timeouts, and paths

## Important Bridge Fields

### `channelIds`

Controls which channel instances participate in bridge orchestration.

Examples:

- `default`
- `feishu-main`
- `weixin-main`

### `allowedScopes`

Controls who is allowed to use the bridge.

Recommended format:

- `default:user:<openid>`
- `default:group:<group_openid>`
- `feishu-main:p2p:<chat_id>`
- `weixin-main:dm:<peer_user_id>`

Compatibility notes:

- legacy `accountIds` is still accepted and mapped into `channelIds`
- legacy `allowedTargets` is still accepted and mapped into `allowedScopes`

### `projects`

Defines which local repositories can be used through the current channel.

Each project should provide:

- alias
- absolute path
- optional description

### `defaultRunMode`

Sets the default mode for newly created sessions:

- `write`
- `read`

### `implicitMessageMode`

Controls the initial mode used when a plain message auto-creates a session; later plain messages follow the current session mode:

- `write`
- `read`

### Path Fields

You should usually set these explicitly:

- `dataDir`
- `stateFile`
- `auditFile`

This avoids confusing default path expansion behavior.

### `maxReplyChars`

Preferred reply splitting limit for chat-channel responses.

Legacy `qqMaxReplyChars` is still accepted and mapped into `maxReplyChars`.
