// Package graph manages the lore knowledge graph stored in refs/tickets/graph.
// The graph is a materialised cache — it can always be rebuilt from git refs.
package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/gitcmd"
	"gopkg.in/yaml.v3"
)

// Index is the top-level graph document stored in refs/tickets/graph.
type Index struct {
	Version  int                    `yaml:"version"`
	Updated  time.Time              `yaml:"updated"`
	Tickets  map[string]*TicketNode `yaml:"tickets"`
	Files    map[string]*FileNode   `yaml:"files"`
	Clusters map[string]*ClusterNode `yaml:"clusters"`
}

// TicketNode holds graph-level metadata for a single ticket.
type TicketNode struct {
	// Score is the system-computed priority score (0-100).
	Score int `yaml:"score"`
	// Blocks is a list of ticket IDs that this ticket directly unblocks.
	Blocks []string `yaml:"blocks,omitempty"`
	// BlockedBy is a list of ticket IDs that this ticket depends on.
	BlockedBy []string `yaml:"blocked_by,omitempty"`
	// Files is the list of file paths this ticket touches.
	Files []string `yaml:"files,omitempty"`
	// ClusterID is the cluster this ticket belongs to (if any).
	ClusterID string `yaml:"cluster_id,omitempty"`
}

// FileNode tracks change frequency for a file path.
type FileNode struct {
	// Heat is a normalised value 0-1 representing how frequently this file changes.
	Heat float64 `yaml:"heat"`
	// Tickets is a list of ticket IDs that touch this file.
	Tickets []string `yaml:"tickets,omitempty"`
}

// ClusterNode represents a group of related tickets found by consolidation.
type ClusterNode struct {
	// TicketIDs are the members of this cluster.
	TicketIDs []string `yaml:"ticket_ids"`
	// Strength is the average pairwise cosine similarity within the cluster.
	Strength float64 `yaml:"strength"`
}

// Empty returns an empty Index ready for use.
func Empty() *Index {
	return &Index{
		Version:  1,
		Tickets:  make(map[string]*TicketNode),
		Files:    make(map[string]*FileNode),
		Clusters: make(map[string]*ClusterNode),
	}
}

// Load reads the graph blob from refs/tickets/graph.
// Falls back to Empty() if the ref does not exist.
func Load(ctx context.Context, gitRoot string) (*Index, error) {
	sha, err := gitcmd.ReadRef(ctx, gitRoot, "refs/tickets/graph")
	if errors.Is(err, gitcmd.ErrRefNotFound) {
		return Empty(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("graph: read ref: %w", err)
	}

	data, err := gitcmd.ReadBlob(ctx, gitRoot, sha)
	if err != nil {
		return nil, fmt.Errorf("graph: read blob: %w", err)
	}

	idx := Empty()
	if err := yaml.Unmarshal(data, idx); err != nil {
		return nil, fmt.Errorf("graph: parse: %w", err)
	}
	return idx, nil
}

// Save marshals idx to YAML and writes it to refs/tickets/graph.
func Save(ctx context.Context, gitRoot string, idx *Index) error {
	idx.Updated = time.Now().UTC()
	data, err := yaml.Marshal(idx)
	if err != nil {
		return fmt.Errorf("graph: marshal: %w", err)
	}
	sha, err := gitcmd.WriteBlob(ctx, gitRoot, data)
	if err != nil {
		return fmt.Errorf("graph: write blob: %w", err)
	}
	if err := gitcmd.WriteRef(ctx, gitRoot, "refs/tickets/graph", sha); err != nil {
		return fmt.Errorf("graph: write ref: %w", err)
	}
	return nil
}
