# Models Command Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Models/Thinking 相关的启动状态、缓存刷新职责、以及 model 字符串尾部 CRLF 导致的展示空行问题。

**Architecture:** 程序启动阶段负责后台刷新 `models.dev` 缓存，TUI/Models 命令只读取本地缓存并渲染，不再在打开 Models palette 时触发网络刷新。思考等级优先使用已缓存的模型元数据，同时对未加载 providerModels 的启动窗口提供基于当前 provider/model 的本地推断兜底，避免误报不支持 thinking levels。

**Tech Stack:** Go, Cobra, Bubble Tea, 标准库 `os`/`time`/`context`, 现有 `internal/llm` / `internal/tui` / `internal/config` 包。

---

### Task 1: 程序启动时后台刷新 models 缓存

**Files:**
- Modify: `cmd/root.go`
- Modify: `internal/llm/modelsdev.go`
- Test: `cmd/root_test.go`
- Test: `internal/llm/modelsdev_test.go`

- [ ] **Step 1: 先写失败测试，锁定“prepare 触发后台刷新 hook”**

在 `cmd/root_test.go` 新增测试，注入一个可观测的 refresh hook，断言 `prepare()` 会调用它一次且不阻塞主流程：

```go
func TestPrepareStartsModelsCatalogRefresh(t *testing.T) {
	oldStart := startModelsCatalogRefresh
	oldVerbose := verbose
	oldLogger := logger
	oldCfg := cfg
	t.Cleanup(func() {
		startModelsCatalogRefresh = oldStart
		verbose = oldVerbose
		logger = oldLogger
		cfg = oldCfg
	})

	called := 0
	startModelsCatalogRefresh = func() { called++ }
	verbose = false

	if err := prepare(); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected one refresh startup call, got %d", called)
	}
}
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./cmd -run TestPrepareStartsModelsCatalogRefresh -count=1`

Expected: FAIL，原因是 `startModelsCatalogRefresh` 还不存在或 `prepare()` 没调用。

- [ ] **Step 3: 实现最小启动 hook**

在 `cmd/root.go` 增加一个可测试的包级变量，并在 `prepare()` 的 config/dirs 初始化成功后调用它：

```go
var startModelsCatalogRefresh = func() {
	llm.StartModelsCatalogRefresh()
}

func prepare() error {
	// existing logger/config/dir setup...
	startModelsCatalogRefresh()
	return nil
}
```

在 `internal/llm/modelsdev.go` 增加异步刷新入口 `StartModelsCatalogRefresh()`，要求：只做后台 best-effort refresh，不影响启动返回值。

- [ ] **Step 4: 补缓存刷新语义测试**

在 `internal/llm/modelsdev_test.go` 增加两类测试：

```go
func TestStartModelsCatalogRefreshSkipsFreshCacheYoungerThan48Hours(t *testing.T) {}
func TestStartModelsCatalogRefreshRefreshesCacheOlderThan48Hours(t *testing.T) {}
```

测试语义：
- cache age <= 48h 时，不发网络请求。
- cache age > 48h 且 < 72h 时，后台刷新写回新 cache。
- cache age > 72h 时，旧 cache 仍可被读取用于 UI 展示，但后台刷新会尽快更新文件。

- [ ] **Step 5: 运行 Task 1 测试确认 GREEN**

Run: `go test ./cmd ./internal/llm -count=1`

Expected: PASS。

- [ ] **Step 6: 提交 Task 1**

Run:
```bash
git add cmd/root.go cmd/root_test.go internal/llm/modelsdev.go internal/llm/modelsdev_test.go
git commit -m "fix: refresh model catalog cache at startup"
```

---

### Task 2: Models palette 仅读缓存，首屏 thinking level 不再误报

**Files:**
- Modify: `internal/tui/providers.go`
- Modify: `internal/tui/provider_service.go`
- Modify: `internal/tui/app.go`
- Test: `internal/tui/providers_test.go`
- Test: `internal/tui/provider_service_test.go`

- [ ] **Step 1: 先写失败测试，覆盖“未打开 Models palette 前也能 cycle think level”**

在 `internal/tui/providers_test.go` 新增测试：仅配置当前 provider/model，不预填 `providerModels`，调用 `cycleThinkLevel()` 后不应得到 `Current model does not advertise thinking levels.`，并且能按推断等级切换。

```go
func TestCycleThinkLevelFallsBackToCurrentConfiguredModelMetadata(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = config.ProviderOpenAI
	cfg.LLM.OpenAI.Model = "gpt-5"
	cfg.LLM.OpenAI.ThinkingLevel = "medium"
	app := New(nil, cfg, nil)

	app.cycleThinkLevel()

	if app.statusMsg == "Current model does not advertise thinking levels." {
		t.Fatalf("expected current model fallback metadata to avoid no-thinking warning")
	}
	if app.thinkLevel == ThinkMedium {
		t.Fatalf("expected thinking level to advance from medium")
	}
}
```

- [ ] **Step 2: 先写失败测试，锁定 Models palette 不再触发刷新逻辑**

在 `internal/tui/providers_test.go` 新增测试：`beginModelsPaletteLoad()` 只发起“读取本地缓存”的加载命令，不再按 provider 标记多个 `providerModelLoading[provider]=true` 的 UI loading 状态。若需要 loading 态，仅保留 palette 级别单一状态位，例如 `modelsPaletteLoading`。

- [ ] **Step 3: 运行 TUI 测试确认 RED**

Run: `go test ./internal/tui -run 'TestCycleThinkLevelFallsBackToCurrentConfiguredModelMetadata|TestModelPalette' -count=1`

Expected: FAIL。

- [ ] **Step 4: 实现 provider/model fallback 和 palette 级 loading**

实现要点：
- `providerService.CurrentModelDescriptor()` 在 `providerModels` 未命中时，基于当前 provider kind + configured model 构造一个 `llm.ModelDescriptor` 兜底，thinking metadata 用 `llm.ThinkingLevelsForModel()` / `llm.DefaultThinkingLevel()` 推断。
- `App.currentThinkingLevels()` 优先走真实缓存元数据，fallback descriptor 仅用于启动窗口。
- `beginModelsPaletteLoad()` 不再设置每 provider loading 文案；Models palette 顶层如需等待只展示单一 `Loading models...` 行。
- `providerModelItems()` 删除 provider 级 loading 分支，只负责渲染已有缓存数据或错误/空列表。

- [ ] **Step 5: 运行 Task 2 测试确认 GREEN**

Run: `go test ./internal/tui -count=1`

Expected: PASS。

- [ ] **Step 6: 提交 Task 2**

Run:
```bash
git add internal/tui/app.go internal/tui/providers.go internal/tui/provider_service.go internal/tui/providers_test.go internal/tui/provider_service_test.go
git commit -m "fix: use cached model metadata in models palette"
```

---

### Task 3: 清洗 model/base URL 文本，去掉尾部 CRLF 导致的空行

**Files:**
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/provider_service.go`
- Test: `internal/tui/commands_test.go`
- Test: `internal/tui/provider_service_test.go`

- [ ] **Step 1: 先写失败测试，复现 `/models` 输出空行**

在 `internal/tui/commands_test.go` 新增测试：构造 `Model="gpt-5\r\n"` 和 `BaseURL="https://example.com/v1\r\n"`，`renderModelsText()` 输出中不应出现被 CRLF 撑开的空行，且字段值应被 trim。

```go
func TestRenderModelsTextTrimsModelAndBaseURL(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DefaultProvider = config.ProviderOpenAI
	cfg.OpenAI.Model = "gpt-5\r\n"
	cfg.OpenAI.BaseURL = "https://api.openai.com/v1\r\n"

	got := renderModelsText(&cfg.LLM)

	if strings.Contains(got, "\r") {
		t.Fatalf("expected CR characters to be trimmed, got %q", got)
	}
	if !strings.Contains(got, "Model:   gpt-5\n") {
		t.Fatalf("expected trimmed model, got %q", got)
	}
	if !strings.Contains(got, "URL:     https://api.openai.com/v1\n") {
		t.Fatalf("expected trimmed base URL, got %q", got)
	}
}
```

- [ ] **Step 2: 先写失败测试，复现保存 model 时带 CRLF**

在 `internal/tui/provider_service_test.go` 新增测试：`SelectModel(config.ProviderOpenAI, "gpt-5\r\n")` 后，配置中持久化值应为 `gpt-5`。

- [ ] **Step 3: 运行测试确认 RED**

Run: `go test ./internal/tui -run 'TestRenderModelsTextTrimsModelAndBaseURL|TestProviderServiceSelectModel' -count=1`

Expected: 至少新增用例 FAIL。

- [ ] **Step 4: 实现统一 TrimSpace**

改动点：
- `renderModelsText()` 对 `providerCfg.Model`、`providerCfg.BaseURL`、`providerCfg.ResolvedAPIKey()` 的派生展示值统一 `strings.TrimSpace()`。
- `providerService.SelectModel()` 已有 trim，补测试锁回归即可。
- 如果发现还有其它渲染入口直接使用 `providerCfg.Model`，同步加 trim，但不要做无关重构。

- [ ] **Step 5: 运行 Task 3 测试确认 GREEN**

Run: `go test ./internal/tui -count=1`

Expected: PASS。

- [ ] **Step 6: 提交 Task 3**

Run:
```bash
git add internal/tui/commands.go internal/tui/commands_test.go internal/tui/provider_service.go internal/tui/provider_service_test.go
git commit -m "fix: trim model text in models output"
```

---

### Task 4: 状态栏提示自动消失

**Files:**
- Modify: `internal/tui/app.go`
- Test: `internal/tui/layout_test.go`

- [ ] **Step 1: 先写失败测试，复现普通 status 文案不会自动消失**

在 `internal/tui/layout_test.go` 新增测试：设置一条普通 `statusMsg`，模拟超时消息到达后应清空状态栏；如果在超时前又设置了新文案，旧定时器不能清掉新文案。

```go
func TestStatusMessageExpiresAndIgnoresStaleTimeout(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)

	cmd := app.setTransientStatus("Thinking level set to High.")
	if cmd == nil {
		t.Fatalf("expected transient status timeout command")
	}
	if app.statusMsg == "" {
		t.Fatalf("expected status message to be set")
	}

	msg := cmd().(statusMsgExpiredMsg)
	app.statusMsg = "Model switched to gpt-5 (OpenAI)."
	updated, _ := app.Update(msg)
	got := updated.(App)
	if got.statusMsg == "" {
		t.Fatalf("expected stale timeout not to clear newer status")
	}
}
```

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/tui -run TestStatusMessageExpiresAndIgnoresStaleTimeout -count=1`

Expected: FAIL，原因是还没有统一的 transient status 超时机制。

- [ ] **Step 3: 实现普通状态文案的统一超时清理**

实现要点：
- 在 `App` 里增加一个状态消息序号/token，新增 `statusMsgExpiredMsg`。
- 新增 `setTransientStatus(message string) tea.Cmd` 统一设置普通提示文案，并返回 `tea.Tick` 超时命令，例如 3 秒后发送 `statusMsgExpiredMsg{token: currentToken}`。
- `Update()` 收到 `statusMsgExpiredMsg` 时，只在 token 匹配当前 token 时清空 `statusMsg`，避免旧定时器误删新文案。
- 把这些普通提示改走 `setTransientStatus()`：`Current model does not advertise thinking levels.`, `Thinking level set to ...`, `Model switched to ...`, provider update/create/clear/delete 成功提示，以及通用即时错误提示。
- `Stopping agent...` 这类运行态提示继续保留现有事件驱动清理，不走超时自动消失。

- [ ] **Step 4: 运行 Task 4 测试确认 GREEN**

Run: `go test ./internal/tui -count=1`

Expected: PASS。

- [ ] **Step 5: 提交 Task 4**

Run:
```bash
git add internal/tui/app.go internal/tui/layout_test.go
git commit -m "fix: auto-expire transient status messages"
```

---

### Task 5: 全量验证和收尾

**Files:**
- Modify: touched Go files only, no broad refactor

- [ ] **Step 1: gofmt 所有改动过的 Go 文件**

Run: `gofmt -w cmd/root.go cmd/root_test.go internal/llm/modelsdev.go internal/llm/modelsdev_test.go internal/tui/app.go internal/tui/providers.go internal/tui/provider_service.go internal/tui/providers_test.go internal/tui/provider_service_test.go internal/tui/commands.go internal/tui/commands_test.go`

Expected: 命令成功，无输出。

- [ ] **Step 2: 跑全量测试**

Run: `go test ./...`

Expected: PASS。

- [ ] **Step 3: 手工烟测**

Run:
```bash
go run . --help
go run . tui
```

手工检查：
- 刚启动/重进 TUI 后，当前模型的 thinking level badge 和切换逻辑正常。
- 打开 Models palette 时只读缓存，不再出现每个 provider 各自一条 loading。
- `/models` 输出里的 model/base URL 没有因为 `\r\n` 多出空行。

- [ ] **Step 4: 最终提交**

Run:
```bash
git status --short
git commit -m "fix: stabilize models palette cache and thinking state"
```

---

## Self-Review

- Spec coverage: 三个用户反馈点都已分别落到 Task 1/2/3，Task 4 覆盖格式化和全量验证。
- Placeholder scan: 无 `TBD` / `TODO` / “稍后补充” 类占位描述。
- Type consistency: 计划中新增 hook 命名统一为 `startModelsCatalogRefresh` / `StartModelsCatalogRefresh()`，TUI 侧 fallback 仍通过 `CurrentModelDescriptor()` 与 `currentThinkingLevels()` 串联。
