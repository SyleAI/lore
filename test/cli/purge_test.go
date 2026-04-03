package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPurge_TicketsOnly(t *testing.T) {
	repo := newRepo(t)
	newTicket(t, repo, "ticket to purge")

	// Install an orchestrator — it should survive.
	src := makeOrchestrator(t, "survivor", true)
	run(t, repo, 0, "install", src)

	run(t, repo, 0, "purge", "--tickets", "--force")

	// Tickets gone.
	out := run(t, repo, 0, "list")
	assert.Contains(t, out, "no tickets found")

	// Orchestrator intact.
	orchDir := filepath.Join(repo, ".lore", "orchestrators", "survivor")
	_, err := os.Stat(orchDir)
	assert.NoError(t, err)
}

func TestPurge_OrchestratorOnly(t *testing.T) {
	repo := newRepo(t)
	id := newTicket(t, repo, "ticket that survives")

	src := makeOrchestrator(t, "to-purge", true)
	run(t, repo, 0, "install", src)

	run(t, repo, 0, "purge", "--orchestrators", "--force")

	// Orchestrator gone.
	out := run(t, repo, 0, "orchestrators")
	assert.Contains(t, out, "No orchestrators installed")

	// Ticket intact.
	list := run(t, repo, 0, "list")
	assert.Contains(t, list, id)
}

func TestPurge_All(t *testing.T) {
	repo := newRepo(t)
	newTicket(t, repo, "ticket")
	src := makeOrchestrator(t, "orch", true)
	run(t, repo, 0, "install", src)

	run(t, repo, 0, "purge", "--force")

	assert.Contains(t, run(t, repo, 0, "list"), "no tickets found")
	assert.Contains(t, run(t, repo, 0, "orchestrators"), "No orchestrators installed")
}

func TestPurge_NoTickets(t *testing.T) {
	repo := newRepo(t)
	// Purging with no tickets should not error.
	out := run(t, repo, 0, "purge", "--tickets", "--force")
	assert.Contains(t, out, "purged 0")
}

func TestPurge_NoOrchestrators(t *testing.T) {
	repo := newRepo(t)
	out := run(t, repo, 0, "purge", "--orchestrators", "--force")
	assert.Contains(t, out, "no orchestrators installed")
}
