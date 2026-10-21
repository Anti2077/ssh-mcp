# ssh-mcp

让 Codex 等支持本地 stdio MCP 的 AI 客户端连接你登记的 Linux 服务器，执行命令、查看日志、读取文件和查询数据库。

[English](README_EN.md) · [快速开始](docs/quickstart.md) · [使用指南](docs/usage.md) · [工具参考](docs/mcp-tools.md)

> 本仓库是 [lswzw/ssh-mcp](https://github.com/lswzw/ssh-mcp) 的社区分支，保留上游历史与 GPL-3.0 许可证，感谢原作者的工作。此分支增加私钥登录和键盘菜单，并修复 macOS 长路径与保存反馈问题。
>
> **当前为源码预览版，尚未发布本分支的预编译 Release。** 请按下面步骤编译；上游发行文件不包含本分支新增功能。

## 功能

- **SSH 运维**：执行非交互命令、检查服务状态、读取日志；支持密码和本地私钥登录。
- **加密私钥**：支持 OpenSSH / PEM 格式；口令在本机控制台输入，解锁后的签名器仅缓存于 daemon 内存。
- **本地管理**：中文 / 英文终端界面，纵向操作菜单、清晰的选中标记和单选项。
- **文件与数据库**：SFTP 文件读取、校验并部署文件，MySQL / MariaDB / PostgreSQL 查询。
- **访问边界**：目标须本地登记并启用，SSH 主机指纹须人工核对，凭据库由主密码保护。
- **macOS 修复**：长安装路径自动缩短 Unix socket 路径；修复重复确认和后台刷新覆盖保存结果的问题。

`serve` 是客户端启动的 stdio bridge，按需连接或启动本地 daemon；`manage` 打开用户操作的终端控制台。daemon 管理凭据和远端连接，程序不提供 HTTP 服务端口。

~~~text
AI 客户端 ── MCP / stdio ── ssh-mcp serve ── 本地 daemon ── SSH / SFTP / 数据库
                                                 ↑
                                          ssh-mcp manage
~~~

## 安装

需要 Git、Go（最低版本见 [go.mod](go.mod)，当前为 1.26.5）以及支持本地 stdio MCP 的客户端。本地支持 macOS、Linux 和 Windows；远端 SSH 命令按 Linux 语义执行。

### macOS / Linux

~~~bash
git clone https://github.com/Anti2077/ssh-mcp.git
cd ssh-mcp
make build
mkdir -p "$HOME/.local/share/ssh-mcp"
chmod 700 "$HOME/.local/share/ssh-mcp"
install -m 700 ./bin/ssh-mcp "$HOME/.local/share/ssh-mcp/ssh-mcp"
"$HOME/.local/share/ssh-mcp/ssh-mcp" manage
~~~

`make build` 编译当前系统和架构的程序；`mkdir -p` 创建稳定安装目录，`chmod 700` 仅允许当前用户访问，`install -m 700` 复制程序并设置执行权限。`manage` 进入本地管理控制台。

状态数据库和审计日志保存在**可执行文件旁边**，应先固定安装路径再登记服务器。已有安装不要直接移动：先停止 daemon 并按[迁移说明](docs/installation.md#文件位置)备份。

### Windows PowerShell

安装 Git 和 Go 后执行：

~~~powershell
git clone https://github.com/Anti2077/ssh-mcp.git
cd ssh-mcp
go build -o bin/ssh-mcp.exe ./cmd/ssh-mcp
$installDir = Join-Path $env:LOCALAPPDATA 'ssh-mcp'
New-Item -ItemType Directory -Force $installDir | Out-Null
Copy-Item .\bin\ssh-mcp.exe (Join-Path $installDir 'ssh-mcp.exe')
& (Join-Path $installDir 'ssh-mcp.exe') manage
~~~

`go build -o` 指定输出文件；程序固定放到当前用户本地应用数据目录，MCP 配置使用该文件的完整绝对路径。

## 首次配置

1. 在控制台设置主密码或解锁凭据库。
2. 进入“管理服务器与数据库”，`Tab` 切到操作菜单，用 `↑↓` 选择“新增 SSH”，`Enter` 打开。
3. 填写服务器 IP、端口、用户名；选择“密码登录”或“私钥登录”。私钥路径可用 `~/.ssh/id_ed25519`。
4. 选择“验证并保存”；加密私钥的口令只在本机输入。
5. 与可信渠道核对服务器主机指纹，再选择信任并验证。看到“目标验证通过，配置已保存”后完成登记。

目标列表和纵向操作菜单之间用 `Tab` 切换焦点，组内用 `↑↓` 移动，`Enter` 执行，`Esc` 返回。表单用 `Tab / Shift+Tab` 切换字段，`←→` 调整单选项。详见[私钥认证与菜单操作](docs/private-key-auth.md)。

## 接入 MCP 客户端

通用 stdio 配置示例（command 替换为真实绝对路径）：

~~~json
{
  "mcpServers": {
    "ssh-mcp": {
      "command": "/absolute/path/to/ssh-mcp",
      "args": ["serve"]
    }
  }
}
~~~

不同客户端的外层字段可能不同；必需信息是本地可执行文件路径和参数 `serve`。只有支持启动本地 stdio 进程的客户端可以接入。

### Codex CLI

~~~bash
codex mcp add ssh-mcp -- "$HOME/.local/share/ssh-mcp/ssh-mcp" serve
codex mcp get ssh-mcp
~~~

`add ssh-mcp` 注册这个名称；-- 后是程序路径及其参数；`get` 查看注册配置。重新加载客户端或开启新会话，再要求列出 ssh-mcp 工具并调用 `list_targets`；仅看到注册配置不代表连接验证完成。命令格式参考 [Codex 官方 MCP 文档](https://developers.openai.com/codex/mcp)。

## 让 AI 做什么

~~~text
使用 ssh-mcp 列出已登记目标和能力，再检查我指定测试服务器的
系统负载、内存和磁盘。仅执行只读命令，不重启服务、不修改配置。
~~~

~~~text
使用 ssh-mcp 查看测试服务器 example.service 最近 200 行日志，
定位启动失败原因。限制输出长度，报告命令、退出码和关键错误。
~~~

先用 `list_targets` / `describe_target_capability` 确认目标，再使用 `run_ssh`、`read_ssh_file` 等工具。更多[日志、测试与数据库示例](docs/usage.md)见使用指南。

## 限制与维护

- 远端目标当前按 IP 登记；不支持 `~/.ssh/config` 别名、ssh-agent、硬件密钥或交互式 TTY。
- 主密码、SSH 密码、私钥口令和主机指纹确认由用户在本地完成，不要将凭据贴入 AI 对话。
- 命令与 SQL 有固定高危类别拦截，但不代表任意命令均安全；部署和测试命令仍可能修改远端状态。
- `unlock_required` / `ssh_key_unlock_required`：在同一安装路径运行 `manage` 解锁，再重新发起操作；请求不会自动重放。
- `status` 查看 daemon 状态，`stop` 停止空闲 daemon。有活动 bridge 时先关闭客户端会话。
- macOS 默认自动打开系统 Terminal；可配置 `SSH_MCP_TERMINAL`，更改后需重启 daemon。见[安装与运行](docs/installation.md)。

## 更多文档

[文档目录](docs/README.md) · [安装与迁移](docs/installation.md) · [目标配置](docs/configuration.md) · [MCP 工具](docs/mcp-tools.md) · [安全模型](docs/security-model.md) · [日常维护](docs/operations.md) · [贡献指南](CONTRIBUTING.md)

## 许可证与致谢

基于 [lswzw/ssh-mcp](https://github.com/lswzw/ssh-mcp) 继续开发，原始作者与贡献记录保留在 Git 历史中。整体按 [GNU GPL v3.0](LICENSE) 发布，第三方依赖遵循各自许可证。
