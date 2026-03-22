# codecli-channels Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将项目 `qq-codex-go` 全量重命名为 `codecli-channels`，并在不破坏现有 QQ 能力的前提下完成产品身份去 QQ 化。

**Architecture:** 本次改造只重命名项目身份层与公共配置层，不重构当前 QQ 平台实现。代码层通过 module/path、命令入口、日志、运行时目录和文档同步改名；配置层增加兼容字段 `maxReplyChars`，为未来多平台能力预留中性命名。

**Tech Stack:** Go 1.25、标准库、当时的 `internal/bridge`/`internal/config`/`internal/qq` 模块、标准库测试

---

## 文件结构

- Modify: `go.mod`
  - 更新 module 名为 `codecli-channels`
- Move: `cmd/qq-codex-go/main.go` -> `cmd/codecli-channels/main.go`
  - 更新 import path、默认配置路径、启动日志
- Modify: `internal/**/*.go`
  - 更新 import path 与运行时目录名
- Modify: `internal/config/config.go`
  - 新增 `maxReplyChars`，兼容旧 `qqMaxReplyChars`
- Modify: `config/qqbot.example.json`
  - 改示例 alias 与描述
- Add: `config/codecli-channels.example.json`
  - 作为新的主示例配置
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `CONTRIBUTING.md`
- Modify: `docs/*.md`
  - 同步项目名与多平台定位
- Modify: `internal/config` 相关测试或新增测试
  - 覆盖 `maxReplyChars` 兼容逻辑

### Task 1: 先用测试锁定配置兼容行为

**Files:**
- Modify: `internal/config/config.go`
- Create or Modify: `internal/config/config_test.go`

- [ ] **Step 1: 写 `maxReplyChars` 兼容测试**

覆盖：
- 只设置 `maxReplyChars` 时能生效
- 只设置 `qqMaxReplyChars` 时仍兼容
- 两者同时设置时优先使用 `maxReplyChars`

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config -run TestNormalizeReplyCharSettings`
Expected: FAIL，提示字段或逻辑不存在

- [ ] **Step 3: 做最小实现**

在 `BridgeConfig` 中增加中性字段，并在 `Normalize` 中处理兼容优先级。

- [ ] **Step 4: 重新运行 config 测试**

Run: `go test ./internal/config`
Expected: PASS

- [ ] **Step 5: 提交当前小步**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add maxReplyChars compatibility"
```

### Task 2: 重命名 module、命令入口和 import path

**Files:**
- Modify: `go.mod`
- Move: `cmd/codecli-channels/main.go` -> `cmd/codecli-channels/main.go`
- Modify: all Go files importing `codecli-channels/...`

- [ ] **Step 1: 写或补充最小编译级测试保障**

不额外新增复杂行为测试，使用现有测试套件作为编译回归保护。

- [ ] **Step 2: 运行基线测试**

Run: `go test ./...`
Expected: PASS，作为改名前基线

- [ ] **Step 3: 做最小实现**

更新 module 名、命令目录、默认配置路径、启动日志和全部 import path。

- [ ] **Step 4: 重新运行全量测试**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: 提交当前小步**

```bash
git add go.mod cmd internal
git commit -m "refactor: rename module to codecli-channels"
```

### Task 3: 统一运行时目录、客户端标识和对外名称

**Files:**
- Modify: `internal/codex/runner.go`
- Modify: `internal/codex/app_server.go`
- Modify: `internal/claude/runner.go`
- Modify: `LICENSE`

- [ ] **Step 1: 写会受命名影响的失败测试**

补充或修改已有测试，确保运行时目录/环境中不再出现旧项目名。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/codex ./internal/claude`
Expected: FAIL，仍包含旧项目名

- [ ] **Step 3: 做最小实现**

替换运行时目录名前缀、app-server client name、许可证项目名。

- [ ] **Step 4: 重新运行相关测试**

Run: `go test ./internal/codex ./internal/claude`
Expected: PASS

- [ ] **Step 5: 提交当前小步**

```bash
git add internal/codex internal/claude LICENSE
git commit -m "refactor: rename runtime identifiers to codecli-channels"
```

### Task 4: 迁移示例配置和默认启动方式

**Files:**
- Modify: `config/qqbot.example.json`
- Add: `config/codecli-channels.example.json`
- Modify: `cmd/codecli-channels/main.go`

- [ ] **Step 1: 写配置示例存在性的失败检查**

使用简单文件存在/路径引用检查，确保新示例文件加入，并且默认路径改到新文件名。

- [ ] **Step 2: 运行检查确认失败**

Run: `test -f config/codecli-channels.example.json && rg -n "codecli-channels.json|codecli-channels.example.json" cmd README* docs`
Expected: FAIL

- [ ] **Step 3: 做最小实现**

新增新示例配置文件，保留旧 QQ 示例作为兼容或迁移参考，并更新默认 config 路径。

- [ ] **Step 4: 重新运行检查**

Run: `test -f config/codecli-channels.example.json && rg -n "codecli-channels.json|codecli-channels.example.json" cmd README* docs`
Expected: PASS

- [ ] **Step 5: 提交当前小步**

```bash
git add config cmd/codecli-channels/main.go README.md README.zh-CN.md
git commit -m "chore: add codecli-channels config defaults"
```

### Task 5: 更新文档定位为多平台 channels 产品

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `CONTRIBUTING.md`
- Modify: `docs/architecture.md`
- Modify: `docs/architecture.zh-CN.md`
- Modify: `docs/configuration.md`
- Modify: `docs/configuration.zh-CN.md`
- Modify: `docs/troubleshooting.md`
- Modify: `docs/troubleshooting.zh-CN.md`
- Modify: `docs/2026-03-22-product-ux-upgrades-*.md`

- [ ] **Step 1: 写文档一致性检查**

检查 README/docs 中不再保留旧项目名作为主身份，只在“当前 QQ 实现”语境中出现必要说明。

- [ ] **Step 2: 运行检查确认失败**

Run: `rg -n "codecli-channels" README* CONTRIBUTING.md docs`
Expected: FAIL

- [ ] **Step 3: 做最小实现**

统一文档中的项目名、启动命令、定位描述，并强调“当前支持 QQ，后续支持多平台”。

- [ ] **Step 4: 重新运行文档检查和全量测试**

Run: `rg -n "codecli-channels" README* CONTRIBUTING.md docs && go test ./...`
Expected: 文档检查仅剩必要上下文引用或无结果；测试 PASS

- [ ] **Step 5: 提交当前小步**

```bash
git add README.md README.zh-CN.md CONTRIBUTING.md docs
git commit -m "docs: rename project to codecli-channels"
```

## 备注

- 本计划形成时不做 `internal/qq` 架构迁移；该迁移已在后续多平台内核重构中完成
- 若旧配置名和旧示例文件保留，需要在 README 中明确说明其兼容身份
- 所有验证命令统一使用仓库内 `.gocache/.gomodcache`
