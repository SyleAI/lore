# Lore

**Intent execution substrate for autonomous codebases.**

Lore is a shared blackboard with a knowledge graph that emits events, versioned in git. It gives autonomous agents memory, coordination, and continuity on a codebase.

It is not a ticketing system. It is not an agent orchestration framework. It is the layer between human intent and agent execution.

---

## The Problem

Autonomous agents acting on codebases have no shared substrate. They are capable in isolation and chaotic at scale.

- Every agent run starts cold — no memory of what failed, what was decided, or why the code is the way it is.
- Multiple agents claim overlapping files with no awareness of each other.
- There is no middle ground between reviewing everything and trusting blindly.
- Intent evaporates when the prompt window closes.

Lore solves all four.

---

## What Lore Is

### Versioned Intent Store

Tickets live in `refs/tickets/` — a separate ref namespace in the git object store, orthogonal to branches. No merge conflicts on ticket state. `git clone` transfers the complete ticket history.

```
refs/tickets/open/fix-auth-timeout
refs/tickets/closed/2026-03/fix-session-drop
refs/tickets/graph
refs/tickets/policy
refs/tickets/questions/q-014
```

### Knowledge Graph

Computed relationships between everything on the blackboard: which tickets touch the same files, describe similar intent, have co-failed historically, block each other, or collectively imply a structural change nobody named yet. Updated incrementally on every state change.

### Event Emitter

Every state change emits a structured JSON event. Any agentic loop or integration tool is built by consuming this stream.

```json
{ "type": "ticket.created",      "ticket": "fix-auth-timeout",  "ts": "..." }
{ "type": "question.asked",      "ticket": "fix-auth-timeout",  "question_id": "q-014", "ts": "..." }
{ "type": "ticket.merged",       "ticket": "fix-auth-timeout",  "ts": "..." }
{ "type": "regression.detected", "dimension": "test_coverage",  "delta": -0.03, "ts": "..." }
```

---

## Install

```sh
# Homebrew
brew install loreteam/tap/lore

# From source
go install github.com/loreteam/lore@latest
```

Requires Go 1.23+. Single static binary, no CGO.

---

## Quick Start

```sh
# Initialize Lore in a git repository
lore init

# Create a ticket
lore new "fix auth timeout on long sessions"

# List open tickets
lore list

# Claim a ticket (agent takes ownership)
lore claim fix-auth-timeout

# Append to the execution trace (no git side effect)
lore update fix-auth-timeout "investigated token refresh path, found stale TTL config"

# Record structured progress against acceptance criteria
lore checkpoint fix-auth-timeout

# Ask a blocking question
lore ask fix-auth-timeout "should we extend the TTL or add a refresh endpoint?"

# Answer a question
lore answer q-014 "extend the TTL to 24h for now, revisit with refresh endpoint in v2"

# Close a ticket
lore close fix-auth-timeout

# Follow the event stream
lore events --follow
```

---

## Core Commands

| Command | Description |
|---|---|
| `lore init` | Initialize Lore in the current git repo |
| `lore new` | Create a new ticket |
| `lore list` | List tickets (filterable by status, tag, agent) |
| `lore show` | Show full ticket detail |
| `lore claim` | Claim a ticket for execution |
| `lore update` | Append to execution trace (lightweight, no git commit) |
| `lore checkpoint` | Record structured criteria progress |
| `lore close` | Close a ticket with outcome summary |
| `lore ask` | Ask a question on a ticket |
| `lore answer` | Answer an open question |
| `lore escalate` | Escalate a blocked ticket to human |
| `lore pick` | Let Lore select the highest-priority ticket for an agent |
| `lore graph` | Query the knowledge graph |
| `lore related` | Show tickets related to a given ticket |
| `lore prioritize` | Show priority-ranked ticket queue |
| `lore review` | Trigger risk-scored merge review |
| `lore approve` / `lore reject` | Approve or reject a ticket for merge |
| `lore consolidate` | Cluster and deduplicate similar tickets |
| `lore events` | Tail the event log |
| `lore context` | Emit full agent context for a ticket (for prompt injection) |

---

## Agent Integration

Lore exposes a JSON event stream and a context command designed for agent integration:

```sh
# Get structured context for prompt injection
lore context fix-auth-timeout --json

# Watch for events to trigger agent actions
lore events --follow --json | your-agent-loop
```

Agent skill templates ship with Lore and are installed into the repo on `lore init`:

- `.lore/skills/lore-lead.md` — Lead agent: assigns tickets, never codes
- `.lore/skills/lore-worker.md` — Worker agent: executes tickets
- `.lore/skills/lore-observer.md` — Observer agent: continuous improvement mode

---

## Policy and Risk

Merge policy is stored in `refs/tickets/policy`. Risk is computed from blast radius, file heat, and historical failure signals — not commit volume. Humans approve what warrants judgment.

```sh
lore policy show
lore policy edit
lore score fix-auth-timeout   # compute risk score
```

---

## Design Principles

- **Git refs, not files** — no merge conflicts, full portability, clone transfers everything
- **Blackboard architecture** — agents coordinate through shared state, not peer-to-peer conversation
- **Events as the contract** — Lore does not implement the agentic loop; it emits events
- **Emergent priority** — computed from dependency depth, failure history, file heat, age/drift, and blast radius
- **Questions as first-class operations** — blocking, non-blocking, and AI-resolved variants
- **Split storage** — stubs and summaries in git refs; large logs and embeddings in external store

---

## License

MIT
