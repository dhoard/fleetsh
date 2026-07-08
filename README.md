[![Build](https://github.com/dhoard/fleetsh/actions/workflows/build.yaml/badge.svg)](https://github.com/dhoard/fleetsh/actions/workflows/build.yaml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue)](./LICENSE)

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
| `--no-trunc` | | | Disable output line truncation |
| `--tty` | `-T` | | Allocate a pseudo-TTY for real-time output streaming |
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

Text output uses an aligned host prefix followed by a symbol indicating the type of line:

```
info      | fleetsh v0.0.2-POST     version info (literal "info" prefix)
host      * line                    stdout from remote host
host      ! line                    stderr from remote host
host      ! message                 connection or execution error
host      | exit=N duration=...      per-host completion (exit code, duration)
summary   | ok=N failed=N total=N exit=N duration=Nms
```

Example:
```
info      | fleetsh v0.0.2-POST
runner-1  * Linux 5.15.0-generic
runner-1  ! CPU: 2 cores, Load: 0.12
runner-1  | exit=0 duration=123ms
summary   | ok=1 failed=0 total=1 exit=0 duration=130ms
```

Lines with `*` are stdout from the remote host. Lines with `!` are stderr or connection/execution errors. The `| exit=... duration=...` line is per-host completion metadata, and the final `summary` line aggregates results. The host prefix is padded so columns align.

## Error Output Format

When a CLI error occurs (e.g., invalid flags, missing arguments), the error is displayed with the message on its own line, indented by 2 spaces. For validation errors, a blank line and the usage information follow:

```
Error:
  flag needs an argument: 'c' in -c

Usage:
  fleetsh [alias] [flags]

Flags:
...
```

### Mutually Exclusive Flags

`--command`, `--script`, and `--ping` are mutually exclusive. If more than one is provided, the error lists only the flags that were actually supplied, in the order they appeared on the command line:

```
$ fleetsh -p 3 -c "uptime"
Error:
  --ping, --command are mutually exclusive

$ fleetsh -c "uptime" -s ./script.sh
Error:
  --command, --script are mutually exclusive
```

## Targeting Hosts

- **Alias** (positional argument) — run on a single host: `fleetsh runner-1 -c "uptime"`
- **Group** (`-g` flag) — run on all hosts in a group: `fleetsh -g runners -c "uptime"`
- `-g` and positional argument are mutually exclusive

### Pattern Matching

Use bracket syntax `[...]` for regex patterns:

```bash
# Regex on group names
fleetsh -g [web.*] -c "uptime"

# Regex on aliases
fleetsh [runner-.*] -c "uptime"

# Match groups containing "prod"
fleetsh -g [.*prod.*] -c "df -h"
```

**How it works**: If the target starts with `[` and ends with `]`, the inner content is treated as a regex pattern. Otherwise, it is treated as an exact name (alias or group) with fallback to implicit regex matching on group names when using `-g`.

**Examples**:
```
fleetsh [runner-.*] -c "uname -a"           # matches runner-1, runner-2, runner-prod
fleetsh "[db0[12]]" -c "postgres --version" # matches db01, db02
fleetsh -g [web.*] -c "systemctl status"    # matches web-prod, web-staging
```

**Note**: Patterns are not implicitly anchored. Use `^` and `$` for exact matching.

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
fleetsh -g all --ping                 # ping each host 3 times (default)
fleetsh -g all --ping -l 20           # ping with 20 concurrent hosts
```

Output:
```
info      | fleetsh v0.0.2-POST
runner-1  | min=0.500ms avg=0.750ms max=1.200ms ok=3 failed=0 total=3
runner-1  | exit=0 duration=5ms
summary   | ok=1 failed=0 total=1 exit=0 duration=8ms
```

### Ping Behavior

- Attempts ICMP ping first (requires root or `cap_net_raw`)
- Falls back to TCP ping (connects to ports 22, 80, 443, 8080) if ICMP fails with permission denied
- Reports min/avg/max RTT and packet counts
- Honors `--timeout`, which bounds the overall ping operation

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
{"source":"fleetsh","type":"info","message":"fleetsh v0.0.2-POST"}
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

## Real-Time Output Streaming (TTY)

Remote programs typically block-buffer stdout when connected to a pipe (not a TTY). This means output may be delayed until the program exits, even for long-running processes that produce periodic output.

The `--tty` flag forces pseudo-TTY allocation on the remote host via `ssh -tt`. When a PTY is attached, most programs switch to line-buffered output, enabling real-time streaming.

```bash
# Without --tty: output may be delayed (programs block-buffer to pipes)
fleetsh -g runners -c "node app.js"

# With --tty: real-time streaming (programs line-buffer to PTY)
fleetsh -g runners -c "node app.js" --tty
```

**Trade-offs:**

- **stdout/stderr merge** — PTY combines stdout and stderr into a single stream. The distinction between `*` (stdout) and `!` (stderr) lines in the output is no longer reliable.
- **ANSI escape codes** — Some programs emit color or control codes when a TTY is detected. To suppress, pass `-o RequestTTY=no` via the inventory SSH args or use `TERM=dumb`.
- **Carriage returns** — PTY output may include `\r\n` line endings. fleetsh strips trailing `\r` automatically.

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
| alias + user + host | `runner-1 administrator@runner-1.address.cx` |
| alias + user + host + port | `db1 postgres@db1.example.cx:5432` |
| alias + host (no user) | `web01 web01.example.com` |
| alias + host + port (no user) | `web01 web01.example.com:2222` |
| alias + user + host + SSH args | `runner-1 admin@runner-1.address.cx -i key.pem -o ServerAliveInterval=60` |
| alias only | `localhost` |

When user is omitted, the current system user is used.

**Note**: Aliases must contain only letters, numbers, underscores, and hyphens (`[a-zA-Z0-9_-]`). Spaces and special characters are not allowed.

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
- Aliases and group names must match `[a-zA-Z0-9_-]`
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

Requires [GoReleaser](https://goreleaser.com/install/).

### Local development build

```bash
./build.sh
```

Runs tests, vet, and builds a single binary for the current platform. Output in `dist/`.

### Cross-platform release packaging

```bash
./build.sh release
```

Runs tests, vet, cross-compiles for all target platforms (Linux amd64, arm64, 386; macOS amd64, arm64), and creates `.tar.gz` archives and a `checksums.txt` in `dist/`.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

---

Copyright 2026-present Douglas Hoard
