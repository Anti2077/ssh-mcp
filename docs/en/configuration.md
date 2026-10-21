# Target & Vault Configuration Guide

[English](configuration.md) | [中文说明](../configuration.md)

All targets, passwords, and security boundaries are managed exclusively through the local Terminal User Interface (TUI). MCP clients can only discover non-sensitive target metadata; they cannot add, edit, or delete targets.

---

## 1. SSH Target Configuration

Open Servers and databases from the dashboard. Use Tab to select Add SSH or Edit, then Enter.

| Field | Description |
| :--- | :--- |
| **IP** | Remote IPv4 or IPv6 address. (Domain names/hostnames are currently not accepted to prevent DNS rebinding attacks). |
| **SSH Port** | Target SSH port (default: `22`). |
| **Username / Password** | Authentication credentials. Leave password blank when editing to retain existing encrypted credentials. |
| **Authentication / Private key path** | Choose `password` or `private_key` with Left/Right. Enter an absolute local key path or `~/` path. |
| **Command Blacklist** | Comma-separated Go RE2 regular expressions. Matching commands are blocked immediately. |
| **Description / Environment** | Local metadata for operator context and agent discovery (e.g., `production`, `staging`). |
| **Allow File Operations** | Boolean (`true`/`false`). Controls access to `read_ssh_file` and `deploy_ssh_binary`. Defaults to `true`. |

### Connection Testing & Host Fingerprint Pinning
Upon selecting Verify and save and pressing Enter, `ssh-mcp` immediately tests the connection:
- **First-time connection**: The server presents the remote host's public key fingerprint. You must manually inspect and select Trust and verify and press Enter.
- **Fingerprint mismatch**: If a previously pinned fingerprint changes, the connection is blocked with a security warning. The target is never blindly trusted.
- **Target OS**: Remote target semantics must be Linux.

### Command Blacklist Syntax & Examples
The blacklist accepts Go RE2 regular expressions separated by commas:

```text
rm /data/.*, cat /etc/passwd, passwd.*
```

> [!NOTE]
> The target-level blacklist only adds extra restrictions; it cannot bypass the 11 built-in pre-dispatch guardrails. Leaving this field blank means relying solely on the built-in system guardrails. Updating a blacklist automatically invalidates active SSH working sessions for that target.

---

## 2. Database Target Configuration

In the target list, select Add database and press Enter to register a database instance. Supported engines:
- **MySQL / MariaDB** (MySQL 5.7+, 8.0+; MariaDB 10.3+)
- **PostgreSQL** (version 12+)

| Field | Description |
| :--- | :--- |
| **IP:Port** | Unique instance identifier. IPv6 addresses must be bracketed (e.g., `[2001:db8::10]:5432`). |
| **Default Database** | Fallback database name when the MCP client query does not specify one. |
| **Read-Only User / Pass** | Mandatory credentials used for queries, schemas, and table listings. |
| **Writable User / Pass** | Optional credentials required for queries containing write keywords. If left blank, write operations are strictly blocked. |
| **Transport Policy** | `tls_verified` or `legacy_plaintext`. |
| **CA Certificate Path** | Mandatory local absolute path to the root CA certificate when using `tls_verified`. |
| **Description / Environment** | Local metadata for agent context. |

### TLS & Account Segregation Safeguards
- **Mandatory Validation**: When `tls_verified` is selected, server certificates are strictly validated against the CA. Verification failures will abort the connection rather than downgrade to plaintext.
- **Strict Account Isolation**: If no writable user is configured, any write query fails with `write_credential_not_configured`. `ssh-mcp` never silently executes write queries using read-only credentials.

---

## 3. Target Lifecycle: Toggle, Delete, & Re-enable

- Select **Enable / disable** to change target status. Re-enabling requires editing, verifying, and saving the target again.
- Select **Delete**, then explicitly select the confirmation button with Left/Right and press Enter.
- Select **Refresh** to reload the list.

---

## 4. Vault Encryption & Master Password

The master password protects the encrypted credential envelope stored inside `state.db`. Secrets are decrypted in daemon memory only while the vault is unlocked.

The dashboard provides menus for backup, restore, changing the master password, rotating the data key, locking the vault, and changing language. Choose with Up/Down or Tab and press Enter. Rotation still requires the confirmation text `ROTATE`.

> [!WARNING]
> If you lose your master password, the encrypted credentials cannot be recovered by the MCP server or agent. Always store your master password in a password manager and maintain encrypted backups.

See [private-key login and keyboard menus](private-key-auth.md) for local passphrase handling and lifecycle.
