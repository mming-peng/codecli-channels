# 排障说明

## QQ 机器人完全不回消息

优先检查：

- 进程是否还在运行
- 日志里是否出现 `QQ 网关 READY`
- `allowedTargets` 里是否包含对应的 `openid` 或 `group_openid`
- `appId` / `clientSecret` 是否正确

## `/ping` 有回复，但普通任务没反应

优先检查：

- 当前项目是否正确：`/project current`
- 当前会话是否异常：`/session current`
- 会话是否绑定到了不合适的 thread
- bridge 日志里是否记录到了该 QQ 消息

## QQ 里没有收到审批

先区分是哪一种：

- bridge 自己的高风险确认
- Codex 原生审批

然后查看日志里是否出现主动消息发送和审批相关记录。

## 状态文件落在了意外的位置

建议在 `qqbot.json` 中显式设置：

- `bridge.dataDir`
- `bridge.stateFile`
- `bridge.auditFile`

## 响应很慢

通常有两类原因：

- 模型本身执行慢
- bridge 在等待最终答案

bridge 可以先回“收到，正在处理…”，但这改善的是体感，不是模型的真实计算耗时。
