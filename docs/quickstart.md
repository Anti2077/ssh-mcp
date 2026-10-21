# 快速开始

本分支目前从源码安装，尚无预编译 Release。上游发行文件不含本分支新增功能。

## 1. 编译并固定安装路径

需要 Git、Go（最低版本见 go.mod，当前 1.26.5）及支持本地 stdio MCP 的客户端。macOS / Linux：

~~~bash
git clone https://github.com/Anti2077/ssh-mcp.git
cd ssh-mcp
make build
mkdir -p "$HOME/.local/share/ssh-mcp"
chmod 700 "$HOME/.local/share/ssh-mcp"
install -m 700 ./bin/ssh-mcp "$HOME/.local/share/ssh-mcp/ssh-mcp"
~~~

`make build` 编译本机版本；`install -m 700` 复制到稳定目录并设置仅当前用户可访问。Windows PowerShell 步骤见[项目首页](../README.md#安装)。

程序将数据库和审计日志放在可执行文件旁边，不同安装目录有不同的目标库。已有安装请先看[备份与迁移](installation.md#文件位置)，不要直接覆盖正在运行的程序。

## 2. 登记目标

~~~bash
"$HOME/.local/share/ssh-mcp/ssh-mcp" manage
~~~

`manage` 连接或启动本地 daemon，进入交互控制台。在终端内：

1. 设置主密码或解锁凭据库。
2. 进入“管理服务器与数据库”，`Tab` 切到操作菜单，`↑↓` 选“新增 SSH”或“新增数据库”，`Enter` 打开。
3. 填写目标。SSH 使用 IP、端口和用户名；`←→` 选择密码或私钥认证。私钥填写绝对路径或 `~/.ssh/id_ed25519`。
4. `Tab` 到“验证并保存”，`Enter` 提交；加密私钥的口令在本地输入。
5. 核对 SSH 指纹后，`↑↓` 选择“信任并验证连接”，`Enter` 确认。默认选“返回”。等待明确的保存成功提示。

`Tab` 在目标列表和纵向操作菜单间切换，`↑↓` 在当前组移动；表单用 `Tab / Shift+Tab` 切字段，`←→` 调整单选项，`Esc` 返回。[私钥说明](private-key-auth.md)。

## 3. 注册 MCP

Codex CLI 示例：

~~~bash
codex mcp add ssh-mcp -- "$HOME/.local/share/ssh-mcp/ssh-mcp" serve
codex mcp get ssh-mcp
~~~

add 注册服务，-- 后为可执行文件及参数，`serve` 是 stdio bridge；`get` 查看配置。其他客户端填写同一绝对路径与参数 `serve`，具体配置格式见[项目首页](../README.md#接入-mcp-客户端)。

重新加载客户端或开启新会话后，让 AI 列出 ssh-mcp 工具并调用 `list_targets`；有配置不等于工具连接已验证。

## 4. 第一次请求

~~~text
使用 ssh-mcp 列出已登记目标，再检查我指定测试服务器的负载、
内存和磁盘。先查看目标能力，只读排查，不修改配置或重启服务。
~~~

推荐先调用 `list_targets`、`describe_target_capability`，再调用 `run_ssh` 或 `read_ssh_file`。详见[使用指南](usage.md)。

## 5. 状态与停止

~~~bash
"$HOME/.local/share/ssh-mcp/ssh-mcp" status
"$HOME/.local/share/ssh-mcp/ssh-mcp" stop
~~~

`status` 查看 daemon、锁定和 bridge 状态，`stop` 停止空闲 daemon。若存在活动客户端连接，先关闭客户端会话再停止。

## 常见问题

| 现象 | 处理 |
| --- | --- |
| `unlock_required` / `ssh_key_unlock_required` | 用同一安装目录的 `manage` 在本地解锁，再重新提交请求。 |
| 目标未找到 | 检查可执行文件路径是否一致，以及目标是否登记并启用。 |
| 指纹变化 | 先核实服务器身份，再在本地控制台重新验证。 |
| `manage` 无界面 | 在交互终端运行。需要自定义终端时配置 `SSH_MCP_TERMINAL`，并先停止旧 daemon。 |
| 其他对话没有工具 | 重新加载客户端或新建会话，确认使用同一 MCP 配置；工具注册由客户端管理。 |

密码、私钥、口令和指纹确认由用户在本地完成，不要贴进 AI 对话。更多限制见[安装与运行](installation.md)和[安全模型](security-model.md)。
