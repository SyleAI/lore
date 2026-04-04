# Lore

**The intent execution substrate for autonomous codebases.**

Lore is a CLI tool and local web UI that gives autonomous agents and humans a shared coordination layer, stored as plain files under `.tickets/`. No database, no server.

It is not a ticketing system. It is the layer between human intent and agent execution.

---

## How It Works

Tickets live under `.tickets/` in the working tree:

```
.tickets/
  open/<id>.yaml              ← active tickets
  done/<id>.yaml              ← closed tickets
  threads/<id>/<ts>-<eid>.json  ← thread entries (updates, questions, answers, images)
  questions/<qid>.json        ← question index
  blobs/<sha256>              ← content-addressed image blobs
```

Every ticket has a **thread** — an append-only sequence of entries written by agents and humans over the life of the ticket. The thread is the execution history.

Every state change emits a structured JSON event. The event log is a file; `lore events --follow` tails it.

Orchestrators are reusable automation packages that drive agent loops against lore-enabled repos. They live under `.lore/orchestrators/`.

---

## Install

```sh
go install github.com/loreteam/lore@latest
```

Requires Go 1.25+. Single static binary.

---

## Quick Start

```sh
# Initialize Lore in a git repository
lore init

# Open the web UI (http://localhost:7890)
lore ui

# Create a ticket
lore new "fix auth timeout on long sessions"

# List open tickets
lore list

# Claim a ticket (atomic — prevents two agents taking the same one)
lore claim <id>

# Append a progress note
lore update <id> "investigated token refresh path, found stale TTL config"

# Ask a blocking question
lore ask <id> "should we extend the TTL or add a refresh endpoint?" --block

# Mark work ready for human review
lore ready <id>

# Approve or reject
lore approve <id>
lore reject <id> --reason "needs tests"

# Follow the event stream
lore events --follow
```

---

## Commands

### Read / Write

| Command | Description |
|---|---|
| `lore new` | Create a ticket |
| `lore show <id>` | Full ticket: description, status, complete thread |
| `lore list` | List tickets, filterable by status |
| `lore list --closed` | Include done tickets |
| `lore search --query "..."` | Search across all ticket descriptions |

### Agent Operations

| Command | Description |
|---|---|
| `lore claim <id>` | Atomic claim — prevents two agents taking the same ticket |
| `lore update <id> "..."` | Append a free-form update to the thread |
| `lore update <id> --image <path>` | Attach an image; stored as a content-addressed blob |
| `lore ask <id> "..."` | Append a question; `--block` to block the ticket |
| `lore answer <id> --question-id <qid> "..."` | Answer an open question |
| `lore block <id> "reason"` | Mark ticket blocked with a reason |
| `lore unblock <id>` | Clear a block; sets status back to `working` |
| `lore ready <id>` | Signal work is done; sets status to `ready-for-review` |
| `lore close <id>` | Mark ticket done |
| `lore spawn <id> "..."` | Create a child ticket |

### Coordination (Lead + Human)

| Command | Description |
|---|---|
| `lore assign <id> --agent <name>` | Assign ticket to a specific agent |
| `lore tag <id> --question-id <qid> --agent <name>` | Route a question to a specific agent |
| `lore approve <id>` | Approve a ready-for-review ticket |
| `lore reject <id> --reason "..."` | Reject; ticket returns to `working` |

### Orchestrators

| Command | Description |
|---|---|
| `lore install <src>` | Install an orchestrator from a local path, `user/repo`, or git URL |
| `lore run <name> [args...]` | Run an installed orchestrator |
| `lore orchestrators` | List installed orchestrators |
| `lore orchestrators remove <name>` | Remove an installed orchestrator |

### Setup + Maintenance

| Command | Description |
|---|---|
| `lore init` | Initialize Lore in the current git repo |
| `lore doctor` | Check that the environment is correctly configured |
| `lore ui` | Start the local web UI (default port 7890) |
| `lore events --follow` | Stream the event log |
| `lore purge [--force]` | Delete all tickets and orchestrators |
| `lore purge --tickets` | Delete only tickets |
| `lore purge --orchestrators` | Delete only installed orchestrators |

---

## Orchestrators

An orchestrator is a self-contained automation package that drives an agent loop against a lore-enabled repo. It declares a manifest (`lore.yaml`) and an entrypoint script.

```yaml
# lore.yaml
name: newsapp
version: 0.1.0
description: Lead + worker loop for the newsapp project
runtime: python3
entrypoint: orchestrator.py
goal: |
  Coordinate agents to implement features described in open lore tickets.
```

Install and run:

```sh
lore install ./path/to/orchestrator
lore install user/repo              # GitHub shorthand
lore install https://github.com/user/repo.git

lore run newsapp
lore run newsapp --dry-run          # extra args passed through to the orchestrator
```

Lore injects three env vars when running an orchestrator:

| Variable | Value |
|---|---|
| `LORE_REPO_ROOT` | Absolute path to the git repo root |
| `LORE_ORCHESTRATOR_DIR` | Absolute path to the orchestrator's install directory |
| `LORE_ORCHESTRATOR_NAME` | Name of the orchestrator |

---

## Web UI

`lore ui` starts a local web server at `http://localhost:7890`. Agents use the CLI; humans use the UI.

**Review dashboard** — the default landing page. Shows everything that needs human attention: open questions, tickets ready for review, blocked tickets. Answer questions inline, approve or reject without leaving the page.

**Ticket list** — all tickets, filterable by status.

**Ticket detail** — the full thread rendered chronologically. Images inline. Action bar for updates, image attachments, approve/reject/unblock.

**New ticket** — free-form description with drag-and-drop image attachment.

**Search** — substring search across all ticket descriptions.

**Event log** — live SSE stream of all system events.

The UI binds to `localhost` only. Static assets are embedded in the binary.

---

## Ticket Lifecycle

| Status | Meaning |
|---|---|
| `open` | Created, not yet claimed |
| `working` | Claimed by an agent; also the state after rejection |
| `blocked` | Waiting on an answer, dependency, or human decision |
| `ready-for-review` | Agent finished; awaiting approval or rejection |
| `done` | Approved and complete; moved to `done/` |

---

## Events

Every state change emits a structured JSON event to `.lore/events.log`.

```json
{"type": "ticket.ready", "ts": "2025-01-15T10:23:41Z", "data": {"id": "a3f9c12d8e1b"}}
```

| Event | Emitted when |
|---|---|
| `lore.initialized` | `lore init` completes |
| `ticket.created` | `lore new` or `lore spawn` |
| `ticket.assigned` | `lore assign` |
| `ticket.claimed` | `lore claim` |
| `ticket.updated` | `lore update` |
| `ticket.blocked` | `lore block` |
| `ticket.unblocked` | `lore unblock` |
| `ticket.ready` | `lore ready` |
| `ticket.approved` | `lore approve` |
| `ticket.rejected` | `lore reject` |
| `ticket.closed` | `lore close` |
| `question.asked` | `lore ask` |
| `question.answered` | `lore answer` |
| `question.tagged` | `lore tag` |

---

## Design Principles

- **Files, not git internals** — tickets are plain YAML/JSON files in `.tickets/`; no git object store manipulation
- **Blackboard architecture** — agents coordinate through shared state, not peer-to-peer conversation
- **Events as the contract** — Lore emits events; it does not implement the agentic loop
- **Humans in the loop** — review dashboard and inline actions make human oversight low-friction
- **No dependencies** — single static binary, no database, no server, no external services

---

## License

MIT
