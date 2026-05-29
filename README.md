# fleetsh

Run shell commands or scripts across a fleet of servers over SSH, or ping hosts to check connectivity.

## Prerequisites

- OpenSSH client (`ssh` and `scp`) must be installed and on your `PATH`
- SSH key-based or agent-based authentication (no password prompt support)

## Usage

```
fleetsh [alias] [flags]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--group` | `-g` | | Group to target |
| `--inventory` | `-i` | `.fleetsh`, then `~/.fleetsh` | Inventory file path |
| `--command` | `-c` | | Command to run remotely (mutually exclusive with -s, -p) |
| `--script` | `-s` | | Local script file to execute remotely (mutually exclusive with -c, -p) |
| `--timeout` | `-t` | `30000` | Per-host timeout in milliseconds |
| `--parallel` | `-l` | `1` | Max concurrent hosts (default: sequential) |
| `--json` | | | Output JSON |
| `--fail-fast` | | | Stop scheduling new hosts after first failure |
| `--dry-run` | | | Print what would run without connecting |
| `--ping` | `-p` | | Ping hosts (default: 3) (mutually exclusive with -c, -s) |
| `--help` | | | Help for fleetsh |
| `--version` | `-v` | | Version for fleetsh |

Exactly one of `--command`, `--script`, or `--ping` must be provided.

## Validation

| Flag | Constraint |
|------|------------|
| `--ping` | Must be >= 1 |
| `--parallel` | Must be >= 1 |
| `--timeout` | Must be >= 1 |
| `--inventory` | File must exist if provided |

## Output Format

Text output uses prefixes to indicate the type of line:

```
host | line             stdout from remote host
host ! line             stderr from remote host
host | message          error message
host | OK exit=N ...    fleetsh metadata (exit status, duration)
```

Example:
```
runner-1 | Linux 5.15.0-generic
runner-1 ! CPU: 2 cores, Load: 0.12
runner-1 | OK exit=0 duration=123ms
```

Lines with `|` are stdout from the remote host. Lines with `!` are stderr or errors. Lines with `OK` or `FAILED` are fleetsh metadata.

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
fleetsh -g all -p                    # ping each host 3 times (default)
fleetsh -g all -p 5                  # ping each host 5 times
fleetsh -g all --ping 3 -l 20        # ping 3 times with 20 concurrent hosts
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

Output is NDJSON (newline-delimited JSON) with streaming events. Each line is a separate JSON object:

```json
{"source":"fleetsh","type":"info","message":"fleetsh v0.0.1"}
{"source":"host","host":"runner-1","group":"all","type":"stdout","line":"Linux runner-1 5.15.0"}
{"source":"host","host":"runner-1","group":"all","type":"done","exit_code":0,"duration_ms":123}
{"source":"fleetsh","type":"summary","ok":1,"failed":0,"total":1,"duration_ms":150}
```

Event types:
- `info` - version info message
- `stdout` - stdout line from remote host
- `stderr` - stderr line from remote host
- `error` - error message
- `done` - host completed with exit_code and duration_ms
- `warning` - warning message
- `summary` - final summary with ok/failed/total counts

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
runner-1 administrator@runner-1.domain -i /path/to/key.pem -o ServerAliveInterval=60
runner-2 administrator@runner-2.domain -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null
runner-3 administrator@runner-3.domain -tt
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
alias [user@]hostname[:port] [ssh args...]
```

| Format | Example |
|--------|---------|
| Alias + user + host | `runner-1 administrator@runner-1.address.cx` |
| Alias + user + host + port | `db1 postgres@db1.example.cx:5432` |
| Alias + host (no user) | `web01 web01.example.com` |
| Alias + host + port (no user) | `web01 web01.example.com:2222` |
| Alias + user + host + SSH args | `runner-1 admin@runner-1.address.cx -i key.pem -o ServerAliveInterval=60` |
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
- `summary` is a reserved word — cannot be used as an alias or group name
- If an alias and group have the same name, the alias takes priority when resolving

### Comments

Lines starting with `#` or `;` are ignored.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | All hosts succeeded |
| `1` | One or more hosts failed |
| `2` | CLI, config, or inventory error |

## Security

- Host key verification is **enabled by default** using `~/.ssh/known_hosts`
- To disable verification, use `-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null` in the inventory entry
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