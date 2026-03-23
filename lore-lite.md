# Lore Lite — v1 Spec

The minimal substrate. Everything a team of autonomous agents needs to coordinate on a codebase, nothing more.

---

## Ticket Schema

```
id, freeform_desc, status
```

Status values: `open | working | blocked | ready-for-review | done`

Nothing else. Priority, type, spawned_by — all optional metadata you add later. The core fields are what the agent reads before starting and what it writes to during execution.

A ticket is not a structured form. It is a description of intent in whatever form is most useful — a sentence, a paragraph, a list of acceptance criteria. The agent decides what goes in it.

---

## Ticket Thread

Every ticket has a thread — an append-only sequence of entries written by agents and humans over the life of the ticket. The thread is the execution history.

Thread entries come from:
- `lore update` — free-form progress note, observation, or decision
- `lore ask` — a question (same as an update, but emits a `question.asked` event)
- `lore answer` — an answer to a question (same as an update, but emits a `question.answered` event)
- `lore comment` — context written by the lead before or during assignment
- Image attachments — stored as git blobs, referenced by hash in the thread entry

`lore show <id>` returns the full ticket: description, current status, and the complete thread in chronological order. For agents consuming via Claude, image attachments are delivered as base64 in the context payload.

---

## Command Surface

### Read / Write

**`lore new`** — create a ticket (interactive or `--from "..."` for natural language input)
**`lore show <id>`** — read full ticket: description + status + complete thread
**`lore list`** — list tickets, filterable by status
**`lore list --closed`** — include done tickets
**`lore search --query "..."`** — search over full ticket threads on demand; no persistent index, calls the model against the ticket corpus

### Agent Operations

**`lore claim <id>`** — atomic lock; prevents two agents taking the same ticket
**`lore update <id> "..."`** — append a free-form update to the ticket thread
**`lore update <id> --image path/to/file`** — attach an image; stored as a git blob, referenced in the thread
**`lore ask <id> "..."`** — append a question to the thread; emits `question.asked`
**`lore answer <id> --question-id <qid> "..."`** — append an answer; emits `question.answered`
**`lore escalate <id> "..."`** — flag ticket as blocked; sets status to `blocked`
**`lore ready <id>`** — signal work is done; sets status to `ready-for-review`, triggers merge evaluation
**`lore close <id>`** — mark ticket done; sets status to `done`
**`lore spawn <id> --from "..."`** — create a child ticket from within the current one

### Coordination (Lead + Human)

**`lore assign <id> --agent <name>`** — lead assigns ticket to a specific agent
**`lore comment <id> "..."`** — lead writes context onto the ticket before or during assignment
**`lore tag <id> --question-id <qid> --agent <name>`** — route a question to a specific agent
**`lore unblock <id>`** — clear escalation; sets status back to `working`
**`lore approve <id>`** — human approves a high-risk merge
**`lore reject <id> --reason "..."`** — human rejects; ticket returns to `working`
**`lore review`** — show everything that needs human attention (blocked, ready-for-review, open questions)

### Events + Runtime

**Events** — one structured JSON event per meaningful state change
**`lore events --follow`** — stream the event log
**`lore run`** — first-party worker loop for Claude Code
**`lore run --agents <n>`** — run n parallel workers
**`lore run --lead`** — run in lead mode

### Setup

**`lore init`** — initialize Lore in the current git repo
**`lore doctor`** — check that the environment is correctly configured
**`lore ui`** — start the local web UI (default port 7890)

---

## Web UI

`lore ui` starts a local web server and is the primary interface for humans. Agents use the CLI; humans use the UI.

### Views

**Review dashboard** — the default landing page. Shows everything that needs human attention, grouped by urgency:
- Tickets with open unanswered questions
- Tickets in `ready-for-review` awaiting approval or rejection
- Tickets in `blocked` awaiting unblocking

**Ticket list** — all tickets, filterable by status. Clicking a ticket opens the detail view.

**Ticket detail** — the full ticket thread rendered chronologically:
- Description and current status at the top
- Each thread entry (update, question, answer, comment) in order
- Images rendered inline — no opening external apps
- Action bar at the bottom: approve, reject, unblock, answer, comment

**New ticket** — form to create a ticket with a free-form description and optional image attachments.

**Search** — triggers `lore search` against the full ticket corpus; results link to ticket detail views.

**Event log** — live stream of events, auto-updating. Useful for watching agents work in real time.

### Human Actions from the UI

Everything in the coordination surface is available from the UI:
- Answer open questions inline in the thread
- Approve or reject tickets in `ready-for-review`
- Unblock escalated tickets with an optional note
- Assign tickets to agents
- Comment on any ticket
- Create new tickets with image attachments
- Attach images to existing ticket threads

### Implementation Notes

- Served by the `lore` binary itself — no separate server process, no install step
- Static assets embedded in the binary at build time
- Server-side rendered with lightweight JS for live event streaming and inline actions
- Port defaults to `7890`, configurable via `--port` or `.lore/config.yaml`
- Binds to `localhost` only — not exposed to the network

---

## What Is Not in v1

Deferred until real usage data shows it is needed.

**Not needed:**
- Graph engine and `graph.index`
- Consolidation service
- AI resolver running in background
- Improvement mode and observer agent
- Context affinity scoring
- File heat calculation
- Priority scoring algorithm
- Blast radius computation
- Continuously maintained semantic index
- External store / split storage model
- `lore batch`, `lore merge`
- `lore signals`, `lore graph`, `lore why`, `lore candidates`
- `lore measure`, `lore consolidate`
- `lore checkpoint` (criteria progress tracking)

**`lore search`** replaces the knowledge graph. One command, no persistent index, on-demand model call against the full ticket corpus. The lead skill instructs the lead agent to call it before every decision.

**`lore list --closed`** plus `lore show` replaces consolidation. The lead agent reads history and notices patterns. No background service.

---

## Image Support

Humans can attach images to any ticket thread — screenshots, diagrams, annotated UI states, whatever gives the agent visual context.

```sh
lore update fix-auth-timeout --image ./screenshots/error-modal.png
lore update fix-auth-timeout --image ./arch-diagram.png "this is the flow we need to preserve"
```

Images are stored as git blobs (same object store as everything else). The thread entry records the blob hash and optional caption. `lore show` surfaces them; when consumed by a Claude agent via `lore show --json`, images are delivered as base64 in the context payload alongside text entries.

This is write-once. Images are part of the permanent ticket record.

---

## Storage

All state lives in git refs, not in the working tree.

```
refs/tickets/open/<id>        ← ticket: id, freeform_desc, status, thread
refs/tickets/done/<id>        ← closed tickets
refs/tickets/questions/<qid>  ← open questions index
refs/tickets/policy           ← merge thresholds, escalation config
```

No files in `.tickets/`. No merge conflicts on ticket state. `git clone` transfers everything.
