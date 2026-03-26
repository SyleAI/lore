# Lore Lite

**The minimal intent execution substrate for autonomous codebases.**

Lore Lite is a CLI tool and local web UI that gives autonomous agents and humans a shared coordination layer, stored entirely in git refs. No database, no server, no merge conflicts on ticket state. `git clone` transfers everything.

It is not a ticketing system. It is the layer between human intent and agent execution.

---

## How It Works

Tickets live in `refs/tickets/` — a separate ref namespace in the git object store, orthogonal to branches.

```
refs/tickets/open/<id>        ← ticket: id, description, status, thread
refs/tickets/done/<id>        ← closed tickets
refs/tickets/questions/<qid>  ← open questions index
refs/tickets/policy           ← merge thresholds and config
```

Every ticket has a **thread** — an append-only sequence of entries (updates, questions, answers, images) written by agents and humans over the life of the ticket. The thread is the execution history.

Every state change emits a structured JSON event. The event log is a file; `lore events --follow` tails it.

---

## Install

```sh
go install github.com/loreteam/lore@latest
```

Requires Go 1.25+. Single static binary, no CGO.

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
| `lore update <id> --image <path>` | Attach an image; stored as a git blob |
| `lore ask <id> "..."` | Append a question; `--block` to block the ticket |
| `lore answer <id> --question-id <qid> "..."` | Answer an open question |
| `lore block <id> "reason"` | Mark ticket blocked with a reason |
| `lore ready <id>` | Signal work is done; sets status to `ready-for-review` |
| `lore close <id>` | Mark ticket done |
| `lore spawn <id> --from "..."` | Create a child ticket |

### Coordination (Lead + Human)

| Command | Description |
|---|---|
| `lore assign <id> --agent <name>` | Assign ticket to a specific agent |
| `lore unblock <id>` | Clear a block; sets status back to `working` |
| `lore approve <id>` | Approve a ready-for-review ticket |
| `lore reject <id> --reason "..."` | Reject; ticket returns to `working` |

### Setup + UI

| Command | Description |
|---|---|
| `lore init` | Initialize Lore in the current git repo |
| `lore doctor` | Check that the environment is correctly configured |
| `lore ui` | Start the local web UI (default port 7890) |
| `lore events --follow` | Stream the event log |
| `lore purge [--force]` | Delete all tickets and clear the event log |

---

## Web UI

`lore ui` starts a local web server at `http://localhost:7890`. Agents use the CLI; humans use the UI.

**Review dashboard** — the default landing page. Shows everything that needs human attention: open questions, tickets ready for review, blocked tickets. Answer questions inline, approve or reject without leaving the page.

**Ticket list** — all tickets, filterable by status (open / working / blocked / ready / done).

**Ticket detail** — the full thread rendered chronologically. Images inline. Action bar at the bottom for updates, image attachments, approve/reject/unblock.

**New ticket** — free-form description with drag-and-drop image attachment.

**Search** — substring search across all ticket descriptions.

**Event log** — live SSE stream of all system events, color-coded by type. Pause/resume auto-scroll.

The UI binds to `localhost` only. Static assets are embedded in the binary — no install step.

---

## Status Values

Tickets move through a defined lifecycle. Each transition emits an event.

| Status | Meaning |
|---|---|
| `open` | Ticket created, not yet claimed by anyone. Any agent can pick it up. |
| `working` | An agent has claimed the ticket and is actively working on it. Also the state a ticket returns to after rejection. |
| `blocked` | The agent cannot proceed and is waiting on something external — an answer, a dependency, a human decision. |
| `ready-for-review` | The agent has finished and submitted the work for review. No further changes expected until approved or rejected. |
| `done` | Work approved and complete. The ticket moves to `refs/tickets/done/` and is excluded from the active list by default. |

**Approval** is the act of a reviewer (human or lead agent) confirming that the work in a `ready-for-review` ticket actually meets the acceptance criteria. It is a deliberate quality gate — not a formality. A rejected ticket returns to `working` so the agent can revise and resubmit.

---

## Events

Every state change emits a structured JSON event to `.lore/events.log`. Orchestrators tail this file to drive their agent loops. `lore events --follow` streams it live.

Each event has the shape:
```json
{"type": "ticket.ready", "ts": "2025-01-15T10:23:41Z", "data": {"id": "a3f9c12d8e1b"}}
```

### Event Reference

| Event | Emitted when | Key `data` fields |
|---|---|---|
| `lore.initialized` | `lore init` completes | `git_root` |
| `ticket.created` | `lore new` or `lore spawn` | `id`, `description` |
| `ticket.assigned` | `lore assign` | `id`, `agent` |
| `ticket.claimed` | `lore claim` | `id`, `agent` |
| `ticket.updated` | `lore update` (text or image) | `id`; `kind: "image"` if image |
| `ticket.blocked` | `lore block` | `id`, `reason` |
| `ticket.unblocked` | `lore unblock` | `id` |
| `ticket.ready` | `lore ready` | `id` |
| `ticket.approved` | `lore approve` | `id` |
| `ticket.rejected` | `lore reject` | `id`, `reason` |
| `ticket.closed` | `lore close` | `id` |
| `question.asked` | `lore ask` | `id`, `question_id`, `blocking` |
| `question.answered` | `lore answer` | `id`, `question_id` |
| `question.tagged` | `lore tag` | `id`, `question_id` |

### Subscribing

```sh
# Tail the raw event log
lore events --follow

# Emit events to stdout as JSON (useful for orchestrators reading stdin)
lore --json <command>

# Silence events entirely
lore --quiet <command>
```

---

## Images

Humans and agents can attach images to any ticket thread — screenshots, diagrams, annotated UI states.

```sh
lore update <id> --image ./screenshots/error-modal.png
lore update <id> --image ./arch-diagram.png "flow we need to preserve"
```

Images are stored as git blobs. The thread entry records the blob hash and optional caption. `lore show --json` delivers images as base64 in the context payload for Claude agents.

---

## Design Principles

- **Git refs, not files** — no merge conflicts, full portability, `git clone` transfers everything
- **Blackboard architecture** — agents coordinate through shared state, not peer-to-peer conversation
- **Events as the contract** — Lore emits events; it does not implement the agentic loop
- **Humans in the loop** — review dashboard and inline actions make human oversight low-friction
- **No dependencies** — single static binary, no database, no server, no external services

---

## What Is Not in v1

Deliberately deferred until real usage data justifies the complexity:

- Knowledge graph and semantic index
- Priority scoring and blast radius computation
- `lore run` worker loop (depends on validated skill templates — deferred to v2)
- Agent skill file installation (`lore init` does not write skill files in v1)
- Consolidation and improvement mode

See [lore-lite.md](lore-lite.md) for the full v1 spec and v2 roadmap.

---

## License

MIT
