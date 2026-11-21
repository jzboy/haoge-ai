<!-- intro -->
**浩哥AI** 是一个本地 AI 网关与 Agent 模型管理工具：在一个界面里为每个 Agent 选模型，由一个本地网关统一转发，账号额度用完时自动切换到下一个账号。

- **五个 Agent**：Claude Code、Claude Desktop、Codex、WorkBuddy、DeepSeek Harness。
- **四家供应商 + ZCode**：DeepSeek、OpenAI（ChatGPT 订阅 / API Key）、WorkBuddy、Qoder 中国版、ZCode（GLM Coding Plan），社区插件开箱即用。
- **路由与额度**：Smart 等路由策略、路由组、账号轮换、用量账本与配额卡，一处看清、一处调整。
- **两种产物**：Windows `haoge-ai-Windows-amd64.exe`（WebView2，无需额外依赖）、macOS `haoge-ai-macos-arm64.dmg`（Apple 芯片，未签名，首次打开请右键 → 打开）。
<!-- /intro -->

## haoge-v1.8

- 导航精简：移除「路由」「网关」「用量」三个页面入口，以及托盘面板里的「用量（统计）」和「路由」标签。网关、用量记录和路由策略仍在后台照常工作。
- 恢复「资源库」：只提供 Claude 插件、RTK、billion-context 三项，其余分类不再显示。

## haoge-v1.7

- 安装后的程序名与图标：Windows 安装到 `%LOCALAPPDATA%\Programs\haogeai\haogeai.exe`，开始菜单显示「浩哥AI」；macOS 包内可执行文件为 `haogeai`。以前的 magpie 安装（含开始菜单、卸载项、开机启动）会在首次运行时自动清理。
- 发布说明改为固定中文介绍加本次更新内容。
- 安装包更名为 `haoge-ai-Windows-amd64.exe` 与 `haoge-ai-macos-arm64.dmg`。
- 界面视觉打磨：卡片统一圆角与柔和投影、弹层浮起、页面顶部淡光晕。
- 修复：用量图表在缺少模型标识时崩溃、插件内置列表在异常数据下崩溃、Qoder 中国版的编辑器缺少插件签到分组。
- 修复：窄窗口下用量页与额度标题的横向溢出。
