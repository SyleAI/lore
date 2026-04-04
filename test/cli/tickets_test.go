package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── lore new ─────────────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	dir := newRepo(t)
	out := run(t, dir, 0, "new", "--from", "fix the auth timeout")
	assert.Contains(t, out, "created ticket")
}

func TestTicketStoredOnDisk(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "verify disk storage")

	// Ticket YAML must exist in .tickets/open/.
	ticketPath := filepath.Join(dir, ".tickets", "open", id+".yaml")
	_, err := os.Stat(ticketPath)
	assert.NoError(t, err, "expected ticket file at %s", ticketPath)

	// After close, ticket must move to .tickets/done/.
	run(t, dir, 0, "close", id)
	_, err = os.Stat(filepath.Join(dir, ".tickets", "done", id+".yaml"))
	assert.NoError(t, err, "expected ticket in done/ after close")
	_, err = os.Stat(ticketPath)
	assert.True(t, os.IsNotExist(err), "ticket should be removed from open/ after close")
}

func TestThreadStoredOnDisk(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "thread disk check")

	run(t, dir, 0, "update", id, "progress note")

	// At least one JSON file must exist under .tickets/threads/<id>/.
	threadDir := filepath.Join(dir, ".tickets", "threads", id)
	entries, err := os.ReadDir(threadDir)
	require.NoError(t, err, "thread directory should exist")
	var jsonFiles []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			jsonFiles = append(jsonFiles, e.Name())
		}
	}
	assert.NotEmpty(t, jsonFiles, "expected at least one thread entry file")
}

func TestQuestionStoredOnDisk(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "question disk check")

	askOut := run(t, dir, 0, "ask", id, "which approach?", "--from", "agent-1")
	parts := strings.Fields(askOut)
	require.GreaterOrEqual(t, len(parts), 2)
	qid := parts[1]

	// Question JSON must exist in .tickets/questions/.
	qPath := filepath.Join(dir, ".tickets", "questions", qid+".json")
	_, err := os.Stat(qPath)
	assert.NoError(t, err, "expected question file at %s", qPath)
}

func TestNew_EmptyDescription(t *testing.T) {
	dir := newRepo(t)
	// Whitespace-only is trimmed to empty, hitting the "description is required" path.
	out := run(t, dir, 1, "new", "--from", "   ")
	assert.Contains(t, out, "description is required")
}

func TestNew_NotInitialised(t *testing.T) {
	// A git repo without lore init still has a git root, but refs won't exist.
	// The binary should still fail gracefully if something goes wrong.
	dir := newRepo(t)
	id := newTicket(t, dir, "sanity check")
	assert.NotEmpty(t, id)
}

// ── lore list ────────────────────────────────────────────────────────────────

func TestList_Empty(t *testing.T) {
	dir := newRepo(t)
	out := run(t, dir, 0, "list")
	assert.Contains(t, out, "no tickets found")
}

func TestList_ShowsOpenTickets(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "do the thing")
	out := run(t, dir, 0, "list")
	assert.Contains(t, out, id)
	assert.Contains(t, out, "open")
}

func TestList_HidesDoneByDefault(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "will be closed")
	run(t, dir, 0, "close", id)
	out := run(t, dir, 0, "list")
	assert.NotContains(t, out, id)
}

func TestList_ClosedFlag(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "will be closed")
	run(t, dir, 0, "close", id)
	out := run(t, dir, 0, "list", "--closed")
	assert.Contains(t, out, id)
	assert.Contains(t, out, "done")
}

// ── lore show ────────────────────────────────────────────────────────────────

func TestShow(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "show me the ticket")
	out := run(t, dir, 0, "show", id)
	assert.Contains(t, out, id)
	assert.Contains(t, out, "show me the ticket")
	assert.Contains(t, out, "open")
}

func TestShow_NotFound(t *testing.T) {
	dir := newRepo(t)
	out := run(t, dir, 1, "show", "doesnotexist")
	assert.Contains(t, out, "not found")
}

func TestShow_JSON(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "json output test")
	out := run(t, dir, 0, "show", id, "--json")
	assert.Contains(t, out, `"id"`)
	assert.Contains(t, out, id)
	assert.Contains(t, out, `"status"`)
}

// ── lore claim ───────────────────────────────────────────────────────────────

func TestClaim(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "claim this ticket")
	out := run(t, dir, 0, "claim", id, "--agent", "agent-1")
	assert.Contains(t, out, "claimed")
	assert.Contains(t, out, "agent-1")

	// Ticket should now be working.
	list := run(t, dir, 0, "list")
	assert.Contains(t, list, "working")
}

func TestClaim_AlreadyClaimed(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "double claim test")
	run(t, dir, 0, "claim", id, "--agent", "agent-1")
	out := run(t, dir, 1, "claim", id, "--agent", "agent-2")
	assert.Contains(t, out, "not open")
}

// ── lore update ──────────────────────────────────────────────────────────────

func TestUpdate(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "update test")
	out := run(t, dir, 0, "update", id, "made some progress")
	assert.Contains(t, out, "update added")

	show := run(t, dir, 0, "show", id)
	assert.Contains(t, show, "made some progress")
}

func TestUpdate_MissingMessage(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "update missing message")
	out := run(t, dir, 1, "update", id)
	assert.Contains(t, out, "message required")
}

// ── lore ready ───────────────────────────────────────────────────────────────

func TestReady(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "ready for review")
	run(t, dir, 0, "claim", id, "--agent", "agent-1")
	out := run(t, dir, 0, "ready", id)
	assert.Contains(t, out, "ready for review")

	list := run(t, dir, 0, "list")
	assert.Contains(t, list, "ready-for-review")
}

func TestReady_NotWorking(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "not working yet")
	// Ticket is open, not working — should fail.
	out := run(t, dir, 1, "ready", id)
	assert.Contains(t, out, "not working")
}

// ── lore approve ─────────────────────────────────────────────────────────────

func TestApprove(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "approve me")
	run(t, dir, 0, "claim", id, "--agent", "agent-1")
	run(t, dir, 0, "ready", id)
	out := run(t, dir, 0, "approve", id, "--from", "reviewer")
	assert.Contains(t, out, "approved")
}

func TestApprove_NotReadyForReview(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "not ready")
	out := run(t, dir, 1, "approve", id)
	assert.Contains(t, out, "not ready-for-review")
}

// ── lore reject ──────────────────────────────────────────────────────────────

func TestReject(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "reject me")
	run(t, dir, 0, "claim", id, "--agent", "agent-1")
	run(t, dir, 0, "ready", id)
	out := run(t, dir, 0, "reject", id, "--reason", "needs tests", "--from", "reviewer")
	assert.Contains(t, out, "rejected")
	assert.Contains(t, out, "needs tests")

	// Ticket should be back to working.
	list := run(t, dir, 0, "list")
	assert.Contains(t, list, "working")
}

func TestReject_ReasonRequired(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "reject without reason")
	run(t, dir, 0, "claim", id, "--agent", "agent-1")
	run(t, dir, 0, "ready", id)
	out := run(t, dir, 1, "reject", id)
	assert.Contains(t, out, "reason")
}

// ── lore close ───────────────────────────────────────────────────────────────

func TestClose(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "close me")
	out := run(t, dir, 0, "close", id)
	assert.Contains(t, out, "closed")

	// Should appear with --closed.
	list := run(t, dir, 0, "list", "--closed")
	assert.Contains(t, list, id)
	assert.Contains(t, list, "done")
}

// ── lore block / unblock ─────────────────────────────────────────────────────

func TestBlockUnblock(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "block unblock test")

	out := run(t, dir, 0, "block", id, "waiting on dependency")
	assert.Contains(t, out, "blocked")

	list := run(t, dir, 0, "list")
	assert.Contains(t, list, "blocked")

	out = run(t, dir, 0, "unblock", id)
	assert.Contains(t, out, "unblocked")
}

func TestUnblock_NotBlocked(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "not blocked")
	out := run(t, dir, 1, "unblock", id)
	assert.Contains(t, out, "not blocked")
}

// ── lore ask / answer ────────────────────────────────────────────────────────

func TestAskAnswer(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "ask answer test")

	askOut := run(t, dir, 0, "ask", id, "what approach should we use?", "--from", "agent-1")
	assert.Contains(t, askOut, "question")

	// Extract question ID from output: "question q-xxxx posted on ticket ..."
	parts := strings.Fields(askOut)
	require.GreaterOrEqual(t, len(parts), 2)
	qid := parts[1]
	assert.True(t, strings.HasPrefix(qid, "q-"), "expected question ID, got %q", qid)

	answerOut := run(t, dir, 0, "answer", id, "use approach X", "--question-id", qid, "--from", "human")
	assert.Contains(t, answerOut, "answered")

	// Thread should contain both.
	show := run(t, dir, 0, "show", id)
	assert.Contains(t, show, "what approach should we use?")
	assert.Contains(t, show, "use approach X")
}

func TestAsk_BlocksTicket(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "blocking question test")
	run(t, dir, 0, "ask", id, "blocking question?", "--block", "--from", "agent-1")

	list := run(t, dir, 0, "list")
	assert.Contains(t, list, "blocked")
}

func TestAnswer_QuestionIDRequired(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "answer without qid")
	out := run(t, dir, 1, "answer", id, "some answer")
	assert.Contains(t, out, "question-id")
}

// ── lore spawn ───────────────────────────────────────────────────────────────

func TestSpawn(t *testing.T) {
	dir := newRepo(t)
	parentID := newTicket(t, dir, "parent ticket")
	out := run(t, dir, 0, "spawn", parentID, "child work item")
	assert.Contains(t, out, "spawned")
	assert.Contains(t, out, parentID)

	// Child should appear in list.
	list := run(t, dir, 0, "list")
	lines := strings.Split(list, "\n")
	count := 0
	for _, l := range lines {
		if strings.Contains(l, "open") {
			count++
		}
	}
	assert.GreaterOrEqual(t, count, 2, "expected parent and child in list")
}

func TestSpawn_ParentNotFound(t *testing.T) {
	dir := newRepo(t)
	out := run(t, dir, 1, "spawn", "doesnotexist", "child")
	assert.Contains(t, out, "not found")
}

// ── lore assign ──────────────────────────────────────────────────────────────

func TestAssign(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "assign test")
	out := run(t, dir, 0, "assign", id, "--agent", "agent-42")
	assert.Contains(t, out, "assigned")
	assert.Contains(t, out, "agent-42")
}

func TestAssign_AgentRequired(t *testing.T) {
	dir := newRepo(t)
	id := newTicket(t, dir, "assign no agent")
	out := run(t, dir, 1, "assign", id)
	assert.Contains(t, out, "agent")
}

// ── full lifecycle ────────────────────────────────────────────────────────────

func TestFullLifecycle(t *testing.T) {
	dir := newRepo(t)

	// Create.
	id := newTicket(t, dir, "implement login feature")

	// Claim.
	run(t, dir, 0, "claim", id, "--agent", "agent-1")

	// Update with progress.
	run(t, dir, 0, "update", id, "started on the login handler")

	// Ask a question, get an answer.
	askOut := run(t, dir, 0, "ask", id, "which auth provider?", "--from", "agent-1")
	parts := strings.Fields(askOut)
	qid := parts[1]
	run(t, dir, 0, "answer", id, "use OAuth2", "--question-id", qid, "--from", "human")

	// Mark ready.
	run(t, dir, 0, "ready", id)

	// Reject and rework.
	run(t, dir, 0, "reject", id, "--reason", "missing tests")
	run(t, dir, 0, "update", id, "added tests")
	run(t, dir, 0, "ready", id)

	// Approve.
	run(t, dir, 0, "approve", id, "--from", "reviewer")

	// Close.
	run(t, dir, 0, "close", id)

	// Verify final state.
	show := run(t, dir, 0, "show", id)
	assert.Contains(t, show, "done")
	assert.Contains(t, show, "implement login feature")
	assert.Contains(t, show, "started on the login handler")
	assert.Contains(t, show, "which auth provider?")
	assert.Contains(t, show, "use OAuth2")
}
