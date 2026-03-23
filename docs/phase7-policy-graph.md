# Phase 7: Policy Engine and Graph Index

## Overview

Two foundational systems that turn Lore from a ticket store into a coordination layer:

- **Policy engine** — versioned configuration for risk thresholds, agent behaviour, merge gates, and escalation routing. Lives in `refs/tickets/policy` so every policy change is in git history with a full audit trail.
- **Graph index** — materialised relationship map over all tickets, files, agents, and clusters. Lives in `refs/tickets/graph`. Rebuilt incrementally; `lore doctor` can reconstruct it from scratch.

Both are read-only inputs to existing commands and write-only outputs of `lore graph update`. No new external dependencies.

---

## Part 1: Policy Engine

### 1.1 Storage

Policy lives in a single git blob at `refs/tickets/policy`, serialised as YAML. This is the same pattern as tickets — `gitcmd.ReadRef` + `gitcmd.ReadBlob` to read, `gitcmd.WriteBlob` + `gitcmd.WriteRef` to write.

```
refs/tickets/policy  →  SHA of YAML blob
```

Default policy is written by `lore init` if the ref does not exist. All fields have sensible defaults so an absent policy never breaks a command.

### 1.2 Data Model

```go
// internal/policy/policy.go

package policy

type Policy struct {
    Merge         MergePolicy         `yaml:"merge"`
    Agents        AgentsPolicy        `yaml:"agents"`
    Risk          RiskPolicy          `yaml:"risk"`
    Consolidation ConsolidationPolicy `yaml:"consolidation"`
    Escalation    EscalationPolicy    `yaml:"escalation"`
    Questions     QuestionsPolicy     `yaml:"questions"`
}

type MergePolicy struct {
    AutomergeRiskCeiling    float64  `yaml:"automerge_risk_ceiling"`    // default 0.20
    RequireHumanAbove       float64  `yaml:"require_human_above"`       // default 0.60
    ValidationWindowSeconds int      `yaml:"validation_window_seconds"` // default 600
    ProtectedPaths          []string `yaml:"protected_paths"`
}

type AgentsPolicy struct {
    DefaultConfidenceThreshold float64 `yaml:"default_confidence_threshold"` // default 0.80
    MaxConcurrent              int     `yaml:"max_concurrent"`               // default 5
    MaxAttemptsPerTicket       int     `yaml:"max_attempts_per_ticket"`      // default 3
    EscalateAfterAttempts      int     `yaml:"escalate_after_attempts"`      // default 2
}

type RiskPolicy struct {
    Weights RiskWeights `yaml:"weights"`
}

type RiskWeights struct {
    DiffSize            float64 `yaml:"diff_size"`             // default 0.20
    FileHeat            float64 `yaml:"file_heat"`             // default 0.35
    ProtectedPathTouched float64 `yaml:"protected_path_touched"` // default 0.30
    PriorFailures       float64 `yaml:"prior_failures"`        // default 0.15
}

type ConsolidationPolicy struct {
    SpatialThreshold  float64 `yaml:"spatial_threshold"`  // default 0.80
    SemanticThreshold float64 `yaml:"semantic_threshold"` // default 0.75
    RunEverySeconds   int     `yaml:"run_every_seconds"`  // default 300
}

type EscalationPolicy struct {
    DefaultPath  string `yaml:"default_path"`
    BillingPath  string `yaml:"billing_path"`
    SecurityPath string `yaml:"security_path"`
}

type QuestionsPolicy struct {
    AgentAnswerWindowSeconds      int      `yaml:"agent_answer_window_seconds"`       // default 30
    AIResolverConfidenceThreshold float64  `yaml:"ai_resolver_confidence_threshold"`  // default 0.80
    HumanNotifyVia                []string `yaml:"human_notify_via"`
    BlockingQuestionUrgency       string   `yaml:"blocking_question_urgency"`         // default "high"
    NonblockingQuestionUrgency    string   `yaml:"nonblocking_question_urgency"`      // default "low"
}
```

### 1.3 Default Policy

```go
func Default() *Policy {
    return &Policy{
        Merge: MergePolicy{
            AutomergeRiskCeiling:    0.20,
            RequireHumanAbove:       0.60,
            ValidationWindowSeconds: 600,
        },
        Agents: AgentsPolicy{
            DefaultConfidenceThreshold: 0.80,
            MaxConcurrent:              5,
            MaxAttemptsPerTicket:       3,
            EscalateAfterAttempts:      2,
        },
        Risk: RiskPolicy{
            Weights: RiskWeights{
                DiffSize:             0.20,
                FileHeat:             0.35,
                ProtectedPathTouched: 0.30,
                PriorFailures:        0.15,
            },
        },
        Consolidation: ConsolidationPolicy{
            SpatialThreshold:  0.80,
            SemanticThreshold: 0.75,
            RunEverySeconds:   300,
        },
        Questions: QuestionsPolicy{
            AgentAnswerWindowSeconds:      30,
            AIResolverConfidenceThreshold: 0.80,
            HumanNotifyVia:                []string{"lore_review"},
            BlockingQuestionUrgency:       "high",
            NonblockingQuestionUrgency:    "low",
        },
    }
}
```

### 1.4 Load/Save Functions

```go
// Load reads policy from refs/tickets/policy. Returns Default() if the ref does not exist.
func Load(ctx context.Context, gitRoot string) (*Policy, error)

// Save writes policy to refs/tickets/policy.
func Save(ctx context.Context, gitRoot string, p *Policy) error
```

`Load` must never fail on a fresh repo — missing ref returns `Default()`, not an error.

### 1.5 CLI — `lore policy`

```
lore policy              # print current policy as YAML
lore policy set <key> <value>   # set a single field (dot-notation key)
lore policy edit         # open in $EDITOR (like lore edit)
lore policy reset        # overwrite with Default()
```

**`lore policy set` key format:** dot notation matching the YAML path.

```bash
lore policy set merge.automerge_risk_ceiling 0.30
lore policy set agents.max_concurrent 8
lore policy set risk.weights.file_heat 0.40
```

`set` performs a read-modify-write: load current policy, apply the change, validate the result (weights sum to 1.0, thresholds in [0, 1]), save.

**Validation rules:**
- `risk.weights.*` must sum to 1.0 (±0.01 tolerance)
- All threshold fields must be in `[0.0, 1.0]`
- `automerge_risk_ceiling` must be < `require_human_above`
- `max_attempts_per_ticket` must be >= `escalate_after_attempts`

### 1.6 Context Integration

Policy is loaded once per command invocation and stored in the cobra command context alongside config and gitRoot. Commands read it via `policyFromContext(ctx)`.

```go
// cmd/root.go — add to PersistentPreRunE
p, err := policy.Load(ctx, gitRoot)
// store in ctx
```

### 1.7 Changes to `lore prioritize`

`lore prioritize` must be updated to reflect the 0–200 range and the human-override semantics:

```
lore prioritize <ticket-id> --priority N   # N in 0–200; values > 100 pin the ticket
lore prioritize <ticket-id> --urgent       # shorthand for --priority 200
lore prioritize <ticket-id> --unpin        # resets to system-computed score (sets to 0, triggers ScoreTicket)
```

- `--priority 0–100`: sets priority, system may overwrite at next consolidation
- `--priority 101–200`: pins ticket; system will not recompute until explicitly unpinned
- Output should tell the user when they are entering the override zone:
  ```
  ticket t/abc123 priority set to 150 [pinned — system scoring disabled for this ticket]
  ```
- `--unpin`: sets priority to 0, immediately runs `ScoreTicket` to establish a fresh computed score

### 1.8 Integration Points with Existing Commands

| Command | Policy field used |
|---|---|
| `lore consolidate` | `consolidation.spatial_threshold`, `consolidation.semantic_threshold` (currently hardcoded) |
| `lore ask` | `questions.ai_resolver_confidence_threshold` (currently hardcoded at 0.82) |
| `lore pick` | `agents.max_attempts_per_ticket` for filtering over-attempted tickets |
| `lore approve` | `merge.require_human_above` for risk gate (future) |
| `lore escalate` | `escalation.*` paths for routing (future) |

For Phase 7, the minimum is: plumb policy into context, replace hardcoded thresholds in `consolidate.go` and `ask.go` with policy lookups.

---

## Part 2: Graph Index

### 2.1 What It Is

A materialised snapshot of the knowledge graph, stored as a YAML blob at `refs/tickets/graph`. It is a **derived cache** — never the source of truth. If corrupted or deleted, `lore doctor --rebuild-graph` reconstructs it from ticket refs and SQLite.

Contents:
- **Ticket nodes** — priority score, age, file touches, semantic neighbours, blocking edges
- **File nodes** — heat score, churn, regression count, open/closed tickets
- **Cluster nodes** — type, strength, member tickets, status (suggested/batched/ignored)

### 2.2 Data Model

```go
// internal/graph/graph.go

package graph

import "time"

type Index struct {
    Version int       `yaml:"version"`
    Updated time.Time `yaml:"updated"`
    Tickets  map[string]*TicketNode  `yaml:"tickets"`
    Files    map[string]*FileNode    `yaml:"files"`
    Clusters map[string]*ClusterNode `yaml:"clusters"`
}

type TicketNode struct {
    Status        string             `yaml:"status"`
    PriorityScore float64            `yaml:"priority_score"`
    AgeDays       int                `yaml:"age_days"`
    Attempts      int                `yaml:"attempts"`         // times claimed + released/failed
    Touches       []string           `yaml:"touches"`          // file paths
    WorkedBy      []string           `yaml:"worked_by"`        // agent IDs
    SimilarTo     map[string]float64 `yaml:"similar_to"`       // ticket_id → cosine similarity
    Blocks        []string           `yaml:"blocks"`           // ticket IDs this blocks
    BlockedBy     []string           `yaml:"blocked_by"`       // ticket IDs blocking this
}

type FileNode struct {
    Heat          float64  `yaml:"heat"`           // composite score [0, 1]
    Churn30d      int      `yaml:"churn_30d"`      // git commits touching this file, last 30 days
    Regressions90d int     `yaml:"regressions_90d"` // tickets with status regression touching this file
    OpenTickets   []string `yaml:"open_tickets"`
    ClosedTickets []string `yaml:"closed_tickets"`
}

type ClusterNode struct {
    Type     string   `yaml:"type"`     // "spatial" | "semantic"
    Strength float64  `yaml:"strength"`
    Members  []string `yaml:"members"`  // ticket IDs
    Status   string   `yaml:"status"`   // "suggested" | "batched" | "ignored"
}
```

### 2.3 Priority: Single Column, Two Writers

There is one `priority` field on the ticket (int, 0–200). The range is split by convention:

| Range | Writer | Meaning |
|---|---|---|
| 0–100 | System (computed) | Score from signals — agents use this for ordering |
| 101–200 | Human only (`lore prioritize`) | Override zone — system never touches these tickets |

This is enforced by convention, not cryptography. The system (`ScoreTicket`, `ScoreAll`) always outputs values in 0–100 and skips any ticket where `priority > 100`. `lore prioritize` is the only command that can write above 100, and it is human-facing by design. An agent calling `lore prioritize` with a value above 100 is misuse — it should be documented in the skill files as off-limits.

**Semantics of `priority > 100`:** the human has pinned this ticket. The system will not reorder it. It stays at its declared priority until a human explicitly lowers it back into 0–100 (at which point the system resumes computing it).

`lore pick` orders by `priority DESC` — a human-pinned ticket at 150 will always outrank a system-scored ticket at 98, which is the intended behaviour.

### 2.4 Priority Score Formula

Computed in 0–100 range. Inputs: file heat (from graph `FileNode`), blocking relationships (from SQLite), age, and attempt history. User-set priority is **not** an input — it is the output slot, replaced by the computed score.

```
raw = (max_file_heat × 40.0)        // hottest file this ticket touches, scaled to 40 pts max
    + (unblocks_count × 15.0)       // blocked tickets sharing files (proxy for dependency depth)
    + (age_days × 0.2)              // staleness pressure, uncapped
    - min(attempts × 5.0, 20.0)    // repeated failure lowers score, capped at -20

score = clamp(raw, 0, 100)
```

Where:
- `max_file_heat` = highest `FileNode.Heat` [0, 1] across all files the ticket touches
- `unblocks_count` = number of other open tickets in `blocked` status sharing at least one file with this ticket (proxy for dependency depth until explicit edges are tracked)
- `attempts` = number of times the ticket has been claimed and released/escalated (see open questions)

**Why user priority is not an input:** the computed score is the system's independent assessment. If a human disagrees with it, they write above 100 to override — they do not anchor the formula to their preference. This keeps the score honest.

**Why `unblocks_count` is a proxy:** explicit `blocks: [t/abc]` dependency edges are not yet tracked on tickets. File-sharing blocked tickets are a reasonable approximation until dependency tracking is added.

### 2.5 Score Recalculation Triggers

| Trigger | Scope | Notes |
|---|---|---|
| `lore new` | Single ticket (new one only) | Immediate initial score so `lore pick` works on day one |
| `lore consolidate` | All open tickets with `priority ≤ 100` | Full `ScoreAll` — cluster data is warm here |
| `lore graph update` | All open tickets with `priority ≤ 100` | Manual trigger for out-of-band refresh |
| `lore doctor` | All tickets | Full rebuild including scores |

`casUpdate` does **not** trigger score recomputation — it only updates graph node metadata (status, age, files). Score stays at last computed value between consolidation runs.

### 2.4 File Heat Formula

```
heat = clamp(churn_30d / 20.0, 0, 1) × 0.5
     + clamp(regressions_90d / 5.0, 0, 1) × 0.5
```

Churn is read from `git log --since=30.days.ago -- <file>`. Regressions are counted from closed tickets that touched the file and have `regression` in their checkpoints (pattern match on message).

### 2.5 Storage

```
refs/tickets/graph  →  SHA of YAML blob
```

Read with `gitcmd.ReadRef` + `gitcmd.ReadBlob`. Write with `gitcmd.WriteBlob` + `gitcmd.WriteRef`. No CAS needed — only one writer (`lore graph update`).

If the ref does not exist, commands that need graph data fall back to SQLite (status counts, file relationships) and log a soft warning.

### 2.6 Load/Save Functions

```go
// internal/graph/graph.go

// Load reads the graph index from refs/tickets/graph.
// Returns an empty Index if the ref does not exist (not an error).
func Load(ctx context.Context, gitRoot string) (*Index, error)

// Save writes the graph index to refs/tickets/graph.
func Save(ctx context.Context, gitRoot string, idx *Index) error
```

### 2.7 Update Logic

Score computation happens **only inside `lore consolidate`**, not on every `casUpdate`. `casUpdate` updates the ticket node's metadata (status, age, files) but leaves `PriorityScore` unchanged until the next consolidation run. This keeps `casUpdate` fast and keeps score computation where all the data is already warm.

```go
// internal/graph/build.go

// Build constructs a full Index from scratch using SQLite (for ticket metadata)
// and git log (for file churn). Does NOT compute priority scores — call
// ScoreAll after Build when full scoring is needed (e.g. lore doctor).
func Build(ctx context.Context, gitRoot string, s *store.Store) (*Index, error)

// ScoreAll computes priority_score for every open ticket node in idx.
// Called by lore consolidate after cluster detection completes.
func ScoreAll(ctx context.Context, gitRoot string, idx *Index, s *store.Store) error

// UpdateTicket reprocesses a single ticket node's metadata (status, age, files,
// similar_to) without recomputing priority_score. Called after every casUpdate.
func UpdateTicket(ctx context.Context, gitRoot string, idx *Index, ticketID string, s *store.Store) (*Index, error)
```

**`Build` algorithm:**
1. `s.ListTickets({})` — all tickets
2. For each ticket: compute `TicketNode` metadata (age, file touches, `similar_to` from embedding cache). Leave `PriorityScore` at 0 until `ScoreAll` runs.
3. For each unique file path: compute `FileNode` (heat via `git log`, open/closed ticket lists)
4. Copy cluster nodes from graph index if it already exists (preserve existing cluster status)
5. Stamp `Version: 2`, `Updated: now`

**`ScoreAll` algorithm (runs inside `lore consolidate`):**
1. For each open `TicketNode`:
   - Lookup `FileNode.Heat` for each file the ticket touches → take max
   - Query `s.FindRelatedByFiles(ticketID)` → count results with `status == blocked` → `unblocks_count`
   - Compute `age_days` from `TicketNode` metadata
   - Apply formula → set `PriorityScore`
2. Save updated index

**`UpdateTicket` algorithm (runs after every `casUpdate`):**
1. Load current index (or empty if missing)
2. Recompute `TicketNode` metadata for `ticketID` only (status, age, files, `similar_to`)
3. Recompute `FileNode` for each file touched by `ticketID`
4. Preserve existing `PriorityScore` — do not recompute
5. Stamp `Updated: now`, save

### 2.8 CLI — `lore graph`

```
lore graph              # print graph summary (counts, top files by heat, top clusters)
lore graph --ticket <id>  # show node for a single ticket
lore graph --file <path>  # show node for a single file
lore graph update       # incremental update (processes changed tickets since last update)
lore graph update --full  # full rebuild from scratch
```

**`lore graph` summary output:**
```
Graph index (updated 2026-03-22 14:30):

  Tickets:   47 total  (12 open, 4 in-progress, 2 blocked, 1 ready, 28 closed)
  Files:     31 tracked
  Clusters:  3 suggested

  Hottest files:
    src/auth/token.go        heat=0.71  open=2  churn=8/30d
    internal/store/store.go  heat=0.54  open=1  churn=5/30d

  Top priority (by score):
    t/abc123  score=12.4  fix-auth-timeout
    t/def456  score=9.1   add rate limiting
```

### 2.9 Integration with Existing Commands

| Command | Graph integration |
|---|---|
| `lore pick` | Read `priority_score` from graph node; fall back to `priority × 2.0` if absent |
| `lore context <id>` | Pull `similar_to` from graph node (cache hit avoids re-embedding) |
| `lore why <id>` | Show `priority_score` breakdown alongside stored `priority` |
| `lore signals` | Pull hottest files, top clusters from graph |
| `lore consolidate` | Run `ScoreAll` after cluster detection; write clusters + scores to graph |
| `lore doctor` | Run `Build` then `ScoreAll` to fully reconstruct graph |

For Phase 7, minimum integration: `lore graph` command, `lore graph update --full`, `lore doctor` calls `Build`+`ScoreAll`, `lore consolidate` calls `ScoreAll`. Deeper integration into `pick`/`context`/`why` follows naturally once the graph index exists.

### 2.10 Incremental Update Trigger

After every `casUpdate` success, call `graph.UpdateTicket` (best-effort, same as the SQLite upsert). This keeps the graph fresh without a manual `lore graph update` after every command.

```go
// in casUpdate, after s.UpsertTicket:
if idx, gerr := graph.Load(ctx, gitRoot); gerr == nil {
    if updated, gerr := graph.UpdateTicket(ctx, gitRoot, idx, id, s); gerr == nil {
        _ = graph.Save(ctx, gitRoot, updated)
    }
}
```

---

## Implementation Order

1. **`internal/policy`** — `Policy` struct, `Default()`, `Load()`, `Save()`
2. **`cmd/policy.go`** — `lore policy`, `lore policy set`, `lore policy edit`, `lore policy reset`
3. **Policy plumbing** — load in `PersistentPreRunE`, replace hardcoded thresholds in `consolidate.go` and `ask.go`
4. **`internal/graph`** — `Index` struct, `Load()`, `Save()`
5. **`internal/graph/build.go`** — `Build()`, `UpdateTicket()`
6. **`cmd/graph.go`** — `lore graph`, `lore graph update`, `lore graph update --full`
7. **`lore doctor` integration** — call `Build()` at end of rebuild
8. **`casUpdate` hook** — call `UpdateTicket` after every successful write

## Decisions

### `lore pick` scoring

`lore pick` reads `priority_score` from the graph index if it exists, falling back to `ticket.Priority × 2.0` if the graph is absent or a node has no score yet (consolidation has not run). This means:

- Fresh repo: falls back to user priority, works correctly from day one
- After first `lore consolidate`: uses computed scores
- Between consolidation runs: scores may be slightly stale but are never wrong — they reflect the last time consolidation ran, which the spec sets at every 5 minutes

`lore pick` does **not** recompute scores itself. Score computation belongs to `lore consolidate`.

### File churn and heat on shallow clones

If `git log --since=30.days.ago -- <file>` fails or returns nothing, set `churn_30d = 0` and compute heat from the regression component only:

```
churn_component      = clamp(churn_30d / 20.0, 0, 1)    # 0 if git log fails
regression_component = clamp(regressions_90d / 5.0, 0, 1)  # always available from SQLite
heat = churn_component × 0.5 + regression_component × 0.5
```

On shallow clones (`--depth=1`), git log succeeds but history is truncated — churn is underestimated rather than wrong, which is acceptable. `lore graph update` should print a warning when churn is unavailable: `"note: file churn unavailable (shallow clone or bare repo), heat scores are partial"`.

## Open Questions

- **Cluster persistence:** `lore consolidate` already writes clusters to SQLite. Should it also write to the graph index, or should `lore graph update` read from SQLite embeddings and recompute clusters? Recommendation: `consolidate` writes to graph index directly; `Build` reads from the existing SQLite cluster data as input.
- **`attempts` counter:** No ticket field tracks claim/fail cycles currently. `attempts` could be counted from checkpoint messages matching `"claimed by"` patterns, but that is fragile. Cleaner: add `Attempts int` to the `Ticket` struct, increment in `lore claim`, reset on `lore close`. Needs a migration in `lore doctor` (zero-fill from existing tickets).
