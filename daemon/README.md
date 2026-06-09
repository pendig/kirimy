# Daemon Operations (kirimy-daemon)

Operational runbook for running `kirimy-daemon`.

## Running locally

```bash
go run ./cmd/kirimy-daemon --listen :8080 \
  --store ~/.wacli \
  --json
```

Important flags:

- `--wacli-binary`: path to the CLI binary (`wacli`) used by the current daemon wrapper.
- `--store`, `--account`: default storage context for requests.
- `--read-only`: enable default read-only behavior.
- `--json`, `--full`, `--events`: default CLI-style output behavior forwarded to wrapper commands.
- `--command-timeout`: per-request timeout.
- `--api-token`: enable bearer token validation.

## Endpoints

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/commands`
- `POST /api/v1/exec`
- `GET/POST/PUT/PATCH /api/v1/{command...}`

## Example service file

`/etc/systemd/system/kirimy-daemon.service`

```ini
[Unit]
Description=kirimy daemon API
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/kirimy
ExecStart=/opt/kirimy/bin/kirimy-daemon --listen 127.0.0.1:8080 --store /opt/kirimy/data/wacli --json --command-timeout 30s
Restart=always
RestartSec=3
Environment=HOME=/opt/kirimy
Environment=WACLI_STORE_DIR=/opt/kirimy/data/wacli
StandardOutput=append:/var/log/kirimy-daemon/out.log
StandardError=append:/var/log/kirimy-daemon/err.log

[Install]
WantedBy=multi-user.target
```

## Production operations

1. Run behind reverse proxy + TLS.
2. Enable `--api-token` and restrict network access.
3. Keep `--read-only=true` for environments that must not mutate.
4. Set `--read-only=false` only for dedicated worker nodes that need mutation.
5. Monitor latency, exit_code, and error rate.
6. Record contract changes whenever new command mapping is added.

## Evolution notes

The daemon is currently an `exec` wrapper around `wacli`.

Next phase: move wrapper calls behind a shared service layer (internal to this repo) so CLI, API, and future UI/MCP surfaces can reuse one core orchestration path.
