# Three Permission Tiers

Edit each SSH target or database instance in `ssh-mcp manage`. Choose **Operation permission** with Left/Right and verify/save the configuration. This is local operator configuration; MCP tools cannot change it or approve requests.

| Tier | Recognized reads | Writes, scripts, unknown commands, elevation | Deployment |
| --- | --- | --- | --- |
| `read_only` | Automatic | Rejected before dispatch | Rejected |
| `confirm` (default) | Automatic | Local TUI approval for each operation | Approval bound to source SHA-256 and start action |
| `full` | Automatic | No per-operation approval | No per-operation approval |

New and migrated targets default to `confirm`, never `full`. Changing permissions increments the target revision and invalidates older sessions and approvals.

## Local Confirmation

The daemon validates the request and opens the local management console. Review the target, revision, exact command/SQL, root escalation, session directory/environment, time/output limits, and deployment details. Deny is selected by default. Use Up/Down to scroll, Tab to select, Enter to submit, or Escape to deny. Allow requires scrolling to the final page. Terminal escape and bidirectional control characters are displayed as escaped text.

Decisions apply once to the waiting request, expire after two minutes or the caller's earlier deadline, and are never reusable by another tool call. Denial, cancellation, console disconnect, owner process exit, vault lock and expiry prevent dispatch. If a terminal cannot open, the request fails closed; manually open `ssh-mcp manage` and initiate a new request. Configuration and unlock state are checked again after approval and by the transport before dispatch.

Deployment approvals bind to a private local snapshot and SHA-256, so editing the original file after review cannot change the uploaded bytes. Snapshots are removed on completion; a daemon crash may leave a sensitive temporary snapshot behind.

MCP exposes no approval tool or `confirmed=true` field. Agents must not operate the console on behalf of the user or bypass a refusal using another command, tool or system SSH.

## Boundaries

Read-only SSH uses a conservative allowlist of common diagnostics such as `ls`, `cat`, `head`, `tail`, `grep`, `stat`, `wc`, `free`, `df`, `du`, `ps`, restricted `systemctl ... --no-pager`, and restricted `journalctl --no-pager`. Plain pipelines and conditional combinations of recognized reads are accepted. Redirects, assignments, expansions, custom environments, scripts, unknown syntax and elevation require `confirm/full`. SQL classification and read-only database transactions remain in effect; writes also require configured write credentials.

All tiers retain target registration/enabled state, vault unlock, SSH fingerprint checks, target command blacklists, file API switches, recognized destructive hard stops, budgets and auditing. `read_only` retains `opaque_shell_effect`; `confirm/full` allow scripts and interpreters while retaining the other recognized hard stops. Static analysis cannot prove the effects inside arbitrary scripts. These tiers are not an OS sandbox; remote account permissions, server configuration and trusted executable paths still matter.

## Codex Approval

Native Codex approval is a separate layer. Metadata tools and `read_ssh_file` publish `readOnlyHint=true`; generic SSH, SQL and deployment tools remain potentially modifying. In the existing `[mcp_servers.ssh-mcp]` table you may set:

```toml
default_tools_approval_mode = "writes"
tool_timeout_sec = 1200
```

`writes` requests client approval for tools not marked read-only; `prompt` requests approval for every call. The effective reviewer also depends on Codex global settings and organizational policy. Native approval is not guaranteed to be a human decision and cannot replace daemon-local confirmation. Prompt instructions alone do not authorize execution. Do not add a duplicate TOML table. See the [Codex configuration reference](https://developers.openai.com/codex/config-reference/).
