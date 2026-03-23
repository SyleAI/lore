package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/loreteam/lore/internal/ai"
	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/graph"
	"github.com/loreteam/lore/internal/policy"
	"github.com/loreteam/lore/internal/store"
	"github.com/loreteam/lore/internal/templates"
	"github.com/spf13/cobra"
)

const (
	strategyEmbeddings  = "embeddings"
	strategyPairwise    = "pairwise"
	strategySinglePrompt = "single-prompt"
)

// ticketRef is a lightweight ticket summary passed to prompt templates.
type ticketRef struct {
	ID    string
	Title string
}

var consolidateThreshold float64

var consolidateCmd = &cobra.Command{
	Use:   "consolidate",
	Short: "Find duplicate or related open tickets",
	Long: `consolidate groups related open tickets and suggests actions (merge/batch/ignore).

The strategy is controlled by policy.consolidation.strategy:
  single-prompt  (default) send all ticket titles to Claude; no embeddings required
  embeddings     cluster by cosine similarity (requires embeddings provider)
  pairwise       compare every pair with Claude (O(N²); slow for large backlogs)`,
	RunE: runConsolidate,
}

func init() {
	consolidateCmd.Flags().Float64Var(&consolidateThreshold, "threshold", 0, "similarity threshold override (embeddings strategy only)")
	rootCmd.AddCommand(consolidateCmd)
}

func runConsolidate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	pol, err := policy.Load(ctx, gitRoot)
	if err != nil {
		return fmt.Errorf("lore consolidate: load policy: %w", err)
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore consolidate: %w", err)
	}
	defer s.Close()

	rows, err := s.ListTickets(store.ListFilter{Status: "open"})
	if err != nil {
		return fmt.Errorf("lore consolidate: %w", err)
	}
	if len(rows) == 0 {
		fmt.Println("no open tickets")
		return nil
	}

	completer := newCompleter(ctx)
	strategy := pol.Consolidation.Strategy
	if strategy == "" {
		strategy = strategySinglePrompt
	}

	var clusters []ai.Cluster
	switch strategy {
	case strategyEmbeddings:
		threshold := pol.Consolidation.SpatialThreshold
		if consolidateThreshold != 0 {
			threshold = consolidateThreshold
		}
		embedder := newEmbedder(ctx)
		ids, vecs, err := buildEmbeddings(ctx, gitRoot, s, embedder, rows)
		if err != nil {
			return fmt.Errorf("lore consolidate: %w", err)
		}
		clusters = ai.FindClusters(ids, vecs, threshold)
	case strategyPairwise:
		clusters, err = clusterByPairwise(ctx, rows, completer)
		if err != nil {
			return fmt.Errorf("lore consolidate: %w", err)
		}
	default: // strategySinglePrompt
		clusters, err = clusterBySinglePrompt(ctx, rows, completer)
		if err != nil {
			return fmt.Errorf("lore consolidate: %w", err)
		}
	}

	if len(clusters) == 0 {
		fmt.Println("no related ticket groups found")
		return nil
	}

	suggestions, err := classifyClusters(ctx, clusters, rows, completer)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: classification unavailable: %v\n", err)
	}

	printConsolidation(clusters, suggestions, flagJSON)

	// Persist non-ignored clusters to the graph index.
	if err := persistClusters(ctx, gitRoot, clusters, suggestions); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not persist clusters to graph: %v\n", err)
	}

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventConsolidationSuggested, map[string]any{
		"cluster_count": len(clusters),
		"strategy":      strategy,
	}))
	return nil
}

// persistClusters writes non-ignored clusters to the graph index.
func persistClusters(ctx context.Context, gitRoot string, clusters []ai.Cluster, suggestions []consolidationSuggestion) error {
	idx, err := graph.Load(ctx, gitRoot)
	if err != nil {
		return err
	}

	// Clear stale clusters before writing fresh ones.
	idx.Clusters = make(map[string]*graph.ClusterNode)

	for i, c := range clusters {
		action := ""
		if i < len(suggestions) {
			action = suggestions[i].Action
		}
		if action == "ignore" {
			continue
		}
		id := clusterID(c.IDs)
		idx.Clusters[id] = &graph.ClusterNode{
			TicketIDs: c.IDs,
			Strength:  c.Strength,
		}
		// Mark each ticket with its cluster membership.
		for _, tid := range c.IDs {
			if node, ok := idx.Tickets[tid]; ok {
				node.ClusterID = id
			}
		}
	}

	return graph.Save(ctx, gitRoot, idx)
}

// clusterID returns a stable 8-char ID for a group of ticket IDs.
func clusterID(ids []string) string {
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return fmt.Sprintf("%x", h[:4])
}

// clusterBySinglePrompt sends all ticket titles to Claude in one prompt and
// parses the returned groups. No embeddings required.
func clusterBySinglePrompt(ctx context.Context, rows []*store.Row, completer ai.Completer) ([]ai.Cluster, error) {
	tickets := rowsToRefs(rows)
	prompt, err := templates.RenderPrompt("consolidate-find-groups", loreDirFromContext(ctx), map[string]any{
		"Tickets": tickets,
	})
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}
	resp, err := completer.Complete(prompt)
	if err != nil {
		return nil, fmt.Errorf("complete: %w", err)
	}
	groups, err := extractJSONArray[[][]string](resp)
	if err != nil || groups == nil {
		return nil, err
	}
	clusters := make([]ai.Cluster, 0, len(groups))
	for _, ids := range groups {
		if len(ids) >= 2 {
			clusters = append(clusters, ai.Cluster{IDs: ids, Strength: 1.0})
		}
	}
	return clusters, nil
}

// clusterByPairwise compares every pair of tickets using the LLM (O(N²)).
func clusterByPairwise(ctx context.Context, rows []*store.Row, completer ai.Completer) ([]ai.Cluster, error) {
	loreDir := loreDirFromContext(ctx)
	related := make([][]int, len(rows))

	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			prompt, err := templates.RenderPrompt("consolidate-find-groups", loreDir, map[string]any{
				"Tickets": []ticketRef{
					{ID: rows[i].ID, Title: rows[i].Title},
					{ID: rows[j].ID, Title: rows[j].Title},
				},
			})
			if err != nil {
				return nil, err
			}
			resp, err := completer.Complete(prompt)
			if err != nil {
				return nil, err
			}
			groups, _ := extractJSONArray[[][]string](resp)
			if len(groups) > 0 {
				related[i] = append(related[i], j)
			}
		}
	}

	// Union-find to merge transitive relationships.
	parent := make([]int, len(rows))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	for i, rels := range related {
		for _, j := range rels {
			pa, pb := find(i), find(j)
			if pa != pb {
				parent[pa] = pb
			}
		}
	}

	// Count group sizes, then build clusters for groups of 2+.
	groupSize := make(map[int]int, len(rows))
	for i := range rows {
		groupSize[find(i)]++
	}
	groups := make(map[int][]string)
	for i, r := range rows {
		root := find(i)
		if groupSize[root] >= 2 {
			groups[root] = append(groups[root], r.ID)
		}
	}

	clusters := make([]ai.Cluster, 0, len(groups))
	for _, ids := range groups {
		clusters = append(clusters, ai.Cluster{IDs: ids, Strength: 1.0})
	}
	return clusters, nil
}

// buildEmbeddings returns parallel slices of ticket IDs and their embeddings.
// Stale entries (SHA changed) are re-embedded in batch; hits come from SQLite cache.
func buildEmbeddings(
	ctx context.Context,
	gitRoot string,
	s *store.Store,
	embedder ai.Embedder,
	rows []*store.Row,
) ([]string, []ai.Embedding, error) {
	ids := make([]string, len(rows))
	vecs := make([]ai.Embedding, len(rows))

	var staleBatch []int
	for i, r := range rows {
		blob, err := s.GetEmbedding(r.ID, r.SHA)
		if err != nil {
			return nil, nil, fmt.Errorf("get embedding %s: %w", r.ID, err)
		}
		ids[i] = r.ID
		if blob != nil {
			vec, err := ai.DecodeVector(blob)
			if err != nil {
				return nil, nil, fmt.Errorf("decode embedding %s: %w", r.ID, err)
			}
			vecs[i] = vec
		} else {
			staleBatch = append(staleBatch, i)
		}
	}

	if len(staleBatch) == 0 {
		return ids, vecs, nil
	}

	texts := make([]string, len(staleBatch))
	for j, i := range staleBatch {
		t, err := loadTicket(ctx, gitRoot, rows[i].ID)
		if err != nil {
			return nil, nil, fmt.Errorf("load ticket %s: %w", rows[i].ID, err)
		}
		texts[j] = t.Title + "\n" + t.Description
	}

	embedded, err := embedder.Embed(texts)
	if err != nil {
		return nil, nil, fmt.Errorf("embed tickets: %w", err)
	}

	for j, i := range staleBatch {
		vecs[i] = embedded[j]
		_ = s.SetEmbedding(rows[i].ID, rows[i].SHA, ai.EncodeVector(embedded[j]))
	}

	return ids, vecs, nil
}

// consolidationSuggestion is the structured response from Claude per cluster.
type consolidationSuggestion struct {
	Action string `json:"action"` // "merge", "batch", or "ignore"
	Reason string `json:"reason"`
}

// classifyClusters asks the Completer to classify each cluster.
func classifyClusters(ctx context.Context, clusters []ai.Cluster, rows []*store.Row, completer ai.Completer) ([]consolidationSuggestion, error) {
	loreDir := loreDirFromContext(ctx)
	titleOf := make(map[string]string, len(rows))
	for _, r := range rows {
		titleOf[r.ID] = r.Title
	}

	suggestions := make([]consolidationSuggestion, len(clusters))
	for i, c := range clusters {
		tickets := make([]ticketRef, len(c.IDs))
		for j, id := range c.IDs {
			tickets[j] = ticketRef{ID: id, Title: titleOf[id]}
		}
		prompt, err := templates.RenderPrompt("consolidate-classify", loreDir, map[string]any{
			"Strength": c.Strength,
			"Tickets":  tickets,
		})
		if err != nil {
			return suggestions, fmt.Errorf("render prompt: %w", err)
		}
		resp, err := completer.Complete(prompt)
		if err != nil {
			return suggestions, err
		}
		obj, err := extractJSONObject(resp)
		if err != nil || obj == nil {
			suggestions[i] = consolidationSuggestion{Action: "unknown", Reason: resp}
			continue
		}
		if err := json.Unmarshal(obj, &suggestions[i]); err != nil {
			suggestions[i] = consolidationSuggestion{Action: "unknown", Reason: resp}
		}
	}
	return suggestions, nil
}

// extractJSONObject returns the first {...} substring from s, or nil if none found.
func extractJSONObject(s string) ([]byte, error) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, nil
	}
	return []byte(s[start : end+1]), nil
}

// extractJSONArray parses the first [...] substring from s into T.
func extractJSONArray[T any](s string) (T, error) {
	var zero T
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return zero, nil
	}
	var result T
	if err := json.Unmarshal([]byte(s[start:end+1]), &result); err != nil {
		return zero, err
	}
	return result, nil
}

// rowsToRefs converts store rows to ticketRef slices for prompt templates.
func rowsToRefs(rows []*store.Row) []ticketRef {
	refs := make([]ticketRef, len(rows))
	for i, r := range rows {
		refs[i] = ticketRef{ID: r.ID, Title: r.Title}
	}
	return refs
}

func printConsolidation(clusters []ai.Cluster, suggestions []consolidationSuggestion, asJSON bool) {
	if asJSON {
		type clusterOut struct {
			IDs      []string `json:"ids"`
			Strength float64  `json:"strength"`
			Action   string   `json:"action,omitempty"`
			Reason   string   `json:"reason,omitempty"`
		}
		out := make([]clusterOut, len(clusters))
		for i, c := range clusters {
			out[i] = clusterOut{IDs: c.IDs, Strength: c.Strength}
			if i < len(suggestions) {
				out[i].Action = suggestions[i].Action
				out[i].Reason = suggestions[i].Reason
			}
		}
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}

	for i, c := range clusters {
		fmt.Printf("Group %d:\n", i+1)
		for _, id := range c.IDs {
			fmt.Printf("  %s\n", id)
		}
		if i < len(suggestions) && suggestions[i].Action != "" {
			fmt.Printf("  => %s: %s\n", suggestions[i].Action, suggestions[i].Reason)
		}
		fmt.Println()
	}
}
