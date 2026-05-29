# fleetsh

Run shell commands or scripts across a fleet of servers over SSH, or ping hosts to check connectivity.

## Prerequisites

- OpenSSH client (`ssh`) must be installed and on your `PATH`
- SSH key-based or agent-based authentication (no password prompt support)

## Usage

```
fleetsh [alias] [flags]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--inventory` | `-i` | `.fleetsh`, then `~/.fleetsh` | Inventory file path |
| `--group` | `-g` | | Group to target |
| `--command` | `-c` | | Command to run remotely |
| `--script` | `-s` | | Local script file to execute remotely |
| `--user` | `-u` | | SSH username override |
| `--ping` | `-p` | | Ping hosts (default: 10 pings, mutually exclusive with -c/-s) |
| `--ping-count` | | `10` | Number of pings per host (used with -p) |
| `--key` | `-k` | | SSH private key path |
| `--parallel` | `-l` | `1` | Max concurrent hosts (default: sequential) |
| `--timeout` | `-o` | `30000` | Per-host timeout in milliseconds |
| `--dry-run` | | | Print what would run without connecting |
| `--json` | | | Output JSON |
| `--fail-fast` | | | Stop scheduling new hosts after first failure |
| `--insecure` | | | Skip host key verification |
| `--tty` | `-t` | | Allocate a pseudo-terminal (needed for sudo) |

Exactly one of `--command`, `--script`, or `--ping` must be provided.

## Validation

| Flag | Constraint |
|------|------------|
| `--ping-count` | Must be >= 1 |
| `--parallel` | Must be >= 1 |
| `--timeout` | Must be >= 1 |
| `--inventory` | File must exist if provided |
| `--key` | File must exist if provided |

## Output Format

Lines starting with `*` are output from the remote host:
```
runner-1 * Linux 5.15.0-generic
runner-1 * CPU: 2 cores, Load: 0.12
runner-1 | exit=0 duration=123ms
```

Lines starting with `|` are fleetsh metadata (exit status, summary).

## Targeting Hosts

- **Alias** (positional argument) — run on a single host: `fleetsh runner-1 -c "uptime"`
- **Group** (`-g` flag) — run on all hosts in a group: `fleetsh -g runners -c "uptime"`
- `-g` and positional argument are mutually exclusive

## Command Mode

```bash
fleetsh runner-1 -c "uptime"
fleetsh -g runners -c "df -h"
fleetsh -g all -c "hostname" --parallel 20
```

## Script Mode

Execute a local script on remote hosts by copying it to a temporary file on the remote host via SCP, executing it, then cleaning up:

```bash
fleetsh -g db -s ./scripts/check-postgres.sh
fleetsh -g all -s ./maintenance.sh
```

## Ping Mode

Check host connectivity by sending ICMP (if available) or TCP ping:

```bash
fleetsh -g all -p                    # ping each host 10 times (default)
fleetsh -g all -p --ping-count 3     # ping each host 3 times
fleetsh -g all -p -l 20              # ping with 20 concurrent hosts
```

Output:
```
runner-1 | min=0.500ms avg=0.750ms max=1.200ms ok=3 failed=0 total=3
runner-1 | exit=0 duration=5ms
```

### Ping Behavior

- Attempts ICMP ping first (requires root or `cap_net_raw`)
- Falls back to TCP ping (connects to ports 22, 80, 443, 8080) if ICMP fails with permission denied
- Reports min/avg/max RTT and packet counts

## Dry Run

Preview what would be executed without connecting:

```bash
fleetsh -g web -c "sudo systemctl restart nginx" --dry-run
```

## JSON Output

```bash
fleetsh -g all -c "uname -a" --json
```

Returns:

```json
{
  "results": [
    {
      "host": "runner-1",
      "group": "all",
      "success": true,
      "exit_code": 0,
      "stdout": "Linux runner-1 5.15.0",
      "stderr": "",
      "duration": "123ms"
    }
  ],
  "summary": {
    "ok": 1,
    "failed": 0,
    "total": 1
  }
}
```

## Fail Fast

Stop scheduling new hosts after the first failure (already-running hosts finish):

```bash
fleetsh -g all -c "risky-command" --fail-fast
```

## Inventory

Use a `.fleetsh` inventory file in the current directory or your home directory.

### Format

```ini
# hosts
runner-1 administrator@runner-1.domain
runner-2 administrator@runner-2.domain
runner-3 administrator@runner-3.domain
runner-4 administrator@runner-4.domain

[all]
runner-1
runner-2
runner-3
runner-4

[runners]
runner-1
runner-2
runner-3
runner-4

[primary]
runner-1
runner-2

[secondary]
runner-3
runner-4
```

### Note

Username is optional. If not defined, uses current user.

### Host Registry

Lines before any `[group]` header define the host registry. Each host is defined once:

```
alias [user@]hostname[:port]
```

| Format | Example |
|--------|---------|
| Alias + user + host | `runner-1 administrator@runner-1.address.cx` |
| Alias + user + host + port | `db1 postgres@db1.example.cx:5432` |
| Alias + host (no user) | `web01 web01.example.com` |
| Alias + host + port (no user) | `web01 web01.example.com:2222` |
| Alias only | `localhost` |

When user is omitted, the current system user is used.

### Groups

Groups reference hosts by alias only — no connection info is repeated:

```ini
[groupname]
alias1
alias2
```

Rules:

- Groups must be unique — duplicate `[groupname]` headers produce an error
- Aliases must be unique — duplicate aliases produce an error
- Group references must exist — referencing an undefined alias produces an error
- `[all]` is not automatic — define it explicitly if you want `fleetsh -g all` to work

### Comments

Lines starting with `#` or `;` are ignored.

### Priority

CLI flag > host definition > default.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All hosts succeeded |
| `1` | One or more hosts failed |
| `2` | CLI, config, or inventory error |

## Security

- Host key verification is **enabled by default** using `~/.ssh/known_hosts`
- `--insecure` skips verification but prints a loud warning
- No silent host key bypass
- SSH agent authentication is used automatically if `SSH_AUTH_SOCK` is set
- No password prompt support (use SSH keys or agent)

## Limitations

- No password authentication (key/agent only)
- No SFTP/file copy (uses SCP to copy scripts to remote /tmp)
- No playbook or task chaining
- No async or background execution mode
- Script mode requires `sh` on the remote host

## Build & Install

Build from source:

```bash
./build.sh
```

Build then copy the binary to your $PATH

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

---

Copyright 2026-present Douglas Hoard