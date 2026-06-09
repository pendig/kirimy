# Kirimy v2 — Open Source WhatsApp All-in-One Platform

This project is a **fork of**
[`openclaw/wacli`](https://github.com/openclaw/wacli) and evolves it into an all-in-one platform:

- **CLI** for operator and script workflows.
- **Daemon API** for service-to-service integrations.
- **UI** entrypoints for product-style consumption.
- **MCP adapters** for automation tooling.

All surfaces are intended to be layered on top of the same core logic derived from wacli, so new features can be adopted once in the core and then exposed to CLI/API/UI/MCP with small integration work.

## Kirimy Goals

1. Keep existing CLI behavior compatible.
2. Provide a practical API/service surface without breaking current workflows.
3. Keep contracts centralized to avoid drift.
4. Move from temporary `exec`-based daemon execution toward a shared service layer.
5. Make feature rollouts easy: when wacli gains features, propagate them to contracts incrementally instead of rewriting logic.

## Current implementation status

At the moment, `cmd/kirimy-daemon` uses the **exec wrapper** approach for fast delivery:

- CLI remains the primary command entrypoint in `cmd/wacli/`.
- Daemon API runs from `cmd/kirimy-daemon/`.
- Active API surfaces are `POST /api/v1/exec`, `GET /api/v1/commands`, and `GET/POST/PUT/PATCH /api/v1/{command...}`.

Planned evolution toward shared core service layer:

1. Keep exec wrapper stable (minimal risk, fast iteration).
2. Add internal service adapters that call `internal/app` and `internal/config` directly.
3. Route selected APIs through service adapters, while CLI and other surfaces continue existing behavior.
4. Expand to UI/MCP surfaces using the same service contract.

## Architecture (current)

- `cmd/wacli/`: CLI entrypoint.
- `cmd/kirimy-daemon/`: Daemon API entrypoint.
- `internal/*`: Reusable core packages (`internal/app`, `internal/config`, `internal/store`, etc.).
- `api/`: Endpoint contracts and OpenAPI.
- `daemon/`: Operational runbook and deployment notes.

## Build

```bash
cd path/to/kirimy

CGO_ENABLED=1 CGO_CFLAGS='-Wno-error=missing-braces' go build -tags sqlite_fts5 -o ./bin/wacli ./cmd/wacli

CGO_ENABLED=1 CGO_CFLAGS='-Wno-error=missing-braces' go build -tags sqlite_fts5 -o ./bin/kirimy-daemon ./cmd/kirimy-daemon
```

## Running CLI

```bash
./bin/wacli --help
./bin/wacli auth
./bin/wacli sync --follow
./bin/wacli messages search "meeting"
./bin/wacli send text --to 1234567890 --message "hello"
./bin/wacli doctor
```

## Running Daemon

```bash
./bin/kirimy-daemon --listen :8080 --store ~/.wacli --json
```

Important daemon flags:

- `--wacli-binary`: path to the CLI binary (`wacli`) used by the wrapper.
- `--store`, `--account`: default runtime context.
- `--read-only`: safer mode for read-only workloads.
- `--json`, `--full`, `--events`: default CLI-style output modes.
- `--command-timeout`: per-request timeout.
- `--api-token`: enable `Authorization: Bearer <token>`.

## CLI <-> API mapping (live)

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/commands`
- `POST /api/v1/exec`
- `GET/POST/PUT/PATCH /api/v1/{command...}` (resource-first)

Example:

- `GET /api/v1/messages` → `wacli messages list`
- `GET /api/v1/messages/search?q=invoice` → `wacli messages search invoice`
- `GET /api/v1/contacts/search?query=nama` → `wacli contacts search nama`
- `POST /api/v1/send/text?to=1234567890&message=halo` → `wacli send text ...`
- `POST /api/v1/auth/status`
- `POST /api/v1/sync?once=true`
- `POST /api/v1/exec` with body `{ "command": ["messages","search"], "args": ["meeting"], "json": true }`

## References

- API contract and examples: [api/README.md](api/README.md)
- OpenAPI: [api/openapi.yaml](api/openapi.yaml)
- Daemon operations: [daemon/README.md](daemon/README.md)

## Maintenance Checklist

For every new feature:

1. Update CLI behavior and command contract in `cmd/wacli/`.
2. Update API contract in `api/` (or expose via `POST /api/v1/exec`).
3. Update deployment notes if operational behavior changes.
4. When feature is ready for shared service layer, route it through adapters before adding UI/MCP surfaces.
