# Private-key login and keyboard menus

This fork adds SSH private-key authentication and keyboard menus. Use Up/Down or Tab to select a dashboard action and Enter to open it. In Targets, use Up/Down for targets and Tab or Left/Right for actions. Forms expose Verify and save / Cancel buttons reachable with Tab and Shift+Tab. Esc goes back. Legacy letter shortcuts remain compatible but are optional.

For SSH, select `private_key` with Left/Right in Authentication and enter an absolute local key path or `~/.ssh/id_ed25519`. OpenSSH and PEM keys supported by the Go SSH library may be encrypted or unencrypted. The local daemon reads the key file; only its path is stored as local metadata. Key files and passphrases are not stored in the database, backups, audit logs, or MCP responses. Back up key files separately.

An encrypted key opens the local terminal console, or reuses an existing console, for the user to enter its passphrase. The passphrase is used only for local parsing. The unlocked signer is cached only in daemon memory and discarded on lock, key maintenance, or exit. Changed key files require unlocking again. MCP returns `unlock_required` / `ssh_key_unlock_required` with `execution_outcome=not_dispatched`; submit a new request after unlocking. Commands are never automatically replayed. If no console appears, run `ssh-mcp manage` from the same installation. Host fingerprints still require manual verification; Back is selected by default.

IP addresses and Linux remote semantics remain required. ssh-agent, hardware keys, and aliases from `~/.ssh/config` are not supported.
