# Daemon API

`kirimy-daemon` is the current HTTP adapter that executes `wacli` commands via the current daemon implementation.

## Contract surface

- `GET /healthz` → health check.
- `GET /readyz` → readiness.
- `GET /api/v1/commands` → list command catalog.
- `POST /api/v1/exec` → execute any arbitrary `wacli` command.
- `GET/POST/PUT/PATCH /api/v1/{command...}` → resource-first mapping to CLI paths.

Examples:

- `/api/v1/messages` → alias to `wacli messages list`
- `/api/v1/messages/search` → search messages
- `/api/v1/send/{kind}` → `text/file/sticker/voice/react/status/poll/select`
- `/api/v1/auth`, `/api/v1/auth/status`, `/api/v1/auth/logout`
- `/api/v1/sync`
- `/api/v1/chats/list`, `/api/v1/chats/show`, `/api/v1/chats/archive`, plus other CLI subcommands

For commands/subcommands not listed above, use:

- `/api/v1/{command}`
- `/api/v1/{command}/{subcommand}`

## Argument mapping

Query/body values are mapped to CLI flags:

- `snake_case` and `kebab-case` are normalized to `--flag` style.
- Common runtime keys: `store`, `account`, `read_only`, `json`, `full`, `events`, `timeout`, `lock_wait`.
- A special query key `query` is used as positional query for `messages search` and `contacts search`.
- Positional args can be sent with `args` or `positional` in JSON body.

Examples:

Send text:

```bash
curl -X POST 'http://localhost:8080/api/v1/send/text?to=12345&message=halo' \
  -H 'Content-Type: application/json' \
  -d '{"json": true}'
```

Run one-shot sync:

```bash
curl -X POST 'http://localhost:8080/api/v1/sync?once=true&download_media=false&json=true'
```

Generic exec:

```bash
curl -X POST 'http://localhost:8080/api/v1/exec' \
  -H 'Content-Type: application/json' \
  -d '{"command":["messages","search"],"args":["project"],"json":true}'
```

## Security

- `--api-token` enables validation using `Authorization: Bearer <token>`.
- Keep local defaults open only for local development.

## Roadmap

Current runtime still executes the `wacli` binary through `os/exec`.

Planned next step is to introduce service adapters that call `internal/app` and `internal/config` directly, then expose the same endpoint semantics from that shared layer. This reduces duplicate wrapper mapping while keeping existing endpoint shape stable.

## Reliability notes

For heavy/mutation workloads, operational hardening should include:

- job queueing for mutating endpoints (`send`, `sync follow`, media upload),
- metrics + request tracing,
- rate limiting + idempotency keys.
