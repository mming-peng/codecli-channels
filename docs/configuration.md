# Configuration

## Main File

The main config file is typically:

```text
config/qqbot.json
```

Start from:

```text
config/qqbot.example.json
```

## Top-Level Fields

- `defaultAccountId`: default QQ bot account
- `defaultTimezone`: timezone for the bridge
- `accounts`: QQ bot credential map
- `bridge`: bridge behavior, routing, project list, timeouts, and paths

## Important Bridge Fields

### `allowedTargets`

Controls who is allowed to use the bridge.

Recommended format:

- `default:user:<openid>`
- `default:group:<group_openid>`

### `projects`

Defines which local repositories can be used through QQ.

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
