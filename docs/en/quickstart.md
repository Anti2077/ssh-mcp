# Quickstart

This fork is a source preview without prebuilt releases. Upstream binaries do not include this fork's additions.

## 1. Build and install

Requires Git, Go (minimum in go.mod, currently 1.26.5), an interactive terminal and a client that launches local stdio MCP servers. macOS / Linux:

~~~bash
git clone https://github.com/Anti2077/ssh-mcp.git
cd ssh-mcp
make build
mkdir -p "$HOME/.local/share/ssh-mcp"
chmod 700 "$HOME/.local/share/ssh-mcp"
install -m 700 ./bin/ssh-mcp "$HOME/.local/share/ssh-mcp/ssh-mcp"
~~~

`make build` targets your local OS and architecture. `install -m 700` copies the binary with owner-only permissions. Windows PowerShell instructions are in the [README](../../README_EN.md#build-and-install).

State and audit files live beside the executable. Choose a stable directory before registering targets. Follow the [migration guide](installation.md) for existing installations; do not overwrite a running executable.

## 2. Register a target

~~~bash
"$HOME/.local/share/ssh-mcp/ssh-mcp" manage
~~~

1. Set the master password or unlock the vault.
2. Open target management. `Tab` to the action menu, Up/Down to Add SSH or Add database, then `Enter`.
3. `Enter` the IP, port and username. Choose password or private-key authentication with Left/Right. Key paths accept an absolute path or ~/... .
4. `Tab` to Verify and save, then `Enter`. `Enter` an encrypted key's passphrase locally if requested.
5. Check the SSH fingerprint against a trusted source before trusting it. Up/Down selects the confirmation action; Back is selected by default. Wait for the saved message.

`Tab` switches between the target list and vertical menu, Up/Down moves within a group and `Enter` executes. Forms use `Tab / Shift+Tab` for fields and Left/Right for radio selections. `Esc` goes back. [Private-key guide](private-key-auth.md).

## 3. Register the MCP server

Codex CLI example:

~~~bash
codex mcp add ssh-mcp -- "$HOME/.local/share/ssh-mcp/ssh-mcp" serve
codex mcp get ssh-mcp
~~~

add registers the server; -- separates client options from the executable and its `serve` argument. `get` lists configuration. Other clients use the same absolute path and argument with their own configuration format; see the [README](../../README_EN.md#configure-an-mcp-client).

Reload the client or start a new session, inspect the tool list and call `list_targets`. A configuration listing alone does not verify a successful connection.

## 4. First request

~~~text
Use ssh-mcp to list registered targets and describe their capabilities.
On my selected Linux test server, inspect load, memory and disk using
read-only commands. Do not modify configuration or restart services.
~~~

Use `list_targets` and `describe_target_capability` before `run_ssh` or `read_ssh_file`. See the [tool reference](mcp-tools.md) for schemas.

## 5. Status and shutdown

~~~bash
"$HOME/.local/share/ssh-mcp/ssh-mcp" status
"$HOME/.local/share/ssh-mcp/ssh-mcp" stop
~~~

`status` inspects the daemon and vault state; `stop` stops an idle daemon. Close active client sessions first.

For `unlock_required` / `ssh_key_unlock_required`, run `manage` from the same installation, unlock locally and retry. Target configuration and host trust are user-controlled. Keep passwords and key passphrases outside AI conversations. For terminal launchers, updates and troubleshooting, see [installation](installation.md).
