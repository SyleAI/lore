# Lore — Intent Execution Substrate for Autonomous Codebases

**Full System Specification v0.1**

---

## What is Lore?

Lore is a shared blackboard with a knowledge graph that emits events, versioned in git, giving autonomous agents memory, coordination, and continuity on a codebase.

It is not a ticketing system. It is not an agent orchestration framework. It is not a VCS. It is the layer that sits between human goals and agent execution — the operating system for autonomous codebases.

| Property | Value |
|---|---|
| Primary consumer | Agents (humans secondary) |
| Storage primitive | Git refs — versioned, distributed, cloneable |
| Coordination model | Blackboard — agents read/write shared state, no direct chat |
| Intelligence layer | Domain-specific knowledge graph over tickets, files, agents |
| Human role | Define intent, set policy, approve risk — not manage tasks |
| Agent integration | Any system that can run shell commands and parse JSON |

---

## 1. The Problem Lore Solves

Autonomous agents acting on codebases have no shared substrate. They are capable in isolation and chaotic at scale. Every other problem is a consequence of this one.

**Without Lore:**
- Every agent run starts cold. No memory of what failed, what was decided, or why the code is the way it is.
- Multiple agents claim overlapping files with no awareness of each other. Conflicts are discovered after the fact, not prevented.
- There is no middle ground between reviewing everything (does not scale) and trusting blindly (does not work).
- Intent evaporates when the prompt window closes. The code exists. The reasoning that produced it does not.

**With Lore:**
- Every agent run reads the complete execution history of prior runs before starting.
- The ticket graph is the coordination layer. Agents coordinate through shared state, not direct conversation.
- Risk is computable. Humans review what warrants judgment, not every commit.
- Intent lives in git refs alongside the code it produced. Clone the repo, get the complete reasoning history.

> **Core insight:** Lore does not make agents smarter. It gives them continuity, coordination, and a structured relationship with the humans who need to trust them.

---

## 2. What Lore Is

Lore is three things composed into one tool.

### 2.1 Versioned Intent Store

Tickets live in git refs alongside code. Intent is first-class, versioned, portable, and permanent. The ticket and the code it produced are the same artifact viewed from two angles. You cannot have one without the other because they live in the same git object store.

### 2.2 Knowledge Graph

Lore continuously computes relationships between everything on the blackboard. The graph understands:

- Which tickets touch the same files (spatial relationships)
- Which tickets describe similar intent (semantic relationships)
- Which tickets have co-failed historically (historical relationships)
- Which tickets block or depend on each other (causal relationships)
- Which tickets collectively imply a structural change nobody named yet (compositional relationships)
- Which agents have context affinity for which file areas (agent-file relationships)

This is not a general-purpose knowledge graph. It is purpose-built for software development. The specificity is the value.

### 2.3 Event Emitter

Every state change emits a structured JSON event. This event stream is the primary contract Lore exposes to the outside world. Any agentic loop, orchestration framework, or integration tool is built by consuming this stream — Lore has no opinion about what is on the other end.

```json
{ "type": "ticket.created",     "ticket": "fix-auth-timeout", "ts": "..." }
{ "type": "question.asked",     "ticket": "fix-auth-timeout", "question_id": "q-014", "ts": "..." }
{ "type": "ticket.merged",      "ticket": "fix-auth-timeout", "ts": "..." }
{ "type": "regression.detected","dimension": "test_coverage",  "delta": -0.03, "ts": "..." }
```

---

## 3. Key Design Decisions

### 3.1 Git Refs, Not Files

Tickets do not live in `.tickets/` in the working tree. They live in `refs/tickets/` — a separate ref namespace in the git object store.

- Branches do not carry ticket files. No merge conflicts on ticket state.
- `git clone` transfers all refs including `refs/tickets/`. Complete ticket history travels with the repo.
- Tickets are genuinely repo-wide — orthogonal to branches, not entangled with them.
- `graph.index` is derived from ticket refs. Updated incrementally — a node addition or removal triggers reprocessing of that node only, not a full recompute.

```
refs/tickets/open/fix-auth-timeout        ← stub: intent, criteria, status
refs/tickets/open/add-rate-limiting
refs/tickets/closed/2026-03/fix-session-drop  ← summary: outcome, key decisions
refs/tickets/graph                        ← computed relationship index
refs/tickets/policy                       ← merge thresholds, escalation config
refs/tickets/questions/q-014             ← open questions across all tickets
```

### 3.2 Split Storage Model

Not everything belongs in git. The repo stores the minimum necessary to reconstruct intent and outcome. Everything else lives in an external store accessed by reference.

**In git refs (travels with clone):**
- Ticket stubs — intent, criteria, current status (~2KB each)
- Ticket summaries — outcome, key decisions only (~1KB each)
- `graph.index` — relationship map, priority scores
- `policy.yaml` — merge thresholds, escalation config

**External store (fast, queryable, not cloned by default):**
- Full execution logs — every action and rationale
- Complete question threads
- Large artifacts — test outputs, diffs, dumps
- Semantic index — embeddings for similarity search

### 3.3 Emergent Priority, Not Declared Priority

Priority is not set by humans. It is continuously computed from five signals:

- **Dependency depth** — how many tickets are blocked waiting for this one
- **Failure history** — how many prior attempts have failed (repeated failure raises priority, not lowers it)
- **File heat** — churn rate and regression frequency of touched files
- **Age and drift** — time since ticket was written; gap between intent and current codebase state
- **Blast radius** — downstream impact if this ticket is never resolved

Priority override exists for genuine emergencies. It bypasses the score and goes to the front unconditionally. It is not the default mechanism.

### 3.4 Questions as First-Class Operations

Agents have three choices at any uncertainty: act confidently, ask a question, or escalate. Questions are the missing middle ground between guessing silently and stopping completely.

- **Non-blocking questions:** agent states assumption and continues. Answer arrives asynchronously.
- **Blocking questions:** agent parks that specific step but continues everything else in parallel.
- Lore attempts AI resolution from closed ticket history before routing to another agent or human.
- Answers are written back to the ticket permanently. The same question will not be asked twice.

### 3.5 Risk-Tiered Merge, Not Volume-Tiered Review

Humans do not review every commit. They review commits above a risk threshold. Risk is computed from: confidence trajectory, file heat, blast radius, prior failure history, and whether protected paths were touched. Low-risk branches auto-merge. High-risk branches gate on human approval.

### 3.6 Blackboard Architecture, Not Peer-to-Peer

Agents do not talk to each other directly. All coordination flows through Lore. This is a deliberate bias toward observability over expressiveness. For systems making autonomous decisions about production code, every coordination decision must be recorded.

Peer-to-peer conversations that influence codebase decisions and are not written back to Lore create an incomplete institutional memory. If your agentic system uses peer-to-peer communication, write conversation outcomes back to the ticket log explicitly before acting on them.

### 3.7 The queue is the graph

`lore pick` is not a FIFO queue. It reads the graph, scores all open unblocked unclaimed tickets, and returns the highest. Only two queue-level concepts exist on top of this:

- **Priority override** — bypasses scoring for genuine emergencies
- **Batch pick** — `lore pick --batch` returns a consolidation batch when one is ready

Everything else — priority lanes, starvation prevention, fairness, dead letter handling — is handled by the graph scoring model. There is one source of truth for what to work on next.

---

## 4. Ticket Anatomy

A ticket is not a task card. It is a contract, a live execution trace, and a communication channel simultaneously.

### 4.1 The Ticket Schema (what the agent reads)

```yaml
id:                   fix-auth-timeout
intent:               Auth tokens expire silently after 15min, users lose work
acceptance_criteria:
  - token refresh happens automatically before expiry
  - no visible session interruption
  - regression test covers the refresh window
constraints:
  - do not modify JWT secret rotation schedule
  - production env only via staging first
scope:                src/auth/
confidence_threshold: 0.85
escalation_path:      "@platform-team"
related:              fix-session-drop (closed), add-rate-limiting (open)
log: []
```

### 4.2 The Execution Trace (what the agent writes)

The agent writes continuously throughout execution — not just at start and end, but at every meaningful decision point.

```yaml
log:
  - ts: 2026-03-21T09:12Z
    action: branch
    confidence: 0.91

  - ts: 2026-03-21T09:14Z
    action: read
    target: src/auth/token.py
    observation: "TokenManager.validate() has no refresh hook. Expiry checked but not pre-empted."

  - ts: 2026-03-21T09:17Z
    action: modified
    target: src/auth/token.py
    commit: 3fa2c1
    rationale: "Added refresh_if_expiring() 60s before TTL. Mirrors fix-session-drop pattern."
    confidence: 0.91

  - ts: 2026-03-21T09:19Z
    action: test_run
    result: pass
    criteria_met: 2/3
    remaining: ["regression test for refresh window"]

  - ts: 2026-03-21T09:20Z
    action: decision
    considered:
      - "Extend token TTL globally — rejected: violates JWT rotation constraint"
      - "Client-side polling — rejected: outside scope src/auth/"
      - "Server-side proactive refresh — chosen"
    rationale: "Only option within scope and constraints. Consistent with prior fix."

  - ts: 2026-03-21T09:22Z
    action: confidence_update
    from: 0.91
    to: 0.94
    reason: "Regression test passed. All criteria met. No unexpected side effects."

  - ts: 2026-03-21T09:23Z
    action: observation
    note: "refresh_if_expiring() adds ~2ms latency to every token validation call"
    impact: low
    spawned_ticket: optimize-token-validation-latency

status: ready_to_merge
final_confidence: 0.94
criteria_met: 3/3
risk_score: 0.18
diff_summary: "+47 -12 lines across 2 files"
affects: [add-rate-limiting]
spawned: [optimize-token-validation-latency]
```

### 4.3 The Five Things a Ticket Serves As

1. **A briefing document** — everything the agent needs to start, including full context from prior attempts
2. **Working memory** — state that survives interruption; next agent picks up from last checkpoint entry
3. **A reasoning trace** — every decision with options considered and reason for rejection of alternatives
4. **A communication channel** — async Q&A between agents and humans; neither has to be online simultaneously
5. **A work generator** — agents spawn child tickets for out-of-scope observations; the backlog grows from execution not just from planning

### 4.4 Question Types

Questions are not escalations. An escalation says "I cannot continue." A question says "I can continue but would be more confident with this specific information."

```yaml
# clarification — intent is ambiguous, non-blocking
- ts: 2026-03-21T09:15Z
  action: question
  type: clarification
  question: "Should refresh happen silently on every request, or only when client asks?"
  blocking: false
  assumption: "Proceeding with silent server-side refresh. Will revise if incorrect."
  confidence_if_wrong: 0.55

# answered — by human, another agent, or AI resolver from history
- ts: 2026-03-21T10:34Z
  action: answer
  from: human · @sara
  answer: "Silent server-side is correct."
  confidence_delta: +0.08
```

Question types:
- `clarification` — ambiguous intent, non-blocking, agent states assumption
- `constraint_check` — scope boundary question, can be blocking on that specific step
- `knowledge_gap` — missing codebase context, Lore searches history before routing
- `validation` — soft consent before merging, timeout-driven, default-to-action

---

## 5. System Architecture

### 5.1 Reference Agent Roles

The following roles describe the reference template loops that ship alongside Lore — they are not part of core Lore. Core Lore is the CLI and the event stream. These roles describe one way to use them. Users can define entirely different agent structures on top of the same primitives.

**Lead agent** — reads full graph on each cycle, assigns work by context affinity, answers questions from graph before routing, detects emergent refactor patterns, escalates to human with structured reasoning. Never touches code.

**Worker agents** — execute one ticket at a time, log every action with rationale, track confidence, ask questions correctly, stay within scope, spawn child tickets for out-of-scope observations, call `lore ready` only when all criteria are met.

**Observer** (improvement mode only) — scans codebase against `improvement.yaml`, generates candidate tickets with expected improvement deltas, measures delta after every merge, detects regressions, recognises diminishing returns.

**Human** — writes intent, sets policy, approves architectural changes and high-risk merges, answers questions neither lead nor workers can resolve from history.

### 5.2 Full Event Surface

All coordination is reactive. No cron jobs. State changes propagate immediately.

```
ticket.created          ticket.assigned         ticket.claimed
ticket.checkpointed     ticket.escalated        ticket.ready
ticket.merged           ticket.unblocked
question.asked          question.answered       question.tagged
consolidation.suggested regression.detected     pattern.emerged
agent.tagged
```

### 5.3 Agent Loop

Lore does not implement the agentic loop. Lore emits events. Any loop, orchestration framework, or pipeline is built by consuming those events.

A reference worker loop (Claude Code) ships separately as a template — not as part of core Lore. That template shows one way to compose Lore's events and CLI into an autonomous worker: pick a ticket, execute it, pick the next one, with a background listener handling questions and unblocks in parallel. Users are free to ignore it and build whatever loop fits their system.

### 5.4 Claude Skills (ship with lore init)

Three skill files are created by `lore init` and read automatically by Claude Code at session start. These skills encode the behaviour for the reference template loops. They are the suggested starting point — users can modify or replace them entirely.

**`.claude/skills/lore-worker.md`** — reference behaviour for a worker agent:
- Read `lore context` before writing a single line of code
- Log every file read, every file modification, every decision between alternatives
- Track confidence score throughout; escalate if it drops below ticket threshold
- Ask questions rather than guess; state assumption; proceed on non-blocking questions
- Never modify files outside declared scope without constraint_check
- Spawn child tickets for out-of-scope observations instead of fixing them or ignoring them
- Call `lore ready` only when `criteria_met` equals total criteria count

**`.claude/skills/lore-lead.md`** — reference behaviour for a lead agent:
- Run `lore signals`, `lore graph --summary`, `lore consolidate`, `lore review`, `lore list --open` at start of every cycle
- Assign by context affinity — prefer agents with recent history in relevant files
- Answer questions from graph before routing to another agent or human
- After every 5 closures, run `lore signals --trend` and check for emergent refactor patterns
- Propose refactor tickets with `--requires-human-approval true`
- Escalate to human when: refactor scope > 30% codebase, opposing intents in two tickets, own confidence below 0.75

**`.claude/skills/lore-observer.md`** — reference behaviour for an observer agent:
- Measure all dimensions in `improvement.yaml` after every merge
- Generate candidate tickets with current value, target value, expected delta, evidence, affected files
- Report regressions immediately as `regression.detected` events
- Recognise diminishing returns and surface stopping condition hits to human

### 5.5 AI Integration

Lore uses AI internally for exactly three things. Everything else is deterministic computation.

- **Semantic similarity** — embedding comparison for consolidation cluster detection
- **Consolidation reasoning** — classifying clusters as batch / merge / ignore with rationale and confidence
- **Knowledge gap resolution** — semantic search over closed ticket history to answer agent questions; if resolver confidence is below threshold, routes directly to human

All AI calls use structured JSON prompts and return structured JSON. The model is a component, not the system. Users never write or maintain prompts. Swapping models is a config change.

```yaml
# .lore/config.yaml
ai:
  embeddings:
    provider: anthropic     # anthropic | openai | local | custom
    model: voyage-3
    dimensions: 1024
  reasoning:
    provider: anthropic
    model: claude-sonnet-4-6
    max_tokens: 1024
  local_fallback:
    enabled: true
    model_path: ~/.lore/models/nomic-embed.gguf

events:
  adapter: stdout           # stdout | webhook | unix-socket | redis
```

### 5.6 graph.index Structure

```yaml
version: 2
updated: 2026-03-21T09:55Z

tickets:
  fix-auth-timeout:
    status: closed
    priority_score: 9.1
    age_days: 1
    attempts: 1
    touches: [src/auth/token.py, tests/auth/test_token.py]
    worked_by: [agent-A]
    similar_to:
      fix-session-drop: 0.82
    blocks: []

files:
  src/auth/token.py:
    heat: 0.71
    churn_30d: 8
    regressions_90d: 3
    open_tickets: []
    closed_tickets: [fix-auth-timeout, fix-session-drop]

clusters:
  batch-middleware-001:
    type: spatial
    strength: 0.94
    members: [add-rate-limiting, improve-api-logging]
    status: suggested
```

`graph.index` is a materialised cache of relationships — a computed view over ticket refs, not the source of truth. Updates are incremental: a ticket update or close triggers reprocessing of that ticket's node and its edges only. If deleted, `lore doctor` rebuilds it from ticket refs and git history.

### 5.7 policy.yaml Structure

```yaml
merge:
  automerge_risk_ceiling: 0.20
  require_human_above: 0.60
  validation_window_seconds: 600
  protected_paths:
    - src/billing/
    - src/auth/secrets.py
    - infra/

agents:
  default_confidence_threshold: 0.80
  max_concurrent: 5
  max_attempts_per_ticket: 3
  escalate_after_attempts: 2

risk:
  weights:
    diff_size: 0.20
    file_heat: 0.35
    protected_path_touched: 0.30
    prior_failures: 0.15

consolidation:
  spatial_threshold: 0.80
  semantic_threshold: 0.75
  run_every_seconds: 300

escalation:
  default_path: "@platform-team"
  billing_path: "@billing-lead"
  security_path: "@security"

questions:
  agent_answer_window_seconds: 30
  ai_resolver_confidence_threshold: 0.80
  human_notify_via: [slack, email, lore_review]
  blocking_question_urgency: high
  nonblocking_question_urgency: low
```

---

## 6. CLI Reference

`lore` is the single interface for humans and agents. Human commands are mostly nouns. Agent commands are mostly verbs. The ticket is implied — `lore list` not `lore list tickets`.

**Legend:** `[H]` = human, `[A]` = agent, `[B]` = both

### Setup

```
lore init                              [H]  Initialise lore in a git repo
lore init --improvement                [H]  Also scaffolds improvement.yaml
lore doctor                            [B]  Validate refs, graph integrity, config
lore policy                            [B]  Show current policy
lore policy set <key> <value>          [H]  Update a policy value
```

### Ticket Lifecycle

```
lore new                               [H]  Interactive ticket creation wizard
lore new --from "..."                  [B]  Create ticket from natural language
lore new --file spec.md                [B]  Create ticket from markdown spec
lore new --type refactor               [B]  Refactor ticket — requires human approval
lore new --type improvement            [A]  Observer-generated improvement candidate
lore edit <id>                         [H]  Open ticket in $EDITOR
lore close <id>                        [H]  Manually close and archive
lore reopen <id>                       [H]  Move closed ticket back to open
lore split <id>                        [B]  Decompose ticket into subtasks
lore prioritize <id> --urgent          [H]  Priority override — bypasses score
```

### Inspection

```
lore status                            [B]  Live view — agents, tickets, escalations
lore show <id>                         [B]  Full ticket — intent, criteria, log, relationships
lore list                              [B]  Open tickets sorted by priority score
lore list --closed                     [B]  Closed tickets, most recent first
lore list --type refactor              [B]  Filter by ticket type
lore list --agent <id>                 [B]  Tickets for a specific agent
lore log <id>                          [B]  Execution log for a ticket
lore log <id> --reasoning              [B]  Full agent rationale at each step
lore diff <id>                         [B]  Combined diff for a ticket's branch
lore history <file>                    [B]  All tickets that ever touched a file
lore why <id>                          [B]  Priority score with full signal breakdown
```

### Agent Operations

```
lore pick                              [A]  Pick highest priority ticket — atomic lock
lore pick --batch                      [A]  Pick consolidation batch if ready
lore pick --scope <path>               [A]  Pick from tickets within file scope
lore claim <id>                        [A]  Claim specific ticket, create branch
lore context <id>                      [A]  Full context — history, related, signals
lore update <id>                       [B]  Append to ticket trace — freeform if no flags; structured with --action, --rationale, --confidence
lore checkpoint <id>                   [A]  Mark criteria progress, emit confidence
lore ready <id>                        [A]  Signal done, trigger policy engine
lore escalate <id>                     [A]  Flag blocked, surface to lead or human
lore spawn <id>                        [A]  Create child ticket from observation
```

### Questions

```
lore ask <id> --type clarification     [A]  Ambiguous intent — non-blocking
lore ask <id> --type constraint_check  [A]  Scope boundary — can be blocking
lore ask <id> --type knowledge_gap     [A]  Missing context — searches history first
lore ask <id> --type validation        [A]  Soft consent before merge — timeout-driven
lore answer <id> --question-id <qid>   [B]  Answer a question
lore questions                         [B]  All open questions across all tickets
lore questions --unanswered            [B]  Only unresolved
lore questions --mine                  [B]  Questions directed at you
```

### Human Review

```
lore review                            [H]  Everything awaiting human decision
lore review <id>                       [H]  Full escalation detail with trace
lore approve <id>                      [H]  Approve high-risk branch or refactor
lore reject <id> --reason "..."        [H]  Reject — reason written to log permanently
lore comment <id> "..."                [H]  Add guidance for agent to continue
lore unblock <id>                      [H]  Clear escalation, agent resumes
```

### Graph and Consolidation

```
lore graph                             [B]  Ticket relationship and dependency graph
lore graph --summary                   [B]  High-level state — for lead agent cycle
lore related <id>                      [B]  Spatially, semantically, historically related
lore consolidate                       [B]  Run analysis, surface suggestions
lore merge <id> <id>                   [H]  Merge two tickets — inherits both histories
lore batch <id> <id>                   [H]  Schedule spatial batch for single session
```

### Lead Agent

```
lore assign <id> --agent <id>          [A]  Lead assigns ticket with reason
lore tag <id> --question-id <qid> --agent <id>  [A]  Route question to specific agent
lore agents                            [B]  All agents — status, ticket, success rate
lore agents --context <path>           [A]  Agents ranked by context affinity
lore signals                           [B]  System health — failures, pressure, drift
lore signals --trend                   [A]  Pattern analysis over recent closures
```

### Improvement Mode

```
lore measure                           [A]  Score codebase against improvement.yaml
lore measure --delta                   [A]  What changed since last run
lore measure --history                 [B]  Improvement score over time
lore candidates                        [A]  Observer-generated opportunities
lore candidates --generate             [A]  Force fresh scan
```

### Events and Runtime

```
lore events --follow                   [B]  Stream all events as newline-delimited JSON
lore events --filter <type>            [B]  Subscribe to specific event types
lore ui                                [H]  Open local dashboard in browser
```

---

## 7. What Ships vs What Users Build

### Ships with Lore (zero config to get value)

- Git ref storage, versioning, push/pull sync
- Knowledge graph — priority scoring, file heat, relationship detection, cluster formation
- Event bus — all events emit from every lore command automatically
- Policy and risk engine — sensible defaults, user overrides in `policy.yaml`
- AI resolver — consolidation, knowledge gap resolution, emergent ticket detection
- Full CLI surface
- Three Claude skills — `lore-worker.md`, `lore-lead.md`, `lore-observer.md`
- `lore ui` — local dashboard, no server required
- Default `improvement.yaml` with sensible dimensions

### Users Build on Top (domain-specific value)

- **Agentic loops** — the pick-execute cycle is your code, not Lore's. A reference Claude Code loop template ships separately as a starting point.
- **Repo-specific skill extension** (`lore-repo.md`) — codebase conventions, testing patterns, naming rules, domain concepts. Highest leverage item for agent output quality.
- **`improvement.yaml` tuning** — custom dimension weights and targets
- **Custom measurement scripts** — domain-specific metrics the observer calls
- **Escalation integrations** — Slack, PagerDuty, email. Thin consumers of `ticket.escalated`.
- **External event consumers** — post to Slack on refactor proposal, trigger deploys, update PM tools
- **Observer strategies** — custom triggers beyond schedule and post-merge
- **Multi-repo coordination** — advanced; bridges Lore event streams across repos

> **Minimum to get value:** `lore init` → `lore new --from "your first ticket"` → consume `lore events --follow` in your loop

---

## 8. Agent System Compatibility

Lore is agent-agnostic at the infrastructure level. Any system that can run shell commands and parse JSON can integrate. No SDK, no API key, no library to import.

| System | Fit | What you build |
|---|---|---|
| Claude Code | First-class — reference loop template ships separately | Repo-specific skill extension + optional loop customisation |
| Cursor / Windsurf | Good — CLI works natively | Thin loop script + adapted skill |
| LangGraph | Good — wrap lore commands as tools (~8 lines each) | Tool wrappers, agent graph |
| CrewAI | Good — same tool wrapper pattern | Tool wrappers, crew role definitions |
| Custom agents | Full support — CLI + events is the complete interface | Loop script, prompt engineering |
| CI/CD pipelines | Partial — `lore pick` and `lore ready` work as steps | Pipeline yaml, trigger conditions |
| Peer-to-peer MAS | Partial friction | Write conversation outcomes back to ticket log |

**Bias note:** Lore is biased toward blackboard architectures. This is intentional. For systems making autonomous decisions about production code, complete observability of every coordination decision is a safety property, not a preference. Peer-to-peer conversations that influence codebase decisions and are not written back to Lore create an incomplete institutional memory.

### Building a loop on top of Lore

The pattern is the same regardless of the framework: wrap Lore CLI commands as tools, consume the event stream with a background listener, and use the skill content as your system prompt. The pick-execute loop is your code, not Lore's.

```python
# example: lore as LangGraph tools
@tool
def lore_pick() -> dict:
    """Pick the highest priority eligible ticket."""
    result = subprocess.run(['lore', 'pick', '--json'], capture_output=True)
    return json.loads(result.stdout)

@tool
def lore_update(ticket_id: str, action: str, rationale: str, confidence: float):
    """Append a structured entry to the ticket execution trace."""
    subprocess.run(['lore', 'update', ticket_id,
        '--action', action, '--rationale', rationale,
        '--confidence', str(confidence)])

@tool
def lore_ask(ticket_id: str, question: str, type: str, blocking: bool, assumption: str):
    """Ask a question. Lore attempts AI resolution from history first."""
    subprocess.run(['lore', 'ask', ticket_id,
        '--type', type, '--question', question,
        '--blocking', str(blocking).lower(),
        '--assumption', assumption])
```

---

## 9. Self-Improving Mode

When `lore init --improvement` is used, Lore supports a third architecture: the system continuously observes the codebase, generates improvement candidates, and executes them without human-written tickets.

### 9.1 What Changes

In ticket-driven mode, humans define what better means. In self-improving mode, the system defines it — constrained by `improvement.yaml` which humans control.

The feedback loop: every change is measured after it lands. Did the dimension score improve? Did anything regress? That evaluation informs what the system pursues next. Without this, the system is self-modifying, not self-improving.

### 9.2 improvement.yaml

```yaml
dimensions:
  code_quality:
    weight: 0.30
    metrics: [cyclomatic_complexity, test_coverage, duplication_ratio]
    targets:
      cyclomatic_complexity: "< 10 per function"
      test_coverage: "> 0.85"
      duplication_ratio: "< 0.05"
  performance:
    weight: 0.25
    metrics: [p99_latency, memory_footprint]
    targets:
      p99_latency: "< 200ms on critical paths"
    measurement: tests/benchmarks/
  technical_debt:
    weight: 0.25
    metrics: [coupling_score, module_age, churn_vs_stability]
  consistency:
    weight: 0.20
    metrics: [pattern_adherence, naming_convention_score]

constraints:
  never_regress: [test_coverage, security_score]
  human_approval_required_for: [architectural_changes, dependency_additions]

stopping_conditions:
  all_dimensions_above: 0.90
  marginal_gain_below: 0.02
  consecutive_regressions: 3
  human_review_required: true
```

### 9.3 Candidate Ticket Format (observer-generated)

```yaml
id: reduce-auth-complexity-001
type: improvement
source: observer
dimension: code_quality
metric: cyclomatic_complexity
current_value: 18.3
target_value: 9.0
affected_files: [src/auth/token.py, src/auth/session.py]
expected_improvement: 0.12
confidence: 0.81
evidence:
  - TokenManager.validate() has complexity 24
  - SessionStore.refresh() has complexity 19
  - Both have no tests covering complex branches
rationale: "High complexity in auth module correlates with 3 recent regressions"
requires_human_approval: false
```

### 9.4 Stopping Conditions (v2)

`improvement.yaml` supports stopping conditions (`all_dimensions_above`, `marginal_gain_below`, `consecutive_regressions`). Behaviour when a stopping condition is hit is deferred to v2.

### 9.5 Metric Gaming Risk

The most dangerous failure mode is optimising for measurable proxies of quality rather than actual quality. Mitigations:
- Use multiple metrics per dimension — you cannot game all of them simultaneously
- Pair coverage with mutation testing score
- Observer reports actual changes, not just metric deltas — gaming is visible
- Human spot-checks on sampled improvements before the system continues

---

## 10. How Lore Differs from Existing Systems

### vs Conventional Ticketing (Jira, Linear)

| | Conventional | Lore |
|---|---|---|
| Primary consumer | Human | Agent (human secondary) |
| Relationship to code | External, linked by string | Same git object store |
| What a ticket is | Record of human decisions | Live execution trace |
| Written by | Humans manually | Agents continuously |
| Closing a ticket | Status update in database | Atomic ref transition on merge |
| Closed tickets | Archive, rarely consulted | Active knowledge base, always queried |
| Cross-ticket intelligence | Manual — humans search | Automatic — graph continuously computed |
| System learns over time | No | Yes — corpus informs every new ticket |

### vs Generic Blackboards

Generic blackboards are coordination primitives. Lore is what you build when you take that primitive and make it useful for a specific domain.

- Generic blackboards use databases or message brokers. Lore uses git — cloneable, branchable, permanent, cryptographically verified.
- Generic blackboards store entries. Lore's graph understands software development relationships: file heat, agent context affinity, causal dependencies, emergent refactor patterns.
- Generic blackboards have no concept of human oversight. Lore has it as a first-class architectural concern.
- Generic blackboards have no memory between problems. Lore's closed tickets actively inform every new piece of work.

### vs Agent Orchestration Frameworks (LangGraph, CrewAI)

Orchestration frameworks tell agents how to execute at runtime. Lore tells agents what to execute and why, with full historical context. Orchestration frameworks are the runtime. Lore is the substrate the runtime operates on. They are complementary, not competing.

---

## 11. Quick Start for Claude Code

```bash
# install (Homebrew)
brew install yourusername/tap/lore

# install (apt)
sudo apt install lore  # after adding the apt repo, or download .deb from GitHub Releases

# install (GitHub Releases — any platform)
# download the appropriate binary from https://github.com/yourusername/lore/releases

# init repo
cd my-project
lore init
# creates refs/tickets/, .lore/config.yaml
# creates .claude/skills/lore-worker.md
# creates .claude/skills/lore-lead.md

# write tickets
lore new --from "Fix auth tokens expiring silently after 15 minutes"
lore new --from "Add rate limiting to all API endpoints"
lore new --from "Improve error messages in the payment flow"

# start the event stream — build your loop on top of this
lore events --follow

# human workflow
lore status                     # what's happening
lore review                     # what needs your attention
lore answer fix-auth-timeout --question-id q-014 --answer "Silent server-side refresh is correct"
lore approve fix-auth-timeout   # approve high-risk merge
lore ui                         # open dashboard
```

A reference worker loop for Claude Code ships as a separate template. It shows the standard pick-execute pattern and is the fastest path to a running autonomous workflow — but it is not part of core Lore.

---

## 12. Implementation

### Language and Distribution

Lore is written in Go (1.23+). It compiles to a single static binary with no runtime dependency. Distributed via:
- Homebrew (macOS and Linux)
- apt/deb, rpm, apk via nFPM + goreleaser
- GitHub Releases (pre-compiled binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64)

### Key Libraries

| Purpose | Package |
|---|---|
| CLI framework | `github.com/spf13/cobra` + `github.com/spf13/viper` |
| Git operations | `exec.Command` to system git binary (CAS requires `git update-ref --stdin -z`) |
| SQLite (graph cache) | `modernc.org/sqlite` — pure Go, no CGO, cross-compiles correctly |
| YAML | `gopkg.in/yaml.v3` |
| Anthropic SDK | `github.com/anthropics/anthropic-sdk-go` |
| Interactive prompts | `github.com/charmbracelet/huh` |
| Terminal styling | `github.com/charmbracelet/lipgloss` |
| TUI event stream | `github.com/charmbracelet/bubbletea` |
| Distribution | `goreleaser` + `nFPM` |

### Why `exec.Command` for Git, Not go-git

go-git does not fully implement the compare-and-swap form of `git update-ref` (`verify <ref> <old-sha>` in a ref-transaction). This CAS is the atomic primitive for `lore pick` — multiple agents racing to claim the same ticket. Falling back to the system git binary is the only way to guarantee atomicity across processes.

The CAS call:
```
git update-ref --stdin -z <<< "verify <ref>\0<old-sha>\0update <ref>\0<new-sha>\0"
```
Succeeds atomically or fails entirely. No other mechanism provides this guarantee.

### Storage Layout

```
.git/
  refs/tickets/
    open/<id>              ← ticket stub YAML blob
    closed/<year-month>/<id> ← ticket summary YAML blob
    graph                  ← graph.index YAML blob (incremental, not full recompute)
    policy                 ← policy.yaml blob
    questions/<qid>        ← question YAML blob

.lore/
  config.yaml              ← local config (working tree)
  graph.db                 ← SQLite graph mirror (gitignored, rebuilt by lore doctor)
  events.log               ← append-only event log (gitignored)
```

The SQLite DB is a disposable cache. If deleted or corrupted, `lore doctor --rebuild-graph` reconstructs it from the git refs in seconds.

### Atomic Pick

`lore pick` uses a two-layer atomicity strategy:
1. Read top-N priority candidates from SQLite
2. For each candidate: attempt a git ref CAS (`update-ref --stdin -z`). If the CAS fails (another agent claimed it first), move to the next candidate.

No in-process mutex. Correct across concurrent processes on the same machine and across machines sharing a git remote.

### Event Emission

Every mutating command calls `event.Emit()` at the end. The emitter fans out to all configured adapters concurrently, each with a 5-second timeout. Adapter failure is logged to stderr but never fails the command — the event stream is best-effort from the command's perspective.

```
stdout adapter   → JSON line to process stdout (default)
unix-socket      → writes to .lore/events.sock if it exists
webhook          → HTTP POST to configured URL
```

---

## 13. Graph Extensibility (Planned)

### 13.1 Current State

The graph is fixed-shape. Entities (TicketNode, FileNode, ClusterNode) and relationships (blocks/blocked_by, ticket↔file, cluster membership) are hardcoded in `internal/graph/build.go`. The scoring formula is also fixed — four signals combined linearly, with weights exposed via policy.

What users can control today:
- Scoring weights (`lore policy set scoring.*`) — shift how much each signal contributes
- Consolidation strategy and threshold — controls cluster formation
- Ticket `files:` and `parent:` fields — the only inputs to relationship detection

What users cannot control:
- What relationship types exist
- What signals feed into the score
- How file heat is computed (currently: ticket count, not git churn)
- Any traversal or query over the graph

### 13.2 Extension Points to Add

**Custom scoring signals** — let users define additional named signals in policy, each backed by a shell command that takes a ticket ID and emits a float. The scoring engine calls each, multiplies by its weight, and adds to the raw score.

```yaml
# policy.yaml
scoring:
  custom_signals:
    - name: ci_failure_pressure
      command: ".lore/signals/ci-pressure.sh"
      weight: 10
    - name: customer_severity
      command: ".lore/signals/severity.sh"
      weight: 25
```

**Custom relationship extractors** — a list of shell commands in `.lore/config.yaml` that `lore graph update` invokes after the built-in pass. Each command receives the full ticket list on stdin as NDJSON and emits `from_id,to_id,type` lines. Lore merges these into the index as typed edges on TicketNode.

```yaml
# .lore/config.yaml
graph:
  extractors:
    - ".lore/extractors/shared-owner.sh"
    - ".lore/extractors/api-boundary.sh"
```

**Git-churn heat** — replace or supplement ticket-count heat with `git log --follow` commit frequency. Configurable via policy so teams can choose which signal better represents "hot file" for their repo.

```yaml
# policy.yaml
graph:
  file_heat_source: git_churn   # ticket_count (default) | git_churn | combined
  git_churn_window_days: 90
```

### 13.3 What Does Not Change

The graph remains a materialised cache rebuilt from git refs — never the source of truth. Custom relationships produced by extractors are recomputed on every `lore graph update` or `lore score` run. If an extractor is removed, its edges disappear on the next rebuild. This preserves the core property: clone the repo, reconstruct the complete graph from refs.

---

## 14. Glossary

**Ticket** — the primary object in Lore. A versioned intent that becomes an execution trace. Lives in git refs, not in the working tree. Serves simultaneously as a briefing document, working memory, reasoning trace, communication channel, and work generator.

**graph.index** — computed relationship index over the ticket corpus. Stores ticket nodes, file nodes, agent nodes, cluster nodes, and typed edges. Lives in `refs/tickets/graph`. Recomputed on every merge. Derivable from ticket refs — not the source of truth, a materialised cache.

**policy.yaml** — versioned configuration for merge behaviour, agent behaviour, risk scoring, consolidation triggers, and escalation routing. Lives in `refs/tickets/policy`. Changes are in git history with full audit trail.

**improvement.yaml** — user-defined improvement dimensions, weights, targets, and stopping conditions. The human-controlled definition of what "better" means in self-improving mode.

**Consolidation** — detecting that multiple tickets should be batched (spatial overlap), merged (semantic similarity), or are implying an emergent refactor (compositional pattern). Run by the AI resolver on schedule and after every merge.

**File heat** — computed score per file reflecting churn rate and regression frequency. High-heat files raise risk scores of tickets that touch them.

**Context affinity** — measure of how much recent relevant history an agent has for a file area. Used by lead agent to assign tickets to agents most likely to execute them well.

**Emergent ticket** — a ticket no human wrote. Generated by the observer (improvement mode) or AI resolver (consolidation) when a pattern implies work that has not been named yet.

**Priority override** — flag on a ticket that bypasses priority scoring and goes to the front unconditionally. For genuine emergencies, not routine prioritisation.

**Blackboard architecture** — coordination pattern where agents read from and write to shared state rather than communicating directly with each other. Lore's deliberate design choice for complete observability of all coordination decisions.

---

*Lore Specification v0.1 — Graph Extensibility added 2026-03-23*
