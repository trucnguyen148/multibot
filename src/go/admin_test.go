package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecretMatches(t *testing.T) {
	cases := []struct {
		name      string
		expected  string
		presented string
		want      bool
	}{
		// An unset secret must deny rather than allow. A deployment that forgot
		// the variable is the one case where failing open would be worst.
		{"unset secret denies everything", "", "", false},
		{"unset secret denies a guess", "", "anything", false},
		{"wrong secret", "correct", "wrong", false},
		{"empty presented", "correct", "", false},
		{"prefix of the secret", "correct", "corr", false},
		{"correct secret", "correct", "correct", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := secretMatches(tc.expected, tc.presented); got != tc.want {
				t.Fatalf("secretMatches(%q, %q) = %v, want %v", tc.expected, tc.presented, got, tc.want)
			}
		})
	}
}

func TestPresentedSecretPrefersTheHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/admin/stats?secret=from-url", nil)
	request.Header.Set("X-Admin-Secret", "from-header")
	if got := presentedSecret(request); got != "from-header" {
		t.Fatalf("expected the header to win, got %q", got)
	}

	bare := httptest.NewRequest(http.MethodGet, "/api/admin/stats?secret=from-url", nil)
	if got := presentedSecret(bare); got != "from-url" {
		t.Fatalf("expected the query parameter as a fallback, got %q", got)
	}
}

// --- Test app with a real database ---

func newAdminTestApp(t *testing.T) *App {
	t.Helper()
	db, err := initDB(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return &App{
		db:         db,
		conditions: []string{"1-1", "2-1", "3-1"},
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func insertRow(t *testing.T, db *sql.DB, id, condition string, state SessionState, preSurvey, transcript string, created, updated time.Time) {
	t.Helper()
	_, err := db.Exec(`
        INSERT INTO sessions (user_id, condition, current_state, pre_survey_data, chat_transcript, post_survey_data, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		id, condition, string(state), preSurvey, transcript,
		created.Format(time.RFC3339Nano), updated.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

const sampleTranscript = `[
  {"sender":"Vieno","text":"How has the week been?","stage":"STATE_CHAT_STAGE_1"},
  {"sender":"You","text":"one two three four five","stage":"STATE_CHAT_STAGE_1","isUser":true},
  {"sender":"Vieno","text":"Thanks for saying that.","stage":"STATE_CHAT_STAGE_1","mirror":"generated"},
  {"sender":"You","text":"six seven","stage":"STATE_CHAT_STAGE_2","isUser":true},
  {"sender":"Vieno","text":"Thanks for sharing that.","stage":"STATE_CHAT_STAGE_2","mirror":"fallback"}
]`

func TestMetricsForCountsParticipantWordsAndMirrorTurns(t *testing.T) {
	metrics := metricsFor(sampleTranscript)

	if metrics.Turns != 5 {
		t.Fatalf("Turns = %d, want 5", metrics.Turns)
	}
	if metrics.ParticipantTurns != 2 {
		t.Fatalf("ParticipantTurns = %d, want 2", metrics.ParticipantTurns)
	}
	if metrics.ParticipantWords != 7 {
		t.Fatalf("ParticipantWords = %d, want 7", metrics.ParticipantWords)
	}
	if metrics.MirrorGenerated != 1 || metrics.MirrorFallback != 1 {
		t.Fatalf("mirror counts = %d generated / %d fallback, want 1 / 1",
			metrics.MirrorGenerated, metrics.MirrorFallback)
	}
	if metrics.StageWords["STATE_CHAT_STAGE_1"] != 5 {
		t.Fatalf("stage 1 words = %d, want 5", metrics.StageWords["STATE_CHAT_STAGE_1"])
	}
	if metrics.StageWords["STATE_CHAT_STAGE_2"] != 2 {
		t.Fatalf("stage 2 words = %d, want 2", metrics.StageWords["STATE_CHAT_STAGE_2"])
	}
}

func TestMetricsForToleratesEmptyAndMalformedTranscripts(t *testing.T) {
	for _, raw := range []string{"", "   ", "not json", "null"} {
		metrics := metricsFor(raw)
		if metrics.Turns != 0 || metrics.StageWords == nil {
			t.Fatalf("metricsFor(%q) = %#v, want a zeroed value with a usable map", raw, metrics)
		}
	}
}

// --- Authentication at the endpoint, which is the security property ---

func TestAdminEndpointsRejectRequestsWithoutTheSecret(t *testing.T) {
	app := newAdminTestApp(t)
	t.Setenv("EXPORT_SECRET", "s3cret")

	mux := http.NewServeMux()
	app.registerAdminRoutes(mux)
	mux.HandleFunc("/api/export", app.exportHandler)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/admin/sessions", ""},
		{http.MethodGet, "/api/admin/sessions/sess-1", ""},
		{http.MethodDelete, "/api/admin/sessions/sess-1", ""},
		{http.MethodPost, "/api/admin/purge", `{"scope":"all"}`},
		{http.MethodGet, "/api/admin/stats", ""},
		{http.MethodGet, "/api/export", ""},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			// No secret at all.
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("without a secret: status %d, want 401", recorder.Code)
			}

			// Wrong secret.
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("X-Admin-Secret", "wrong")
			recorder = httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("with a wrong secret: status %d, want 401", recorder.Code)
			}
		})
	}
}

func TestAdminEndpointsDenyEverythingWhenTheSecretIsUnset(t *testing.T) {
	app := newAdminTestApp(t)
	t.Setenv("EXPORT_SECRET", "")

	mux := http.NewServeMux()
	app.registerAdminRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	request.Header.Set("X-Admin-Secret", "")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401 when EXPORT_SECRET is unset", recorder.Code)
	}
}

// --- The list view must not ship transcripts ---

func TestAdminListCarriesNoTranscript(t *testing.T) {
	app := newAdminTestApp(t)
	t.Setenv("EXPORT_SECRET", "s3cret")
	now := time.Now().UTC()
	insertRow(t, app.db, "sess-1", "2-1", StateComplete,
		`{"prolific_id":"abc","display_name":"Kim"}`, sampleTranscript, now, now.Add(9*time.Minute))

	request := httptest.NewRequest(http.MethodGet, "/api/admin/sessions", nil)
	request.Header.Set("X-Admin-Secret", "s3cret")
	recorder := httptest.NewRecorder()
	app.adminListHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	// The literal text of a participant turn must not appear anywhere in the
	// list response, or the table ships every transcript it renders a row for.
	if strings.Contains(body, "one two three four five") {
		t.Fatalf("list response leaked transcript text: %s", body)
	}

	var payload struct {
		Sessions []adminSessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(payload.Sessions))
	}
	summary := payload.Sessions[0]
	if summary.ProlificID != "abc" || summary.DisplayName != "Kim" {
		t.Fatalf("identifiers not surfaced: %#v", summary)
	}
	if summary.ParticipantWords != 7 || summary.MirrorFallback != 1 {
		t.Fatalf("metrics not derived: %#v", summary)
	}
	if summary.DurationSeconds != 540 {
		t.Fatalf("DurationSeconds = %v, want 540", summary.DurationSeconds)
	}
}

// --- Purge scopes ---

func TestPurgeTargets(t *testing.T) {
	rows := []adminRow{
		{userID: "real-complete", currentState: string(StateComplete), preSurvey: `{"prolific_id":"abc"}`},
		{userID: "real-abandoned", currentState: string(StatePreSurvey), preSurvey: `{"prolific_id":"def"}`},
		{userID: "test-complete", currentState: string(StateComplete), preSurvey: `{"test_mode":true}`},
		{userID: "test-abandoned", currentState: string(StateOnboarding), preSurvey: `{"test_mode":true}`},
	}

	cases := []struct {
		scope string
		want  []string
	}{
		{purgeAll, []string{"real-complete", "real-abandoned", "test-complete", "test-abandoned"}},
		{purgeTestOnly, []string{"test-complete", "test-abandoned"}},
		// Deliberately excludes test rows: clearing abandonment is not the same
		// request as clearing your own walkthroughs.
		{purgeIncomplete, []string{"real-abandoned"}},
	}
	for _, tc := range cases {
		t.Run(tc.scope, func(t *testing.T) {
			got, err := purgeTargets(rows, tc.scope)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("purgeTargets(%q) = %v, want %v", tc.scope, got, tc.want)
			}
		})
	}
}

func TestPurgeRejectsAnUnknownScope(t *testing.T) {
	// An unrecognised scope must not quietly become "all".
	if _, err := purgeTargets([]adminRow{{userID: "x"}}, "everything"); err == nil {
		t.Fatal("expected an unknown scope to be rejected")
	}
	if _, err := purgeTargets([]adminRow{{userID: "x"}}, ""); err == nil {
		t.Fatal("expected an empty scope to be rejected")
	}
}

func TestAdminPurgeDeletesOnlyTheScopedRows(t *testing.T) {
	app := newAdminTestApp(t)
	t.Setenv("EXPORT_SECRET", "s3cret")
	now := time.Now().UTC()
	insertRow(t, app.db, "real", "1-1", StateComplete, `{"prolific_id":"abc"}`, sampleTranscript, now, now)
	insertRow(t, app.db, "test", "2-1", StateComplete, `{"test_mode":true}`, sampleTranscript, now, now)

	request := httptest.NewRequest(http.MethodPost, "/api/admin/purge", strings.NewReader(`{"scope":"test_only"}`))
	request.Header.Set("X-Admin-Secret", "s3cret")
	recorder := httptest.NewRecorder()
	app.adminPurgeHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var result struct {
		Deleted int `json:"deleted"`
	}
	json.Unmarshal(recorder.Body.Bytes(), &result)
	if result.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", result.Deleted)
	}

	remaining, err := app.loadAdminRows()
	if err != nil {
		t.Fatalf("loadAdminRows: %v", err)
	}
	if len(remaining) != 1 || remaining[0].userID != "real" {
		t.Fatalf("wrong rows survived: %#v", remaining)
	}
}

func TestAdminDeleteRemovesOneRowAnd404sOnTheSecondTry(t *testing.T) {
	app := newAdminTestApp(t)
	t.Setenv("EXPORT_SECRET", "s3cret")
	now := time.Now().UTC()
	insertRow(t, app.db, "sess-1", "1-1", StateComplete, `{"prolific_id":"abc"}`, sampleTranscript, now, now)

	mux := http.NewServeMux()
	app.registerAdminRoutes(mux)

	call := func() int {
		request := httptest.NewRequest(http.MethodDelete, "/api/admin/sessions/sess-1", nil)
		request.Header.Set("X-Admin-Secret", "s3cret")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder.Code
	}

	if code := call(); code != http.StatusOK {
		t.Fatalf("first delete: status %d, want 200", code)
	}
	if code := call(); code != http.StatusNotFound {
		t.Fatalf("second delete: status %d, want 404", code)
	}
}

// --- Statistics ---

func TestComputeStatsSeparatesTestRowsAndCountsRecruitment(t *testing.T) {
	now := time.Now().UTC()
	rows := []adminRow{
		{userID: "a", condition: "1-1", currentState: string(StateComplete),
			preSurvey: `{"prolific_id":"a"}`, transcript: sampleTranscript,
			createdAt: now, updatedAt: now.Add(10 * time.Minute)},
		{userID: "b", condition: "1-1", currentState: string(StatePostSurvey),
			preSurvey: `{"prolific_id":"b"}`, transcript: sampleTranscript, createdAt: now},
		{userID: "c", condition: "2-1", currentState: string(StateOnboarding),
			preSurvey: `{"prolific_id":"c"}`, createdAt: now},
		{userID: "t", condition: "3-1", currentState: string(StateComplete),
			preSurvey: `{"test_mode":true}`, transcript: sampleTranscript,
			createdAt: now, updatedAt: now.Add(2 * time.Minute)},
	}

	stats := computeStats(rows, []string{"1-1", "2-1", "3-1"})

	if stats.TestSessions != 1 {
		t.Fatalf("TestSessions = %d, want 1", stats.TestSessions)
	}
	if stats.TotalSessions != 4 {
		t.Fatalf("TotalSessions = %d, want 4", stats.TotalSessions)
	}
	if stats.TargetPerCondition != targetUsersPerCondition {
		t.Fatalf("TargetPerCondition = %d, want the constant %d", stats.TargetPerCondition, targetUsersPerCondition)
	}

	byCondition := map[string]conditionStat{}
	for _, stat := range stats.Conditions {
		byCondition[stat.Condition] = stat
	}
	if got := byCondition["1-1"]; got.Completed != 1 || got.InProgress != 1 {
		t.Fatalf("1-1 = %#v, want 1 completed and 1 in progress", got)
	}
	// The test walkthrough completed in 3-1 and must not be counted there.
	if got := byCondition["3-1"]; got.Completed != 0 {
		t.Fatalf("3-1 Completed = %d, want 0; test rows must not count", got.Completed)
	}
	if got := byCondition["2-1"]; got.AbandonedAtIntake != 1 || got.InProgress != 0 {
		t.Fatalf("2-1 = %#v, want the onboarding row counted as intake only", got)
	}

	// Only the real completion contributes, so the median is its own duration.
	if stats.MedianCompletionSecs != 600 {
		t.Fatalf("MedianCompletionSecs = %v, want 600", stats.MedianCompletionSecs)
	}
	// Mirror totals exclude the test row's two mirror turns.
	if stats.MirrorGenerated != 2 || stats.MirrorFallback != 2 {
		t.Fatalf("mirror totals = %d / %d, want 2 / 2", stats.MirrorGenerated, stats.MirrorFallback)
	}
	if words := byCondition["1-1"].MedianWordsByStage["STATE_CHAT_STAGE_1"]; words != 5 {
		t.Fatalf("median stage 1 words = %d, want 5", words)
	}
}

func TestComputeStatsCountsEmptyTranscriptsPastTheChat(t *testing.T) {
	now := time.Now().UTC()
	rows := []adminRow{
		// Reached the post-survey with nothing recorded: the demo-mode fallback.
		{userID: "silent", condition: "1-1", currentState: string(StateComplete),
			preSurvey: `{"prolific_id":"a"}`, transcript: "", createdAt: now, updatedAt: now},
		// Still mid-flow with no transcript yet, which is normal.
		{userID: "early", condition: "1-1", currentState: string(StatePreSurvey),
			preSurvey: `{"prolific_id":"b"}`, transcript: "", createdAt: now},
	}

	stats := computeStats(rows, []string{"1-1"})
	if stats.EmptyTranscripts != 1 {
		t.Fatalf("EmptyTranscripts = %d, want 1", stats.EmptyTranscripts)
	}
}

func TestMedianHelpers(t *testing.T) {
	if got := medianInt(nil); got != 0 {
		t.Fatalf("medianInt(nil) = %d, want 0", got)
	}
	if got := medianInt([]int{5, 1, 3}); got != 3 {
		t.Fatalf("medianInt odd = %d, want 3", got)
	}
	if got := medianInt([]int{1, 2, 3, 10}); got != 2 {
		t.Fatalf("medianInt even = %d, want 2", got)
	}
	if got := medianFloat([]float64{4, 1}); got != 2.5 {
		t.Fatalf("medianFloat even = %v, want 2.5", got)
	}
}

// updated_at arrived after the table did, so the migration has to run against a
// database that already exists rather than only on a fresh one.
func TestInitDBAddsUpdatedAtToAnExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// The schema exactly as it stood before this change.
	_, err = db.Exec(`
        CREATE TABLE sessions (
            user_id TEXT PRIMARY KEY,
            condition TEXT NOT NULL,
            current_state TEXT NOT NULL,
            pre_survey_data TEXT,
            chat_transcript TEXT,
            stage_1_comfort_score INTEGER,
            stage_2_comfort_score INTEGER,
            post_survey_data TEXT,
            created_at TEXT NOT NULL
        )`)
	if err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	// Written the way saveSession writes it, with empty strings rather than
	// NULLs, since that is what the production rows actually contain.
	if _, err := db.Exec(`INSERT INTO sessions (user_id, condition, current_state, pre_survey_data, chat_transcript, post_survey_data, created_at) VALUES ('old', '1-1', 'STATE_COMPLETE', '', '', '', ?)`,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	db.Close()

	migrated, err := initDB(path)
	if err != nil {
		t.Fatalf("initDB against an existing database: %v", err)
	}
	defer migrated.Close()

	// Running twice must be a no-op, since the service restarts on every deploy.
	again, err := initDB(path)
	if err != nil {
		t.Fatalf("initDB is not idempotent: %v", err)
	}
	again.Close()

	app := &App{db: migrated, conditions: []string{"1-1"}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rows, err := app.loadAdminRows()
	if err != nil {
		t.Fatalf("loadAdminRows after migration: %v", err)
	}
	if len(rows) != 1 || !rows[0].updatedAt.IsZero() {
		t.Fatalf("expected the legacy row to survive with no updated_at, got %#v", rows)
	}

	// And a save through the normal path fills it in.
	session, err := loadSession(migrated, "old")
	if err != nil {
		t.Fatalf("loadSession: %v", err)
	}
	if err := saveSession(migrated, session); err != nil {
		t.Fatalf("saveSession: %v", err)
	}
	rows, _ = app.loadAdminRows()
	if rows[0].updatedAt.IsZero() {
		t.Fatal("expected saveSession to stamp updated_at")
	}
}
