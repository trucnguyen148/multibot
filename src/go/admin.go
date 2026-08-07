package main

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// The researcher-facing API behind /admin. Everything here is destructive or
// discloses participant data, so each handler authenticates on its own rather
// than trusting a wrapper or the fact that the UI asked for a password.
//
// The page itself is served by `serve -s build`, which rewrites unknown paths
// to index.html, so /admin is publicly reachable and always will be. That is
// fine precisely because it holds nothing. Every byte it shows arrives from an
// endpoint below, and every one of those checks the secret first.

// adminSecret is deliberately EXPORT_SECRET rather than a second variable.
// Anyone holding either could read every transcript, so a separate secret would
// imply a permission split that does not exist.
func adminSecret() string { return os.Getenv("EXPORT_SECRET") }

// secretMatches compares in constant time and treats an unset expected secret
// as "deny everything". A deployment that forgot the variable must not quietly
// become an open export, so this fails closed rather than usefully.
func secretMatches(expected, presented string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}

// presentedSecret prefers the header. The query parameter is accepted so the
// endpoints stay usable from curl, matching /api/export, though the browser
// never uses it. A secret in a URL lands in shell history, proxy logs and
// Railway's request logs.
func presentedSecret(r *http.Request) string {
	if header := r.Header.Get("X-Admin-Secret"); header != "" {
		return header
	}
	return r.URL.Query().Get("secret")
}

// requireAdmin writes the 401 itself and reports whether the caller may
// proceed. The body carries no detail, so a wrong secret and an unset one are
// indistinguishable from outside.
func (app *App) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if secretMatches(adminSecret(), presentedSecret(r)) {
		return true
	}
	app.logger.Warn("unauthorized admin request", "path", r.URL.Path, "ip", r.RemoteAddr)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return false
}

func (app *App) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/admin/sessions", app.adminListHandler)
	mux.HandleFunc("GET /api/admin/sessions/{id}", app.adminGetHandler)
	mux.HandleFunc("DELETE /api/admin/sessions/{id}", app.adminDeleteHandler)
	mux.HandleFunc("POST /api/admin/sessions/delete", app.adminDeleteManyHandler)
	mux.HandleFunc("POST /api/admin/purge", app.adminPurgeHandler)
	mux.HandleFunc("GET /api/admin/stats", app.adminStatsHandler)
}

// --- Transcript metrics ---

// transcriptMetrics is everything the admin page needs to know about a
// transcript without being sent the transcript. The list view would ship
// megabytes otherwise.
type transcriptMetrics struct {
	Turns            int
	ParticipantTurns int
	ParticipantWords int
	MirrorGenerated  int
	MirrorFallback   int
	// Declines is how many of the three stages the participant ended by taking
	// the optional invitation and adding nothing. Zero to three, and a measure
	// in its own right rather than only a health signal.
	Declines int
	// NotSerious is how many turns the host judged were not attempts to answer.
	// A row with several is a candidate for exclusion on data quality, which is
	// the whole reason the count is surfaced rather than just logged.
	NotSerious int
	// StageWords is participant words keyed by the chat sub-state
	// (STATE_CHAT_STAGE_1/2/3, STATE_CLOSING).
	StageWords map[string]int
}

func metricsFor(raw string) transcriptMetrics {
	metrics := transcriptMetrics{StageWords: map[string]int{}}
	if strings.TrimSpace(raw) == "" {
		return metrics
	}
	var transcript []ChatMessage
	if err := json.Unmarshal([]byte(raw), &transcript); err != nil {
		return metrics
	}
	for _, message := range transcript {
		// The comfort check-ins are stored as transcript entries so the rating
		// keeps its place in the conversation. They are not conversational
		// turns, so counting them would inflate every row by three.
		if message.IsAssessment {
			continue
		}
		metrics.Turns++
		if message.IsUser {
			words := len(strings.Fields(message.Text))
			metrics.ParticipantTurns++
			metrics.ParticipantWords += words
			metrics.StageWords[message.Stage] += words
			continue
		}
		// Only host turns carry the field, and only when they were produced at
		// runtime. Scripted turns have no mirror key at all.
		switch message.Mirror {
		case "generated":
			metrics.MirrorGenerated++
		case "fallback":
			metrics.MirrorFallback++
		case "declined":
			metrics.Declines++
		case "not-serious":
			metrics.NotSerious++
		}
	}
	return metrics
}

// --- Session list ---

type adminSessionSummary struct {
	UserID           string    `json:"user_id"`
	Condition        string    `json:"condition"`
	CurrentState     string    `json:"current_state"`
	ProlificID       string    `json:"prolific_id,omitempty"`
	DisplayName      string    `json:"display_name,omitempty"`
	TestMode         bool      `json:"test_mode"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
	DurationSeconds  float64   `json:"duration_seconds"`
	Turns            int       `json:"turns"`
	ParticipantTurns int       `json:"participant_turns"`
	ParticipantWords int       `json:"participant_words"`
	MirrorGenerated  int       `json:"mirror_generated"`
	MirrorFallback   int       `json:"mirror_fallback"`
	Declines         int       `json:"declines"`
	NotSerious       int       `json:"not_serious"`
	HasPostSurvey    bool      `json:"has_post_survey"`
}

// adminRow is one database row with its JSON columns still unparsed. Both the
// list and the stats read through this, so they can never disagree about what
// counts as a test row or how words are tallied.
type adminRow struct {
	userID       string
	condition    string
	currentState string
	preSurvey    string
	transcript   string
	postSurvey   string
	createdAt    time.Time
	updatedAt    time.Time
}

func (app *App) loadAdminRows() ([]adminRow, error) {
	rows, err := app.db.Query(`
        SELECT user_id, condition, current_state,
               COALESCE(pre_survey_data, ''), COALESCE(chat_transcript, ''),
               COALESCE(post_survey_data, ''), created_at, COALESCE(updated_at, '')
        FROM sessions ORDER BY created_at DESC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []adminRow
	for rows.Next() {
		var row adminRow
		var created, updated string
		if err := rows.Scan(&row.userID, &row.condition, &row.currentState,
			&row.preSurvey, &row.transcript, &row.postSurvey, &created, &updated); err != nil {
			return nil, err
		}
		if parsed, err := time.Parse(time.RFC3339Nano, created); err == nil {
			row.createdAt = parsed
		}
		if parsed, err := time.Parse(time.RFC3339Nano, updated); err == nil {
			row.updatedAt = parsed
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// stringField pulls one key out of a stored JSON blob, tolerating an absent or
// malformed blob, which is normal for a session abandoned at onboarding.
func stringField(raw, key string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ""
	}
	value, _ := parsed[key].(string)
	return value
}

func summarize(row adminRow) adminSessionSummary {
	metrics := metricsFor(row.transcript)
	summary := adminSessionSummary{
		UserID:           row.userID,
		Condition:        row.condition,
		CurrentState:     row.currentState,
		ProlificID:       stringField(row.preSurvey, "prolific_id"),
		DisplayName:      stringField(row.preSurvey, "display_name"),
		TestMode:         isTestSession(row.preSurvey),
		CreatedAt:        row.createdAt,
		UpdatedAt:        row.updatedAt,
		Turns:            metrics.Turns,
		ParticipantTurns: metrics.ParticipantTurns,
		ParticipantWords: metrics.ParticipantWords,
		MirrorGenerated:  metrics.MirrorGenerated,
		MirrorFallback:   metrics.MirrorFallback,
		Declines:         metrics.Declines,
		NotSerious:       metrics.NotSerious,
		HasPostSurvey:    strings.TrimSpace(row.postSurvey) != "",
	}
	if !row.updatedAt.IsZero() && !row.createdAt.IsZero() {
		summary.DurationSeconds = row.updatedAt.Sub(row.createdAt).Seconds()
	}
	return summary
}

func (app *App) adminListHandler(w http.ResponseWriter, r *http.Request) {
	if !app.requireAdmin(w, r) {
		return
	}
	rows, err := app.loadAdminRows()
	if err != nil {
		app.logger.Error("failed to list sessions", "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	summaries := make([]adminSessionSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, summarize(row))
	}
	writeJSON(w, map[string]any{"sessions": summaries}, app.logger)
}

func (app *App) adminGetHandler(w http.ResponseWriter, r *http.Request) {
	if !app.requireAdmin(w, r) {
		return
	}
	session, err := loadSession(app.db, r.PathValue("id"))
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	writeJSON(w, session, app.logger)
}

func (app *App) adminDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if !app.requireAdmin(w, r) {
		return
	}
	id := r.PathValue("id")
	// Shares deleteSessions rather than issuing its own statement, so a single
	// delete takes the session's cost rows with it exactly like a bulk one does.
	affected, err := deleteSessions(app.db, []string{id})
	if err != nil {
		app.logger.Error("failed to delete session", "sessionId", id, "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if affected == 0 {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	app.logger.Warn("admin deleted session", "sessionId", id)
	writeJSON(w, map[string]any{"deleted": affected}, app.logger)
}

// adminDeleteManyHandler removes an explicit list of ids in one transaction.
// The scoped purge below covers the routine cases; this covers picking rows out
// of the table by hand, which is what most development clearing actually is.
func (app *App) adminDeleteManyHandler(w http.ResponseWriter, r *http.Request) {
	if !app.requireAdmin(w, r) {
		return
	}
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// An empty list is a mistake rather than a request to delete nothing, and
	// treating it as a no-op would hide a frontend bug that sent no selection.
	if len(payload.IDs) == 0 {
		http.Error(w, "ids must not be empty", http.StatusBadRequest)
		return
	}

	deleted, err := deleteSessions(app.db, payload.IDs)
	if err != nil {
		app.logger.Error("bulk delete failed", "count", len(payload.IDs), "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	app.logger.Warn("admin deleted sessions by id", "requested", len(payload.IDs), "deleted", deleted)
	writeJSON(w, map[string]any{"deleted": deleted}, app.logger)
}

// --- Purge ---

const (
	purgeTestOnly   = "test_only"
	purgeIncomplete = "incomplete"
	purgeAll        = "all"
)

// purgeTargets picks the rows a scope covers. Deciding this in Go rather than
// in the DELETE statement keeps the test-row rule in one place, isTestSession,
// which is also what the allocator uses.
func purgeTargets(rows []adminRow, scope string) ([]string, error) {
	var ids []string
	for _, row := range rows {
		switch scope {
		case purgeAll:
			ids = append(ids, row.userID)
		case purgeTestOnly:
			if isTestSession(row.preSurvey) {
				ids = append(ids, row.userID)
			}
		case purgeIncomplete:
			// Test rows are incomplete more often than not, and a researcher
			// clearing abandonment is not asking to clear their own
			// walkthroughs, so leave those to test_only.
			if SessionState(row.currentState) != StateComplete && !isTestSession(row.preSurvey) {
				ids = append(ids, row.userID)
			}
		default:
			return nil, fmt.Errorf("unknown scope %q", scope)
		}
	}
	return ids, nil
}

func (app *App) adminPurgeHandler(w http.ResponseWriter, r *http.Request) {
	if !app.requireAdmin(w, r) {
		return
	}
	var payload struct {
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	rows, err := app.loadAdminRows()
	if err != nil {
		app.logger.Error("failed to read sessions for purge", "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	ids, err := purgeTargets(rows, payload.Scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	deleted, err := deleteSessions(app.db, ids)
	if err != nil {
		app.logger.Error("purge failed", "scope", payload.Scope, "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	app.logger.Warn("admin purged sessions", "scope", payload.Scope, "deleted", deleted)
	writeJSON(w, map[string]any{"scope": payload.Scope, "deleted": deleted}, app.logger)
}

// deleteSessions removes the named rows in one transaction, so a purge either
// takes effect or does not, rather than stopping halfway through.
func deleteSessions(db *sql.DB, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	statement, err := tx.Prepare(`DELETE FROM sessions WHERE user_id = ?`)
	if err != nil {
		return 0, err
	}
	defer statement.Close()

	// The cost rows go with the session. SQLite enforces no cascade here, so
	// leaving them would make the cost totals describe sessions that are gone.
	// What was actually spent is still on the OpenRouter dashboard.
	usageStatement, err := tx.Prepare(`DELETE FROM mirror_usage WHERE session_id = ?`)
	if err != nil {
		return 0, err
	}
	defer usageStatement.Close()

	deleted := 0
	for _, id := range ids {
		result, err := statement.Exec(id)
		if err != nil {
			return 0, err
		}
		if _, err := usageStatement.Exec(id); err != nil {
			return 0, err
		}
		affected, _ := result.RowsAffected()
		deleted += int(affected)
	}
	return deleted, tx.Commit()
}

// --- Statistics ---

type conditionStat struct {
	Condition string `json:"condition"`
	Completed int    `json:"completed"`
	// InProgress counts real sessions past onboarding that have not finished,
	// which is the abandonment picture. Onboarding rows are page views, and the
	// allocator ignores them for the same reason.
	InProgress         int            `json:"in_progress"`
	AbandonedAtIntake  int            `json:"abandoned_at_intake"`
	MedianWordsByStage map[string]int `json:"median_words_by_stage"`
}

type adminStats struct {
	TargetPerCondition int             `json:"target_per_condition"`
	Conditions         []conditionStat `json:"conditions"`
	DropOffByState     map[string]int  `json:"drop_off_by_state"`
	TotalSessions      int             `json:"total_sessions"`
	TestSessions       int             `json:"test_sessions"`
	// Mirror health. A sustained rise in fallbacks means generation is failing
	// and recruitment should pause, so this is the number to watch daily.
	MirrorGenerated       int     `json:"mirror_generated"`
	MirrorFallback        int     `json:"mirror_fallback"`
	RecentMirrorGenerated int     `json:"recent_mirror_generated"`
	RecentMirrorFallback  int     `json:"recent_mirror_fallback"`
	RecentSessionCount    int     `json:"recent_session_count"`
	EmptyTranscripts      int     `json:"empty_transcripts"`
	MedianCompletionSecs  float64 `json:"median_completion_seconds"`
	// Inference cost, from the mirror_usage table rather than the transcripts,
	// so it survives even when a transcript does not. Test sessions are excluded
	// from every figure here, the same as everywhere else on this page. USD as
	// charged by OpenRouter.
	MirrorCost mirrorCostStats `json:"mirror_cost"`
}

// mirrorCostStats is what the paper needs. The per-participant mean is the
// reportable number, and the per-condition breakdown is not flat, since the
// history sent to the model grows through the chat and 1-1 has fewer turns in it.
type mirrorCostStats struct {
	Calls          int `json:"calls"`
	FailedCalls    int `json:"failed_calls"`
	EmptyInputs    int `json:"empty_inputs"`
	SessionsBilled int `json:"sessions_billed"`
	// PromptTokens and CompletionTokens are totals, useful for a methods
	// sentence that has to survive a price change.
	PromptTokens     int                `json:"prompt_tokens"`
	CompletionTokens int                `json:"completion_tokens"`
	CostTotal        float64            `json:"cost_total"`
	CostPerSession   float64            `json:"cost_per_session"`
	CostByCondition  map[string]float64 `json:"cost_by_condition"`
	Model            string             `json:"model"`
}

// mirrorCostRow is one recorded call joined to the session that produced it. The
// join carries pre_survey_data so test rows can be dropped with isTestSession,
// which is the same rule the allocator and every other figure here uses.
type mirrorCostRow struct {
	sessionID  string
	condition  string
	preSurvey  string
	outcome    string
	prompt     int
	completion int
	cost       float64
}

func (app *App) loadMirrorCostRows() ([]mirrorCostRow, error) {
	rows, err := app.db.Query(`
        SELECT u.session_id, s.condition, COALESCE(s.pre_survey_data, ''),
               u.outcome, u.prompt_tokens, u.completion_tokens, u.cost
        FROM mirror_usage u
        JOIN sessions s ON s.user_id = u.session_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []mirrorCostRow
	for rows.Next() {
		var row mirrorCostRow
		if err := rows.Scan(&row.sessionID, &row.condition, &row.preSurvey,
			&row.outcome, &row.prompt, &row.completion, &row.cost); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// computeMirrorCosts aggregates the recorded calls. The per-session mean divides
// by sessions that were actually billed rather than by every session in the
// table, since a session that abandoned before the chat has no calls and would
// otherwise drag the mean toward zero.
func computeMirrorCosts(rows []mirrorCostRow) mirrorCostStats {
	stats := mirrorCostStats{
		CostByCondition: map[string]float64{},
		Model:           mirrorModel,
	}
	billed := map[string]bool{}
	sessionsByCondition := map[string]map[string]bool{}

	for _, row := range rows {
		if isTestSession(row.preSurvey) {
			continue
		}
		stats.Calls++
		switch row.outcome {
		case mirrorOutcomeFallback:
			stats.FailedCalls++
		case mirrorOutcomeEmptyInput:
			stats.EmptyInputs++
		}
		stats.PromptTokens += row.prompt
		stats.CompletionTokens += row.completion
		stats.CostTotal += row.cost
		stats.CostByCondition[row.condition] += row.cost
		if row.cost > 0 {
			billed[row.sessionID] = true
			if sessionsByCondition[row.condition] == nil {
				sessionsByCondition[row.condition] = map[string]bool{}
			}
			sessionsByCondition[row.condition][row.sessionID] = true
		}
	}

	stats.SessionsBilled = len(billed)
	if stats.SessionsBilled > 0 {
		stats.CostPerSession = stats.CostTotal / float64(stats.SessionsBilled)
	}
	// Per condition the useful figure is also per participant, not the pot spent
	// on a cell that happens to have run more sessions than the others.
	for condition, total := range stats.CostByCondition {
		if count := len(sessionsByCondition[condition]); count > 0 {
			stats.CostByCondition[condition] = total / float64(count)
		}
	}
	return stats
}

// recentSessionWindow is how many of the newest real sessions the mirror-health
// figure looks at. Small enough that a bad afternoon shows up rather than being
// averaged away by every healthy session ever collected.
const recentSessionWindow = 20

func medianInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// computeStats works off the same rows the list does. Test rows are excluded
// from every recruitment figure and counted separately, matching what the
// allocator actually enforces.
func computeStats(rows []adminRow, conditions []string) adminStats {
	stats := adminStats{
		TargetPerCondition: targetUsersPerCondition,
		DropOffByState:     map[string]int{},
		TotalSessions:      len(rows),
	}

	completed := map[string]int{}
	inProgress := map[string]int{}
	atIntake := map[string]int{}
	wordsByConditionStage := map[string]map[string][]int{}
	var completionDurations []float64
	realSeen := 0

	for _, row := range rows {
		if isTestSession(row.preSurvey) {
			stats.TestSessions++
			continue
		}
		metrics := metricsFor(row.transcript)
		stats.MirrorGenerated += metrics.MirrorGenerated
		stats.MirrorFallback += metrics.MirrorFallback

		// Rows arrive newest first, so the first N real ones are the window.
		if realSeen < recentSessionWindow {
			stats.RecentMirrorGenerated += metrics.MirrorGenerated
			stats.RecentMirrorFallback += metrics.MirrorFallback
			stats.RecentSessionCount++
		}
		realSeen++

		stats.DropOffByState[row.currentState]++

		switch SessionState(row.currentState) {
		case StateOnboarding:
			atIntake[row.condition]++
		case StateComplete:
			completed[row.condition]++
			if !row.updatedAt.IsZero() && !row.createdAt.IsZero() {
				completionDurations = append(completionDurations, row.updatedAt.Sub(row.createdAt).Seconds())
			}
		default:
			inProgress[row.condition]++
		}

		// A session that reached the post-survey with nothing in the transcript
		// is the demo-mode fallback showing up in the data. The participant
		// walked the whole flow against local state and saved nothing.
		if metrics.Turns == 0 && (SessionState(row.currentState) == StatePostSurvey || SessionState(row.currentState) == StateComplete) {
			stats.EmptyTranscripts++
		}

		if wordsByConditionStage[row.condition] == nil {
			wordsByConditionStage[row.condition] = map[string][]int{}
		}
		for stage, words := range metrics.StageWords {
			wordsByConditionStage[row.condition][stage] = append(wordsByConditionStage[row.condition][stage], words)
		}
	}

	ordered := append([]string(nil), conditions...)
	sort.Strings(ordered)
	for _, condition := range ordered {
		stat := conditionStat{
			Condition:          condition,
			Completed:          completed[condition],
			InProgress:         inProgress[condition],
			AbandonedAtIntake:  atIntake[condition],
			MedianWordsByStage: map[string]int{},
		}
		for stage, values := range wordsByConditionStage[condition] {
			stat.MedianWordsByStage[stage] = medianInt(values)
		}
		stats.Conditions = append(stats.Conditions, stat)
	}

	stats.MedianCompletionSecs = medianFloat(completionDurations)
	return stats
}

func (app *App) adminStatsHandler(w http.ResponseWriter, r *http.Request) {
	if !app.requireAdmin(w, r) {
		return
	}
	rows, err := app.loadAdminRows()
	if err != nil {
		app.logger.Error("failed to read sessions for stats", "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	stats := computeStats(rows, app.conditions)

	costRows, err := app.loadMirrorCostRows()
	if err != nil {
		app.logger.Error("failed to read mirror usage for stats", "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	stats.MirrorCost = computeMirrorCosts(costRows)

	writeJSON(w, stats, app.logger)
}
