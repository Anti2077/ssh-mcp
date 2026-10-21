# ssh-mcp

A local stdio MCP server for running commands, inspecting logs, reading files, and querying databases on targets you register locally.

[中文](README.md) · [Quickstart](docs/en/quickstart.md) · [Usage guide (Chinese)](docs/usage.md) · [Tool reference](docs/mcp-tools.md)

This is a community fork of [lswzw/ssh-mcp](https://github.com/lswzw/ssh-mcp), retaining its history and GPL-3.0 license. Thanks to the upstream author and contributors.

**Source preview: this fork does not yet have prebuilt releases.** Upstream release binaries do not include these additions.

## Changes in this fork

- SSH password and local private-key authentication, including encrypted OpenSSH / PEM keys.
- Local passphrase entry; unlocked signers remain in daemon memory and are cleared when the vault locks or the daemon stops.
- Chinese / English keyboard console with vertical action menus and clear radio selections.
- A macOS Unix socket path fallback for long installation directories.
- Protection against overlapping verification requests and background polling obscuring save results.

The upstream SSH / SFTP, file deployment, MySQL / MariaDB / PostgreSQL tools, target registration, fingerprint pinning and command policy remain available. Policy rules do not guarantee that every allowed command is safe.

## Build and install

Requires Git, Go (minimum declared in [go.mod](go.mod), currently 1.26.5) and a client that can launch local stdio MCP servers. Local platforms: macOS, Linux, Windows. Remote SSH commands use Linux semantics.

macOS / Linux:

~~~bash
git clone https://github.com/Anti2077/ssh-mcp.git
cd ssh-mcp
make build
mkdir -p "$HOME/.local/share/ssh-mcp"
chmod 700 "$HOME/.local/share/ssh-mcp"
install -m 700 ./bin/ssh-mcp "$HOME/.local/share/ssh-mcp/ssh-mcp"
"$HOME/.local/share/ssh-mcp/ssh-mcp" manage
~~~

`make build` builds for the current OS and architecture. `install -m 700` copies the executable with owner-only access. The database and audit files live beside the executable: choose a stable, private directory before registering targets. See [migration instructions](docs/en/installation.md) for an existing installation.

Windows PowerShell:

~~~powershell
git clone https://github.com/Anti2077/ssh-mcp.git
cd ssh-mcp
go build -o bin/ssh-mcp.exe ./cmd/ssh-mcp
$installDir = Join-Path $env:LOCALAPPDATA 'ssh-mcp'
New-Item -ItemType Directory -Force $installDir | Out-Null
Copy-Item .\bin\ssh-mcp.exe (Join-Path $installDir 'ssh-mcp.exe')
& (Join-Path $installDir 'ssh-mcp.exe') manage
~~~

## Register a target

1. Set the master password or unlock the vault in the local console.
2. Open target management. `Tab` to the action menu, select Add SSH with Up/Down, then `Enter`.
3. `Enter` the IP, port and username. Choose password or private-key authentication; key paths accept ~/... or an absolute path.
4. Choose Verify and save. `Enter` an encrypted key's passphrase locally when requested.
5. Check the SSH host fingerprint against a trusted source before confirming trust. Wait for the explicit saved message.

`Tab` switches between the target list and vertical action menu; Up/Down moves within a group; `Enter` executes; `Esc` goes back. In forms, `Tab / Shift+Tab` moves between fields and Left/Right changes radio selections. [Private-key guide](docs/en/private-key-auth.md).

## Configure an MCP client

Use the executable's absolute path and the `serve` argument:

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

Client-specific wrapper fields may vary. `serve` connects to or starts the local daemon; `manage` opens the interactive console. There is no HTTP listener.

Codex CLI, on macOS / Linux:

~~~bash
codex mcp add ssh-mcp -- "$HOME/.local/share/ssh-mcp/ssh-mcp" serve
codex mcp get ssh-mcp
~~~

-- separates Codex options from the executable and its arguments. Reload the client or start a new session, inspect the tool list and call `list_targets` to confirm the connection. A configuration listing alone is not a successful tool call. See the [official Codex MCP documentation](https://developers.openai.com/codex/mcp).

## Example requests

~~~text
Use ssh-mcp to list registered targets and capabilities. On my selected Linux
test server, inspect load, memory and disk using read-only commands.
~~~

~~~text
Use ssh-mcp to fetch the last 200 lines of example.service logs. Summarize
startup errors, exit codes and truncated output. Do not restart services.
~~~

Discover targets with `list_targets` / `describe_target_capability`, then use `run_ssh`, `read_ssh_file`, `run_sql` or the session tools. Consult the [tool reference](docs/mcp-tools.md) for exact schemas.

## Limits and troubleshooting

- Targets currently use IP addresses. SSH config aliases, ssh-agent, hardware keys and interactive TTYs are not supported.
- Keep passwords, passphrases and host trust decisions in the local console, outside AI conversations.
- For `unlock_required` / `ssh_key_unlock_required`, run `manage` from the same installation, unlock locally, then retry. Requests are not automatically replayed.
- `status` inspects daemon state; `stop` stops an idle daemon. Close active client sessions first.
- macOS automatically launches Terminal.app by default. Custom launchers use `SSH_MCP_TERMINAL`; changes require a daemon restart. See [installation](docs/en/installation.md).

## Documentation and license

[Documentation index](docs/en/README.md) · [Configuration](docs/en/configuration.md) · [Security model](docs/en/security-model.md) · [Contributing](CONTRIBUTING.md)

Derived from [lswzw/ssh-mcp](https://github.com/lswzw/ssh-mcp). Original author and contributor history is retained. Licensed under [GNU GPL v3.0](LICENSE); dependencies retain their respective licenses.
