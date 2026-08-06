package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

// --- Types & Structs ---

type SessionState string

const (
	StateOnboarding  SessionState = "STATE_ONBOARDING"
	StatePreSurvey   SessionState = "STATE_PRE_SURVEY"
	StateInteraction SessionState = "STATE_INTERACTION"
	StatePostSurvey  SessionState = "STATE_POST_SURVEY"
	StateComplete    SessionState = "STATE_COMPLETE"
)

type ChatMessage struct {
	ID              string    `json:"id,omitempty"`
	Sender          string    `json:"sender"`
	Text            string    `json:"text"`
	Timestamp       time.Time `json:"timestamp"`
	Stage           string    `json:"stage"`
	IsUser          bool      `json:"isUser,omitempty"`
	IsAssessment    bool      `json:"isAssessment,omitempty"`
	AssessmentScore *int      `json:"assessmentScore,omitempty"`
	// Mirror marks a host turn produced at runtime rather than read from the
	// script: "generated" when the model answered, "fallback" when it did not
	// and the fixed line was used. Absent on every scripted turn.
	//
	// Without this the export cannot separate generated text from script when
	// the transcripts are coded, and cannot tell a real acknowledgement from a
	// timed-out one. The field has to exist here as well as in the frontend
	// because the transcript is unmarshalled into this struct, so an unknown
	// key would be silently discarded on the way into the database.
	Mirror string `json:"mirror,omitempty"`
}

type Session struct {
	UserID             string          `json:"user_id"`
	Condition          string          `json:"condition"`
	CurrentState       SessionState    `json:"current_state"`
	PreSurveyData      json.RawMessage `json:"pre_survey_data"`
	ChatTranscript     []ChatMessage   `json:"chat_transcript"`
	Stage1ComfortScore *int            `json:"stage_1_comfort_score"`
	Stage2ComfortScore *int            `json:"stage_2_comfort_score"`
	Stage3ComfortScore *int            `json:"stage_3_comfort_score"`
	PostSurveyData     json.RawMessage `json:"post_survey_data"`
	CreatedAt          time.Time       `json:"created_at"`
}

type BotScript struct {
	Sender string `json:"sender"`
	Text   string `json:"text"`
	// Tag classifies the turn so the design invariants in scripts_test.go can be
	// checked mechanically. One of: open, peer-neutral, question, disclosure, ack.
	Tag string `json:"tag,omitempty"`
}

type StageConfig struct {
	Type           string                 `json:"type"`
	CurrentState   string                 `json:"currentState"`
	Condition      string                 `json:"condition"`
	Title          string                 `json:"title"`
	AllScripts     map[string][]BotScript `json:"allScripts,omitempty"`
	CompletionCode string                 `json:"completionCode,omitempty"`
}

type experimentData struct {
	Conditions map[string]struct {
		Label  string                 `json:"label"`
		Stages map[string][]BotScript `json:"stages"`
	} `json:"conditions"`
}

type App struct {
	db         *sql.DB
	data       experimentData
	conditions []string
	logger     *slog.Logger
}

const (
	// Recruitment stops once every condition reaches this many *completed*
	// sessions. Abandoned and test sessions do not count against it.
	targetUsersPerCondition = 60
	// Derived total across the three conditions. Recorded for reference; the
	// allocator enforces the per-condition target above, not this number.
	maximumUsers = targetUsersPerCondition * 3
)

// --- Main Entry Point ---

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	root, err := os.Getwd()
	if err != nil {
		logger.Error("failed to get working directory", "error", err)
		os.Exit(1)
	}

	port := getEnvOrDefault("PORT", "8080")
	dataDir := getEnvOrDefault("DATA_DIR", root)
	allowedOrigin := getEnvOrDefault("ALLOWED_ORIGIN", "*")

	dataPath := filepath.Join(root, "data.json")
	content, err := os.ReadFile(dataPath)
	if err != nil {
		logger.Error("failed to read data.json", "error", err)
		os.Exit(1)
	}

	var appData experimentData
	if err := json.Unmarshal(content, &appData); err != nil {
		logger.Error("failed to parse data.json", "error", err)
		os.Exit(1)
	}

	var conditions []string
	for k := range appData.Conditions {
		conditions = append(conditions, k)
	}

	// The host acknowledging what the participant wrote is part of the design,
	// not a feature that can be switched off, so the key it needs is simply
	// required. Without it every acknowledgement would silently degrade to the
	// fixed fallback line and the whole sample would be the scripted study
	// wearing the mirroring study's name.
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		logger.Error("OPENROUTER_API_KEY is not set; the host cannot acknowledge participant messages without it")
		os.Exit(1)
	}
	logger.Info("mirroring host configured", "model", mirrorModel)

	dbPath := filepath.Join(dataDir, "sessions.db")
	db, err := initDB(dbPath)
	if err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	app := &App{
		db:         db,
		data:       appData,
		conditions: conditions,
		logger:     logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.healthHandler)
	mux.HandleFunc("/api/session/init", app.createSessionHandler)
	mux.HandleFunc("/api/stage", app.stageHandler)
	mux.HandleFunc("/api/submit", app.submitHandler)
	mux.HandleFunc("/api/export", app.exportHandler)
	mux.HandleFunc("/api/mirror", app.mirrorHandler)
	app.registerAdminRoutes(mux)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: app.withCORS(mux, allowedOrigin),
	}

	go func() {
		app.logger.Info("server starting", "port", port, "env", "production")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.logger.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	app.logger.Info("shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		app.logger.Error("server forced to shutdown", "error", err)
	}
	app.logger.Info("server exited safely")
}

// --- Middleware ---

func (app *App) withCORS(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		// DELETE and X-Admin-Secret are both here for /api/admin. Omitting
		// either fails the preflight, and the failure only shows up in the
		// browser: curl skips preflight entirely and works regardless.
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Secret")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Handlers ---

func (app *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (app *App) createSessionHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	testMode := isTruthy(query.Get("test"))
	requested := strings.TrimSpace(query.Get("condition"))
	requestedStart := strings.TrimSpace(query.Get("start"))

	var condition string
	var err error

	switch {
	case testMode:
		// A researcher walkthrough must never touch the allocator. It neither
		// consumes a participant slot nor gets turned away once recruitment is
		// full, which is exactly when you most want to walk the flow.
		condition, err = app.testCondition(requested)
		if err != nil {
			app.logger.Warn("rejected test session", "condition", requested, "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	default:
		// Forcing a cell is a test-mode privilege. Honouring it for a real
		// participant would let anyone who reads the bundle pick their own
		// condition and skew the allocation.
		if requested != "" {
			app.logger.Warn("ignored condition override outside test mode", "condition", requested)
		}
		condition, err = app.randomCondition()
		if err != nil {
			app.logger.Warn("condition assignment quota reached", "error", err)
			http.Error(w, "Condition allocation is full", http.StatusConflict)
			return
		}
	}

	session := &Session{
		UserID:       fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		Condition:    condition,
		CurrentState: StateOnboarding,
		CreatedAt:    time.Now().UTC(),
	}

	// Flag the row at creation rather than waiting for the onboarding submit, so
	// a walkthrough abandoned partway through is still excluded from the tally.
	if testMode {
		session.PreSurveyData = json.RawMessage(`{"test_mode":true}`)
	}

	// Starting partway through lets a researcher open the post-survey without
	// answering a pre-survey and a chat first. Resolved here rather than in the
	// frontend so the screen is the real one, driven by buildStageResponse, with
	// the live completion code and the real scripts for the cell.
	switch {
	case testMode && requestedStart != "":
		start, err := parseStartState(requestedStart)
		if err != nil {
			app.logger.Warn("rejected test session", "start", requestedStart, "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		session.CurrentState = start
	case requestedStart != "":
		// Same reasoning as the condition override. Honouring this for a real
		// participant would hand anyone a completion code in one request.
		app.logger.Warn("ignored start override outside test mode", "start", requestedStart)
	}

	if err := saveSession(app.db, session); err != nil {
		app.logger.Error("failed to save new session", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, session, app.logger)
}

func (app *App) stageHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	if sessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	session, err := loadSession(app.db, sessionID)
	if err != nil {
		app.logger.Warn("session not found", "sessionId", sessionID, "error", err)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	response := app.buildStageResponse(session)
	writeJSON(w, response, app.logger)
}

func (app *App) submitHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SessionID    string `json:"sessionId"`
		CurrentState string `json:"currentState"`
		ProlificID   string `json:"prolificId"`
		DisplayName  string `json:"displayName"`
		StudyID      string `json:"studyId"`
		// Prolific's own submission id, distinct from SessionID above.
		ProlificSessionID  string         `json:"prolificSessionId"`
		TestMode           bool           `json:"testMode"`
		PreSurveyData      map[string]any `json:"preSurveyData"`
		ChatTranscript     []ChatMessage  `json:"chatTranscript"`
		Stage1ComfortScore *int           `json:"stage1ComfortScore"`
		Stage2ComfortScore *int           `json:"stage2ComfortScore"`
		Stage3ComfortScore *int           `json:"stage3ComfortScore"`
		PostSurveyData     map[string]any `json:"postSurveyData"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		app.logger.Error("invalid request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	session, err := loadSession(app.db, payload.SessionID)
	if err != nil {
		app.logger.Warn("session not found for submission", "sessionId", payload.SessionID)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	switch payload.CurrentState {
	case "STATE_ONBOARDING":
		// Merged over whatever the row already carries (the test_mode flag
		// written at session creation) rather than replacing it.
		initialData := map[string]any{}
		if len(session.PreSurveyData) > 0 {
			json.Unmarshal(session.PreSurveyData, &initialData)
		}
		if payload.ProlificID != "" {
			initialData["prolific_id"] = payload.ProlificID
			if payload.DisplayName != "" {
				initialData["display_name"] = payload.DisplayName
			}
			// Prolific url params, kept so submissions can be reconciled against
			// Prolific's own export.
			if payload.StudyID != "" {
				initialData["study_id"] = payload.StudyID
			}
			if payload.ProlificSessionID != "" {
				initialData["prolific_session_id"] = payload.ProlificSessionID
			}
			// Researcher walked through with ?test=true. Exclude from analysis.
			if payload.TestMode {
				initialData["test_mode"] = true
			}
		}
		data, _ := json.Marshal(initialData)
		session.PreSurveyData = data
		session.CurrentState = StatePreSurvey

	case "STATE_PRE_SURVEY":
		if payload.PreSurveyData != nil {
			payload.PreSurveyData = sanitizeSurveyData(payload.PreSurveyData)
			var existingData map[string]any
			if len(session.PreSurveyData) > 0 {
				json.Unmarshal(session.PreSurveyData, &existingData)
				carryOver := []string{
					"prolific_id", "display_name", "study_id",
					"prolific_session_id", "test_mode",
				}
				for _, key := range carryOver {
					if value, ok := existingData[key]; ok {
						payload.PreSurveyData[key] = value
					}
				}
			}
			data, _ := json.Marshal(payload.PreSurveyData)
			session.PreSurveyData = data
		}
		session.CurrentState = StateInteraction

	case "STATE_INTERACTION":
		if payload.ChatTranscript != nil {
			data, _ := json.Marshal(payload.ChatTranscript)
			var transcript []ChatMessage
			if err := json.Unmarshal(data, &transcript); err == nil {
				session.ChatTranscript = transcript
			}
		}
		session.Stage1ComfortScore = payload.Stage1ComfortScore
		session.Stage2ComfortScore = payload.Stage2ComfortScore
		session.Stage3ComfortScore = payload.Stage3ComfortScore
		session.CurrentState = "STATE_POST_SURVEY"

	case "STATE_POST_SURVEY":
		if payload.PostSurveyData != nil {
			payload.PostSurveyData = sanitizeSurveyData(payload.PostSurveyData)
			data, _ := json.Marshal(payload.PostSurveyData)
			session.PostSurveyData = data
		}
		session.CurrentState = StateComplete

	default:
		app.logger.Warn("submit for unrecognized state", "sessionId", payload.SessionID, "state", payload.CurrentState)
	}

	if err := saveSession(app.db, session); err != nil {
		app.logger.Error("failed to save session", "error", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	nextStage := app.buildStageResponse(session)
	writeJSON(w, map[string]any{
		"sessionId":    session.UserID,
		"currentState": session.CurrentState,
		"nextStage":    nextStage,
	}, app.logger)
}

func (app *App) exportHandler(w http.ResponseWriter, r *http.Request) {
	// Shares the constant-time comparison in admin.go rather than reimplementing
	// it. This endpoint stays for scripts; /admin is the human path.
	if !secretMatches(adminSecret(), presentedSecret(r)) {
		app.logger.Warn("unauthorized export attempt", "ip", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := app.db.Query(`SELECT user_id, condition, current_state, pre_survey_data, chat_transcript, stage_1_comfort_score, stage_2_comfort_score, stage_3_comfort_score, post_survey_data, created_at FROM sessions`)
	if err != nil {
		app.logger.Error("failed to query sessions for export", "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		var pre, chat, post, created string
		var s1, s2, s3 sql.NullInt64
		if err := rows.Scan(&s.UserID, &s.Condition, &s.CurrentState, &pre, &chat, &s1, &s2, &s3, &post, &created); err != nil {
			continue
		}
		if pre != "" {
			s.PreSurveyData = []byte(pre)
		}
		if chat != "" {
			json.Unmarshal([]byte(chat), &s.ChatTranscript)
		}
		if post != "" {
			s.PostSurveyData = []byte(post)
		}
		if s1.Valid {
			val := int(s1.Int64)
			s.Stage1ComfortScore = &val
		}
		if s2.Valid {
			val := int(s2.Int64)
			s.Stage2ComfortScore = &val
		}
		if s3.Valid {
			val := int(s3.Int64)
			s.Stage3ComfortScore = &val
		}
		if parsed, err := time.Parse(time.RFC3339Nano, created); err == nil {
			s.CreatedAt = parsed
		}
		sessions = append(sessions, s)
	}

	app.logger.Info("export successful", "recordCount", len(sessions))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=study_data.json")
	json.NewEncoder(w).Encode(sessions)
}

// --- Helpers ---

// conditionTally separates the two questions the allocator has to answer.
// Recruitment stops on completed sessions, so abandoned attempts never
// permanently reduce the achievable sample. Balancing uses started sessions, so
// people who are mid-study still spread new arrivals across the other cells.
type conditionTally struct {
	started   map[string]int
	completed map[string]int
}

// isTestSession reports whether a row came from ?test=true. Those are researcher
// walkthroughs and must not consume participant slots.
func isTestSession(preSurveyData string) bool {
	if preSurveyData == "" {
		return false
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(preSurveyData), &parsed); err != nil {
		return false
	}
	flag, _ := parsed["test_mode"].(bool)
	return flag
}

func (app *App) tallyConditions() (conditionTally, error) {
	tally := conditionTally{
		started:   make(map[string]int, len(app.conditions)),
		completed: make(map[string]int, len(app.conditions)),
	}
	for _, condition := range app.conditions {
		tally.started[condition] = 0
		tally.completed[condition] = 0
	}

	rows, err := app.db.Query(`SELECT condition, current_state, COALESCE(pre_survey_data, '') FROM sessions`)
	if err != nil {
		return tally, err
	}
	defer rows.Close()

	for rows.Next() {
		var condition, currentState, preSurveyData string
		if err := rows.Scan(&condition, &currentState, &preSurveyData); err != nil {
			return tally, err
		}
		if _, known := tally.started[condition]; !known {
			continue
		}
		if isTestSession(preSurveyData) {
			continue
		}
		// A row is written the instant the page loads, so anyone still sitting
		// at onboarding is a page view rather than a participant.
		if SessionState(currentState) == StateOnboarding {
			continue
		}
		tally.started[condition]++
		if SessionState(currentState) == StateComplete {
			tally.completed[condition]++
		}
	}
	return tally, rows.Err()
}

// startStates are the screens a researcher link can open directly, keyed by the
// shorthand used in the URL. The full `STATE_` spelling is accepted too, so a
// state copied out of the export or the admin table works without translation.
var startStates = map[string]SessionState{
	"onboarding":  StateOnboarding,
	"pre":         StatePreSurvey,
	"pre_survey":  StatePreSurvey,
	"chat":        StateInteraction,
	"interaction": StateInteraction,
	"post":        StatePostSurvey,
	"post_survey": StatePostSurvey,
	"complete":    StateComplete,
}

// parseStartState rejects anything it does not recognise rather than falling
// back to onboarding, so a typo fails loudly instead of quietly opening the
// wrong screen and looking like the link is broken.
func parseStartState(value string) (SessionState, error) {
	key := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "state_")
	if state, known := startStates[key]; known {
		return state, nil
	}
	return "", fmt.Errorf("unknown start state %q", value)
}

// testCondition resolves the cell for a researcher walkthrough. An explicit
// request wins so long as it names a real condition; otherwise any cell will do,
// since the row is excluded from the tally either way.
func (app *App) testCondition(requested string) (string, error) {
	if len(app.conditions) == 0 {
		return "", fmt.Errorf("no conditions configured")
	}
	if requested == "" {
		return app.conditions[rand.Intn(len(app.conditions))], nil
	}
	if _, known := app.data.Conditions[requested]; !known {
		return "", fmt.Errorf("unknown condition %q", requested)
	}
	return requested, nil
}

func (app *App) randomCondition() (string, error) {
	if len(app.conditions) == 0 {
		return "1-1", nil
	}
	tally, err := app.tallyConditions()
	if err != nil {
		return "", err
	}
	return app.selectCondition(tally)
}

func (app *App) selectCondition(tally conditionTally) (string, error) {
	if len(app.conditions) == 0 {
		return "", fmt.Errorf("no conditions configured")
	}

	minStarted := -1
	candidates := make([]string, 0, len(app.conditions))
	for _, condition := range app.conditions {
		// Skip cells that already have their full complement of completions.
		if tally.completed[condition] >= targetUsersPerCondition {
			continue
		}
		count := tally.started[condition]
		switch {
		case minStarted == -1 || count < minStarted:
			minStarted = count
			candidates = []string{condition}
		case count == minStarted:
			candidates = append(candidates, condition)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("condition allocation is full")
	}
	return candidates[rand.Intn(len(candidates))], nil
}

func (app *App) buildStageResponse(session *Session) StageConfig {
	response := StageConfig{
		CurrentState: string(session.CurrentState),
		Condition:    session.Condition,
	}

	switch SessionState(session.CurrentState) {
	case StateOnboarding:
		response.Type = "onboarding"
		response.Title = "Welcome to the Study"
	case StatePreSurvey:
		response.Type = "survey"
		response.Title = "Pre-interaction Survey"
	case StateInteraction:
		response.Type = "chat"
		response.Title = "Peer Support Group Chat"
		if conditionData, ok := app.data.Conditions[session.Condition]; ok {
			response.AllScripts = conditionData.Stages
		}
	case StatePostSurvey:
		response.Type = "survey"
		response.Title = "Post-interaction Survey"
	case StateComplete:
		response.Type = "complete"
		response.Title = "Session Complete"
		// The real code for this study, so a participant is credited even if the
		// environment variable is missing or stale. The completion screen builds
		// https://app.prolific.com/submissions/complete?cc=<this> from it. The
		// previous default was the placeholder "DEFAULT_CODE", which would have
		// failed silently at the one point in the study where failure costs a
		// participant their payment.
		response.CompletionCode = getEnvOrDefault("PROLIFIC_COMPLETION_CODE", "CT7D4WEX")
	default:
		response.Type = "unknown"
		response.Title = "Unknown Stage"
	}
	return response
}

func initDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS sessions (
            user_id TEXT PRIMARY KEY,
            condition TEXT NOT NULL,
            current_state TEXT NOT NULL,
            pre_survey_data TEXT,
            chat_transcript TEXT,
            stage_1_comfort_score INTEGER,
            stage_2_comfort_score INTEGER,
            post_survey_data TEXT,
            created_at TEXT NOT NULL
        )
    `)
	if err != nil {
		return nil, err
	}
	// Added later than the table, so existing volumes need the ALTER. Without a
	// second timestamp there is no way to say how long a session took, since
	// created_at is written the instant the page loads. Rows that predate the
	// column keep a NULL and are simply left out of the duration figures.
	// Added after the table existed, so an existing volume needs the ALTER. Each
	// is idempotent, since the service reruns this on every restart.
	migrations := []string{
		// Without a second timestamp there is no way to say how long a session
		// took, since created_at is written the instant the page loads.
		`ALTER TABLE sessions ADD COLUMN updated_at TEXT`,
		// The stage 3 comfort check-in, added 2026-08-03. Rows collected before
		// then have no Late-stage reading at all, not a zero.
		`ALTER TABLE sessions ADD COLUMN stage_3_comfort_score INTEGER`,
	}
	for _, statement := range migrations {
		if _, err := db.Exec(statement); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return nil, err
		}
	}
	return db, nil
}

func saveSession(db *sql.DB, session *Session) error {
	chatTranscript, _ := json.Marshal(session.ChatTranscript)
	preSurveyData := string(session.PreSurveyData)
	postSurveyData := string(session.PostSurveyData)
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.Exec(`
        INSERT INTO sessions (user_id, condition, current_state, pre_survey_data, chat_transcript, stage_1_comfort_score, stage_2_comfort_score, stage_3_comfort_score, post_survey_data, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(user_id) DO UPDATE SET
            condition=excluded.condition,
            current_state=excluded.current_state,
            pre_survey_data=excluded.pre_survey_data,
            chat_transcript=excluded.chat_transcript,
            stage_1_comfort_score=excluded.stage_1_comfort_score,
            stage_2_comfort_score=excluded.stage_2_comfort_score,
            stage_3_comfort_score=excluded.stage_3_comfort_score,
            post_survey_data=excluded.post_survey_data,
            created_at=excluded.created_at,
            updated_at=excluded.updated_at
    `, session.UserID, session.Condition, session.CurrentState, preSurveyData, chatTranscript, session.Stage1ComfortScore, session.Stage2ComfortScore, session.Stage3ComfortScore, postSurveyData, session.CreatedAt.Format(time.RFC3339Nano), updatedAt)
	return err
}

func loadSession(db *sql.DB, userID string) (*Session, error) {
	var session Session
	var preSurveyData, chatTranscript, postSurveyData, createdAt string
	var stage1Score, stage2Score, stage3Score sql.NullInt64
	err := db.QueryRow(`
        SELECT user_id, condition, current_state, pre_survey_data, chat_transcript, stage_1_comfort_score, stage_2_comfort_score, stage_3_comfort_score, post_survey_data, created_at
        FROM sessions WHERE user_id = ?
    `, userID).Scan(&session.UserID, &session.Condition, &session.CurrentState, &preSurveyData, &chatTranscript, &stage1Score, &stage2Score, &stage3Score, &postSurveyData, &createdAt)
	if err != nil {
		return nil, err
	}
	if preSurveyData != "" {
		session.PreSurveyData = []byte(preSurveyData)
	}
	if chatTranscript != "" {
		json.Unmarshal([]byte(chatTranscript), &session.ChatTranscript)
	}
	if postSurveyData != "" {
		session.PostSurveyData = []byte(postSurveyData)
	}
	if stage1Score.Valid {
		value := int(stage1Score.Int64)
		session.Stage1ComfortScore = &value
	}
	if stage2Score.Valid {
		value := int(stage2Score.Int64)
		session.Stage2ComfortScore = &value
	}
	if stage3Score.Valid {
		value := int(stage3Score.Int64)
		session.Stage3ComfortScore = &value
	}
	if createdAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, createdAt)
		if err == nil {
			session.CreatedAt = parsed
		}
	}
	return &session, nil
}

func writeJSON(w http.ResponseWriter, value any, logger *slog.Logger) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		logger.Error("failed to encode json response", "error", err)
	}
}

// isTruthy accepts the same spellings the frontend does, so ?test=1 and
// ?test=yes behave like ?test=true on both sides.
func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func getEnvOrDefault(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func sanitizeSurveyData(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	cleanData := make(map[string]any, len(data))
	for key, value := range data {
		cleanData[key] = value
	}
	return cleanData
}
