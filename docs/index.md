---
title: Overview
permalink: /
description: "Kirimy is a fork of wacli that keeps the same CLI/data model and adds a daemon API layer for service integrations."
---

# Kirimy (wacli fork)

A script-friendly WhatsApp CLI built on [`whatsmeow`](https://github.com/tulir/whatsmeow). The same core logic powers the `wacli` CLI and `kirimy-daemon` API. A linked-device session stores messages in local SQLite with FTS5 and exposes workflows for terminals, scripts, and service integrations.

## Why wacli

- **Local mirror, fast search.** All synced messages land in a SQLite store with an FTS5 index; offline `messages search` returns hits in milliseconds.
- **Chat state controls.** Archive, pin, mute, and mark chats read/unread from the CLI, then filter `chats list` by those states.
- **Stable output.** Human-readable tables by default, `--json` to stdout for scripts, NDJSON `--events` for long-running commands. Human progress, prompts, and errors stay on stderr so pipes stay clean.
- **Dual surface.** `wacli` is the main operator CLI; `kirimy-daemon` exposes the same command model through HTTP (`POST /api/v1/exec`, `GET /api/v1/commands`, resource-first endpoints). Run `wacli auth`, then `wacli sync --follow` to keep the store warm.
- **Built for agents.** `--read-only` (or `WACLI_READONLY=1`) blocks every command that mutates WhatsApp or local state. Store locks prevent two instances from racing on the same device identity.
- **Boundable storage.** `sync` warns when storage is uncapped; `--max-messages` / `--max-db-size` cap local growth. Send retries are bounded; media uploads/downloads cap at 100 MiB.
- **Best-effort history.** `history coverage` shows local anchors, `history fill --dry-run` plans candidate chats, and `history backfill` requests older messages per chat from your primary device.

## Pick your path

- **Trying it.** Read [Install](install.md), then [Quickstart](quickstart.md). Pair, sync, and send your first message in under five minutes.
- **Using multiple WhatsApp accounts.** Read [Accounts](accounts.md) for named account stores and `--account`.
- **Searching old chats.** Read [Sync](sync.md) for the sync model and [History](history.md) for coverage planning and on-demand backfill.
- **Managing chat state.** Read [Chats](chats.md) for archive, pin, mute, and read/unread commands.
- **Managing local storage.** Read [Store](store.md) for stats, dry-run cleanup, and local-only pruning.
- **Sending from scripts.** Read [Send](send.md) for recipient resolution, channels, status broadcasts, replies, mentions, files, and reactions.
- **Mirroring address-book names.** Read [Contacts import-system](contacts-import-system.md) to import macOS Contacts display names into local wacli metadata.
- **Wiring up an agent.** Pair `--read-only`, `--json`, and `--events` from [Overview](overview.md); read [Doctor](doctor.md) for self-checks.
- **Serving APIs.** Read [Daemon API](../api/README.md) for endpoint contracts and [Daemon ops](../daemon/README.md) for production behavior.
- **Building companion tools.** Read [Companion integrations](integrations.md) for safe read-only SQLite and JSON integration patterns.
- **Looking up a flag.** Open the per-command pages from [Overview](overview.md).

## Status

Core implementation is in place. The [CHANGELOG](https://github.com/openclaw/wacli/blob/main/CHANGELOG.md) tracks shipped behavior. WhatsApp Web is not a published API; expect occasional breakage from upstream protocol changes — `wacli` follows `whatsmeow` upstream.

## Out of scope

- Guaranteed full-history export (WhatsApp Web history is best-effort).
- Hosted SaaS infrastructure, tenant billing, or end-user account-management portal.
- End-to-end "contact creation" inside WhatsApp; local aliases and tags only.

## Disclaimer

`wacli` is a third-party tool that uses the WhatsApp Web protocol via `whatsmeow`. It is **not affiliated with WhatsApp or Meta**. Use at your own risk; pairing as a linked device is subject to WhatsApp's terms.

Released under the [MIT license](https://github.com/openclaw/wacli/blob/main/LICENSE).
