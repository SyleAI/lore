// Package policy manages the lore policy configuration stored in refs/tickets/policy.
package policy

import (
	"context"
	"errors"
	"fmt"

	"github.com/loreteam/lore/internal/gitcmd"
	"gopkg.in/yaml.v3"
)

// Policy is the top-level policy document stored in refs/tickets/policy.
type Policy struct {
	Merge         MergePolicy         `yaml:"merge"`
	Agents        AgentsPolicy        `yaml:"agents"`
	Risk          RiskPolicy          `yaml:"risk"`
	Consolidation ConsolidationPolicy `yaml:"consolidation"`
	Escalation    EscalationPolicy    `yaml:"escalation"`
	Questions     QuestionsPolicy     `yaml:"questions"`
	Scoring       ScoringPolicy       `yaml:"scoring"`
}

// MergePolicy governs when agents may auto-merge.
type MergePolicy struct {
	AutomergeRiskCeiling    float64  `yaml:"automerge_risk_ceiling"`
	RequireHumanAbove       float64  `yaml:"require_human_above"`
	ValidationWindowSeconds int      `yaml:"validation_window_seconds"`
	ProtectedPaths          []string `yaml:"protected_paths"`
}

// AgentsPolicy governs agent behaviour limits.
type AgentsPolicy struct {
	DefaultConfidenceThreshold float64 `yaml:"default_confidence_threshold"`
	MaxConcurrent              int     `yaml:"max_concurrent"`
	MaxAttemptsPerTicket       int     `yaml:"max_attempts_per_ticket"`
	EscalateAfterAttempts      int     `yaml:"escalate_after_attempts"`
}

// RiskPolicy configures the risk scoring formula weights.
type RiskPolicy struct {
	Weights RiskWeights `yaml:"weights"`
}

// RiskWeights are the four components of the risk score (must sum to 1.0).
type RiskWeights struct {
	DiffSize             float64 `yaml:"diff_size"`
	FileHeat             float64 `yaml:"file_heat"`
	ProtectedPathTouched float64 `yaml:"protected_path_touched"`
	PriorFailures        float64 `yaml:"prior_failures"`
}

// ConsolidationPolicy controls how lore consolidate works.
type ConsolidationPolicy struct {
	// Strategy is one of: "single-prompt" (default), "pairwise", "embeddings".
	Strategy          string  `yaml:"strategy"`
	SpatialThreshold  float64 `yaml:"spatial_threshold"`
	SemanticThreshold float64 `yaml:"semantic_threshold"`
	RunEverySeconds   int     `yaml:"run_every_seconds"`
}

// EscalationPolicy maps escalation categories to notification targets.
type EscalationPolicy struct {
	DefaultPath  string `yaml:"default_path"`
	BillingPath  string `yaml:"billing_path"`
	SecurityPath string `yaml:"security_path"`
}

// QuestionsPolicy configures async question handling.
type QuestionsPolicy struct {
	AgentAnswerWindowSeconds      int      `yaml:"agent_answer_window_seconds"`
	AIResolverConfidenceThreshold float64  `yaml:"ai_resolver_confidence_threshold"`
	HumanNotifyVia                []string `yaml:"human_notify_via"`
	BlockingQuestionUrgency       string   `yaml:"blocking_question_urgency"`
	NonblockingQuestionUrgency    string   `yaml:"nonblocking_question_urgency"`
}

// ScoringPolicy configures how system priority scores (0-100) are computed.
type ScoringPolicy struct {
	// FileHeatWeight is added per unit of max file heat (default 40).
	FileHeatWeight float64 `yaml:"file_heat_weight"`
	// UnblocksWeight is added per ticket this ticket unblocks (default 15).
	UnblocksWeight float64 `yaml:"unblocks_weight"`
	// AgeDayWeight is added per day of age (default 0.2).
	AgeDayWeight float64 `yaml:"age_day_weight"`
	// AttemptsPenalty is subtracted per attempt (default 5).
	AttemptsPenalty float64 `yaml:"attempts_penalty"`
	// MaxAttemptsPenalty caps the total attempts deduction (default 20).
	MaxAttemptsPenalty float64 `yaml:"max_attempts_penalty"`
}

// Default returns a Policy with all sensible defaults pre-filled.
func Default() *Policy {
	return &Policy{
		Merge: MergePolicy{
			AutomergeRiskCeiling:    0.20,
			RequireHumanAbove:       0.60,
			ValidationWindowSeconds: 600,
			ProtectedPaths:          []string{},
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
			Strategy:          "single-prompt",
			SpatialThreshold:  0.80,
			SemanticThreshold: 0.75,
			RunEverySeconds:   300,
		},
		Escalation: EscalationPolicy{
			DefaultPath:  "@platform-team",
			BillingPath:  "@billing-lead",
			SecurityPath: "@security",
		},
		Questions: QuestionsPolicy{
			AgentAnswerWindowSeconds:      30,
			AIResolverConfidenceThreshold: 0.80,
			HumanNotifyVia:                []string{"lore_review"},
			BlockingQuestionUrgency:       "high",
			NonblockingQuestionUrgency:    "low",
		},
		Scoring: ScoringPolicy{
			FileHeatWeight:     40,
			UnblocksWeight:     15,
			AgeDayWeight:       0.2,
			AttemptsPenalty:    5,
			MaxAttemptsPenalty: 20,
		},
	}
}

// Load reads the policy blob from refs/tickets/policy.
// Falls back to Default() if the ref does not exist.
func Load(ctx context.Context, gitRoot string) (*Policy, error) {
	p := Default()

	sha, err := gitcmd.ReadRef(ctx, gitRoot, "refs/tickets/policy")
	if errors.Is(err, gitcmd.ErrRefNotFound) {
		return p, nil
	}
	if err != nil {
		return nil, fmt.Errorf("policy: read ref: %w", err)
	}

	data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
	if err != nil {
		return nil, fmt.Errorf("policy: read blob: %w", err)
	}
	if err := yaml.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("policy: parse: %w", err)
	}
	return p, nil
}

// Save marshals p to YAML and writes it to refs/tickets/policy.
func Save(ctx context.Context, gitRoot string, p *Policy) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("policy: marshal: %w", err)
	}
	sha, err := gitcmd.WriteBlob(ctx, gitRoot, data)
	if err != nil {
		return fmt.Errorf("policy: write blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, "refs/tickets/policy", sha); err != nil {
		return fmt.Errorf("policy: write ref: %w", err)
	}
	return nil
}
