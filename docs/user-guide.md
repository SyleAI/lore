# Lore User Guide

Lore is a coordination layer for autonomous codebases. It gives agents and humans a shared blackboard — versioned in git, queryable as a CLI, observable as an event stream.

**Core idea:** agents pick tickets, execute work, ask questions, and mark ready. Humans define intent, answer questions, and approve. All state lives in git refs alongside the code it produced.

---

## Table of Contents

1. [Setup](#1-setup)
2. [Creating tickets](#2-creating-tickets)
3. [The agent loop](#3-the-agent-loop)
4. [Questions and answers](#4-questions-and-answers)
5. [Human review](#5-human-review)
6. [Situational awareness](#6-situational-awareness)
7. [Multi-ticket features](#7-multi-ticket-features)
8. [Escalation and recovery](#8-escalation-and-recovery)
9. [AI features](#9-ai-features)
10. [Event stream](#10-event-stream)
11. [Maintenance](#11-maintenance)
12. [Command reference](#12-command-reference)

---

## 1. Setup

```bash
# Initialise lore in any git repo
cd your-repo
lore init

# Check everything is working
lore doctor
```

`lore init` creates `.lore/config.yaml` for local configuration. All ticket state goes into git refs — nothing in the working tree.

### Optional: AI features

Add to `.lore/config.yaml` to enable embeddings and consolidation:

```yaml
ai:
  embeddings:
    provider: anthropic   # uses Voyage AI via VOYAGE_API_KEY or ANTHROPIC_API_KEY
    model: voyage-3
  reasoning:
    provider: anthropic
    model: claude-sonnet-4-6
```

All commands work without AI configured. Embedding-dependent features (`consolidate`, semantic similarity in `related` and `context`) degrade gracefully to no-ops.

---

## 2. Creating tickets

```bash
# Minimal ticket
lore new --title "Fix login regression"

# With all fields
lore new \
  --title "Fix login regression" \
  --description "Sessions are not persisting after password reset" \
  --priority 4 \
  --files internal/auth/session.go,internal/auth/token.go

# Edit a ticket in $EDITOR after creation, or later
lore edit t/a3f9bc

# Update fields non-interactively (good for agents)
lore update t/a3f9bc --priority 5
lore update t/a3f9bc --files "internal/auth/session.go,internal/auth/token.go"
```

### Priority

Priority is an integer. The system computes scores in 0–100. Values above 100 are human overrides — the system will not reorder those tickets.

```bash
lore prioritize t/a3f9bc --priority 80      # within system range
lore prioritize t/a3f9bc --priority 150     # human pin — system scoring disabled
lore prioritize t/a3f9bc --urgent           # shorthand for maximum priority
```

### Breaking work into children

```bash
lore new --title "Auth system overhaul" --priority 4
# → t/parent1

lore spawn t/parent1 --title "Migrate session storage to Redis"
lore spawn t/parent1 --title "Implement token rotation"
lore spawn t/parent1 --title "Add brute-force protection"
```

Child tickets carry the parent ID. Closing all children does not auto-close the parent — that remains a human decision.

---

## 3. The agent loop

The reference pattern for an agent session:

### Step 1: Read the room

```bash
lore signals
# System signals (2026-03-22):
#
#   Tickets:
#     open         12
#     in-progress   4
#     blocked       2
#     ready         1
#     closed       47
#
#   Oldest open: t/a3f9bc  "Fix login regression"  (12d)
#   Blocking unanswered questions: 3
#   Active agents: 2
```

### Step 2: Claim a ticket

```bash
# Claim the highest-priority unclaimed ticket atomically
lore pick
# ticket t/a3f9bc claimed: Fix login regression

# Or claim a specific one
lore claim t/a3f9bc
```

`lore pick` is safe under concurrent agents — it uses a git compare-and-swap so two agents can never claim the same ticket.

### Step 3: Read the full briefing

```bash
lore context t/a3f9bc
# ID:       t/a3f9bc
# Title:    Fix login regression
# Status:   in-progress
# Priority: 4
# Agent:    agent-1
# Files:    internal/auth/session.go, internal/auth/token.go
#
# Created:  2026-03-10 09:00:00
# Updated:  2026-03-22 14:30:00
#
# Sessions are not persisting after password reset. Reproduces on
# all browsers. Likely the TTL is not being reset on session refresh.
#
# Checkpoints:
#   [2026-03-10 09:15:00] ticket created
#
# Related (shared files):
#   t/c2d1ef  [in-progress]  Refresh token expiry fix
#
# Semantically similar:
#   t/9a2e3f  [open]         Fix remember-me cookie not persisting
#
# Open questions:
#   q-3a1f [BLOCKING]  Should we invalidate all active sessions on password reset?
```

Always read `lore context` before writing any code. It shows related work, similar past tickets, and open questions — all the things you need to avoid duplicating effort or conflicting with another agent.

### Step 4: Log progress

```bash
lore checkpoint t/a3f9bc "Identified root cause: TTL not reset on session refresh"
lore checkpoint t/a3f9bc "Fixed in internal/auth/session.go:142"
lore checkpoint t/a3f9bc "All auth tests passing"
```

Checkpoints are append-only timestamped notes. Write them frequently — they are the execution trace that future agents and humans read to understand what happened.

### Step 5: Mark ready

```bash
lore ready t/a3f9bc
# ticket t/a3f9bc marked ready for review
```

This moves the ticket to `ready` status and surfaces it in `lore review` for human approval.

---

## 4. Questions and answers

Questions are the structured middle ground between guessing silently and stopping completely.

### Asking questions

```bash
# Non-blocking: state your assumption and keep moving
lore ask t/a3f9bc \
  --type clarification \
  --question "Should error messages include the failed field name or stay generic?" \
  --assumption "Using generic messages for security" \
  --no-blocking

# Blocking: genuinely cannot proceed without an answer
lore ask t/a3f9bc \
  --type constraint_check \
  --question "Is it safe to drop the legacy_token column in this migration?" \
  --blocking

# Knowledge gap: lore checks answered questions in history first
lore ask t/a3f9bc \
  --type knowledge_gap \
  --question "What's the correct way to invalidate JWTs in this codebase?"
# hint: similar answered question found — "Use the TokenBlacklist service, see t/8bc2a1"
```

**Question types:**
- `clarification` — ambiguity in the ticket requirements
- `constraint_check` — safety check before an irreversible action
- `knowledge_gap` — missing domain knowledge (lore searches history first)
- `validation` — asking another agent or human to verify a decision

### Answering questions

```bash
# Human or another agent answers
lore answer t/a3f9bc \
  --question-id q-3a1f \
  --answer "Yes, invalidate all sessions on password reset — security requirement"

# Route a question to the right agent before answering
lore tag t/a3f9bc --question-id q-3a1f --agent agent-security
```

Answers are written back to the ticket permanently. The same question will not surface as a knowledge gap again — future agents searching for the same information will find it.

### Viewing open questions

```bash
# All unanswered questions across all tickets
lore questions

# Only blocking questions
lore questions --blocking

# Questions directed at a specific agent
lore questions --directed-to agent-auth
```

---

## 5. Human review

### What needs attention

```bash
lore review
# Ready for approval (1):
#   TICKET          TITLE
#   t/a3f9bc        Fix login regression
#
# Escalated / blocked (1):
#   TICKET          TITLE
#   t/c2d1ef        Refresh token expiry fix
#
# Blocking unanswered questions (2):
#   TICKET          Q-ID      QUESTION
#   t/c2d1ef        q-4f2a    Is it safe to drop the legacy_token column...
#   t/b9e3cd        q-1a8b    Should we rate-limit password reset attempts?
```

### Approving work

```bash
# Read the full ticket first
lore review t/a3f9bc

# Approve
lore approve t/a3f9bc
# ticket t/a3f9bc approved

# Reject with a reason — returns ticket to in-progress for rework
lore reject t/a3f9bc --reason "Missing test coverage for the TTL propagation path"
```

### Leaving feedback without blocking

```bash
# Add a comment — status unchanged, agent can read it in context
lore comment t/a3f9bc "Check the session cleanup job too — it may have the same TTL bug"
```

### Unblocking a stuck ticket

```bash
lore unblock t/a3f9bc
# ticket t/a3f9bc unblocked
```

---

## 6. Situational awareness

### Before starting work on a ticket

```bash
# What other tickets are touching the same files, or describe similar work?
lore related t/a3f9bc
# Related (shared files):
#   t/c2d1ef  [in-progress]  Refresh token expiry fix
#   t/f1b8cd  [open]         Add session audit logging
#
# Related (semantic):
#   t/9a2e3f  [open]         Fix remember-me cookie not persisting

# Full history of a specific file
lore history internal/auth/session.go
# Tickets touching internal/auth/session.go (4):
#   t/a3f9bc  in-progress  Fix login regression
#   t/c2d1ef  in-progress  Refresh token expiry fix
#   t/7f3a1b  closed       Add OAuth2 provider support
#   t/2c9d4e  closed       Fix concurrent login race condition

# Why is this ticket ranked where it is?
lore why t/a3f9bc
# Priority breakdown for t/a3f9bc: Fix login regression
#
#   Stored priority:       4
#   Status:                in-progress
#   Age:                   12d
#   File-sharing tickets:  3
#   Blocking questions:    1 unanswered
```

### Viewing all tickets

```bash
lore list
lore list --status open
lore list --status blocked
lore list --agent agent-auth
```

### Agent workload

```bash
lore agents
#   AGENT                 TOTAL   OPEN    WIP    BLK    RDY
#   agent-auth                3      1      2      0      0
#   agent-redis               2      0      1      1      0

# Filtered to tickets touching a specific file
lore agents --context internal/auth/session.go
```

### Trend view

```bash
lore signals --trend
# System signals (2026-03-22):
#   ...counts...
#
#   Activity last 7 days (8 tickets):
#     TICKET          STATUS        TITLE
#     t/a3f9bc        in-progress   Fix login regression
#     t/c2d1ef        in-progress   Refresh token expiry fix
#     ...
```

---

## 7. Multi-ticket features

### Assigning tickets

```bash
lore assign t/a3f9bc --agent agent-auth
lore assign t/b9e3cd --agent agent-redis
```

### Closing completed work

```bash
lore close t/a3f9bc
```

Closed tickets are excluded from `lore pick` and `lore list` by default but remain in git history permanently. Their answered questions remain searchable for future knowledge gap resolution.

### Reopening a ticket

```bash
# Fix didn't hold, regression detected
lore reopen t/a3f9bc
# ticket t/a3f9bc moved from closed to open
```

---

## 8. Escalation and recovery

```bash
# Agent cannot proceed — escalate with a reason
lore escalate t/a3f9bc \
  --reason "Session table has 200M rows — migration needs DBA review before proceeding"

# Human adds context
lore comment t/a3f9bc "DBA confirmed: use batched migration with pt-online-schema-change"

# Human clears the block — agent can resume
lore unblock t/a3f9bc
```

Escalated tickets appear in `lore review` and `lore signals`. They do not fall out of the queue — the blocking reason is preserved in the ticket's checkpoint history.

---

## 9. AI features

These require `VOYAGE_API_KEY` or `ANTHROPIC_API_KEY` in the environment.

### Consolidation — find duplicates and related clusters

```bash
lore consolidate
# Cluster 1 (strength: 0.91) — suggested: merge
#   t/a3f9bc  Fix login regression
#   t/9a2e3f  Fix remember-me cookie not persisting
#   Action: merge — both address session persistence after credential change
#   Confidence: 0.88
#
# Cluster 2 (strength: 0.78) — suggested: batch
#   t/f1b8cd  Add session audit logging
#   t/3e7c2a  Add login event tracking
#   Action: batch — related logging work, more efficient done together
#   Confidence: 0.81

# Adjust the similarity threshold (default 0.80)
lore consolidate --threshold 0.70

# Output as JSON for tooling
lore consolidate --json
```

**Actions:**
- `merge` — tickets describe the same intent, close one and fold work into the other
- `batch` — tickets are related, worth doing in one session to share context
- `ignore` — similarity is coincidental, proceed independently

### Semantic context in `lore related`

When AI is configured, `lore related` supplements file-based relationships with embedding similarity — it finds tickets describing similar intent even if they touch different files.

### Knowledge gap resolution

When an agent asks a `knowledge_gap` question, lore automatically searches all answered questions across the full ticket history using embeddings. If a past answer is sufficiently similar, it surfaces a hint before routing to a human.

---

## 10. Event stream

Every command emits a structured JSON event. Pipe it into your own tooling — Slack alerts, deployment triggers, PM system updates, monitoring dashboards.

```bash
# Single command with JSON output
lore pick --json
# {"type":"ticket.claimed","ts":"2026-03-22T14:30:00Z","data":{"ticket_id":"t/a3f9bc","agent":"agent-1"}}

lore ready t/a3f9bc --json
# {"type":"ticket.ready","ts":"2026-03-22T15:45:00Z","data":{"ticket_id":"t/a3f9bc"}}

lore answer t/a3f9bc --question-id q-3a1f --answer "Yes" --json
# {"type":"question.answered","ts":"...","data":{"ticket_id":"t/a3f9bc","question_id":"q-3a1f"}}
```

**All event types:**

| Event | Emitted by |
|---|---|
| `ticket.created` | `lore new`, `lore spawn` |
| `ticket.claimed` | `lore claim`, `lore pick` |
| `ticket.assigned` | `lore assign` |
| `ticket.prioritized` | `lore prioritize` |
| `ticket.checkpointed` | `lore checkpoint` |
| `ticket.escalated` | `lore escalate` |
| `ticket.ready` | `lore ready` |
| `ticket.unblocked` | `lore unblock` |
| `ticket.merged` | `lore close` (approved) |
| `question.asked` | `lore ask` |
| `question.answered` | `lore answer` |
| `question.tagged` | `lore tag` |
| `consolidation.suggested` | `lore consolidate` |

```bash
# Suppress events when you only want clean stdout
lore list --quiet
lore signals --quiet
```

---

## 11. Maintenance

```bash
# Health check and rebuild SQLite index from git refs
lore doctor

# The SQLite index is a disposable cache — safe to delete
# lore doctor reconstructs it from scratch in seconds
rm .lore/graph.db && lore doctor
```

`lore doctor` is safe to run at any time. It reads git refs (the source of truth) and rebuilds the local index. Run it if `lore list` or `lore signals` look wrong, or after pulling from a remote where other agents have been working.

---

## 12. Command reference

### Setup
| Command | Description |
|---|---|
| `lore init` | Initialise lore in current git repo |
| `lore doctor` | Health check and rebuild index |

### Ticket lifecycle
| Command | Description |
|---|---|
| `lore new` | Create a ticket |
| `lore show <id>` | Show ticket detail |
| `lore list` | List tickets |
| `lore edit <id>` | Edit ticket in `$EDITOR` |
| `lore update <id>` | Update fields non-interactively |
| `lore close <id>` | Close a ticket |
| `lore reopen <id>` | Move closed ticket back to open |
| `lore spawn <id>` | Create a child ticket |

### Agent operations
| Command | Description |
|---|---|
| `lore pick` | Atomically claim highest-priority open ticket |
| `lore claim <id>` | Atomically claim a specific ticket |
| `lore assign <id> --agent` | Assign ticket to an agent |
| `lore checkpoint <id>` | Append a timestamped progress note |
| `lore ready <id>` | Mark ticket ready for review |
| `lore escalate <id> --reason` | Mark blocked, surface to humans |
| `lore prioritize <id>` | Override priority (0–200; >100 pins) |

### Questions
| Command | Description |
|---|---|
| `lore ask <id>` | Record a question on a ticket |
| `lore answer <id> --question-id` | Answer a question |
| `lore tag <id> --question-id --agent` | Route question to a specific agent |
| `lore questions` | List questions across all tickets |

### Human review
| Command | Description |
|---|---|
| `lore review` | Show everything awaiting human decision |
| `lore approve <id>` | Approve a ready ticket |
| `lore reject <id> --reason` | Reject back for rework |
| `lore unblock <id>` | Clear escalation, resume ticket |
| `lore comment <id> <message>` | Add a note without changing status |

### Situational awareness
| Command | Description |
|---|---|
| `lore signals` | System health dashboard |
| `lore context <id>` | Full agent briefing for a ticket |
| `lore related <id>` | Tickets related by files or semantics |
| `lore history <file>` | All tickets that touched a file |
| `lore why <id>` | Priority score breakdown |
| `lore agents` | Agent workload summary |

### AI
| Command | Description |
|---|---|
| `lore consolidate` | Find duplicate/related tickets via embeddings |
