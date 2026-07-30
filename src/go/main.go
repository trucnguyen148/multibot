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

// Session states. These mirror the coarse states the frontend drives in
// src/index.tsx. The chat is one server-visible step (StateInteraction); the
// three chat stages run client-side inside that step.
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
}

type Session struct {
	UserID             string          `json:"user_id"`
	Condition          string          `json:"condition"`
	CurrentState       SessionState    `json:"current_state"`
	PreSurveyData      json.RawMessage `json:"pre_survey_data"`
	ChatTranscript     []ChatMessage   `json:"chat_transcript"`
	Stage1ComfortScore *int            `json:"stage_1_comfort_score"`
	Stage2ComfortScore *int            `json:"stage_2_comfort_score"`
	PostSurveyData     json.RawMessage `json:"post_survey_data"`
	CreatedAt          time.Time       `json:"created_at"`
}

type BotScript struct {
	Sender string `json:"sender"`
	Text   string `json:"text"`
}

// StageConfig is the shape src/index.tsx expects back from /api/stage and as
// the nextStage field of /api/submit. AllScripts carries every chat stage for
// the assigned condition at once, because the frontend plays them locally.
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

// App encapsulates our dependencies (Dependency Injection)
type App struct {
	db         *sql.DB
	data       experimentData
	conditions []string
	logger     *slog.Logger
}

// --- Main Entry Point ---

func main() {
	// 1. Initialize Structured Logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	root, err := os.Getwd()
	if err != nil {
		logger.Error("failed to get working directory", "error", err)
		os.Exit(1)
	}

	// 2. Load Environment Variables
	port := getEnvOrDefault("PORT", "8080")
	dataDir := getEnvOrDefault("DATA_DIR", root)
	allowedOrigin := getEnvOrDefault("ALLOWED_ORIGIN", "*") // Default to * for dev, restrict in Railway

	// 3. Load Script Data dynamically
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

	// Dynamically extract conditions (No magic strings)
	var conditions []string
	for k := range appData.Conditions {
		conditions = append(conditions, k)
	}

	// 4. Initialize Database
	dbPath := filepath.Join(dataDir, "sessions.db")
	db, err := initDB(dbPath)
	if err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 5. Construct the App instance
	app := &App{
		db:         db,
		data:       appData,
		conditions: conditions,
		logger:     logger,
	}

	// 6. Setup Router
	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.healthHandler)
	mux.HandleFunc("/api/session/init", app.createSessionHandler)
	mux.HandleFunc("/api/stage", app.stageHandler)
	mux.HandleFunc("/api/submit", app.submitHandler)
	mux.HandleFunc("/api/export", app.exportHandler)

	// 7. Configure Server with Graceful Shutdown
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: app.withCORS(mux, allowedOrigin),
	}

	// Start server in a goroutine
	go func() {
		app.logger.Info("server starting", "port", port, "env", "production")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.logger.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	app.logger.Info("shutting down server gracefully...")

	// Give outstanding requests a deadline to finish
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Handlers (Methods on App) ---

func (app *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (app *App) createSessionHandler(w http.ResponseWriter, r *http.Request) {
	condition := app.randomCondition()
	session := &Session{
		UserID:       fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		Condition:    condition,
		CurrentState: StateOnboarding,
		CreatedAt:    time.Now().UTC(),
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
	// 1. Define the complete payload struct to catch all possible incoming data
	var payload struct {
		SessionID          string         `json:"sessionId"`
		CurrentState       string         `json:"currentState"`
		ProlificID         string         `json:"prolificId"`  // From Onboarding
		DisplayName        string         `json:"displayName"` // Chat name chosen at onboarding
		PreSurveyData      map[string]any `json:"preSurveyData"`
		ChatTranscript     []ChatMessage  `json:"chatTranscript"`
		Stage1ComfortScore *int           `json:"stage1ComfortScore"`
		Stage2ComfortScore *int           `json:"stage2ComfortScore"`
		PostSurveyData     map[string]any `json:"postSurveyData"`
	}

	// Decode incoming JSON
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		app.logger.Error("invalid request body", "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Load existing session from SQLite
	session, err := loadSession(app.db, payload.SessionID)
	if err != nil {
		app.logger.Warn("session not found for submission", "sessionId", payload.SessionID)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// 2. Process data and advance the state machine
	switch payload.CurrentState {
	
	case "STATE_ONBOARDING":
		if payload.ProlificID != "" {
			initialData := map[string]any{
				"prolific_id": payload.ProlificID,
			}
			// Self-chosen chat name. Participants are told a nickname is fine,
			// but treat it as personal data since some will use a real name.
			if payload.DisplayName != "" {
				initialData["display_name"] = payload.DisplayName
			}
			data, _ := json.Marshal(initialData)
			session.PreSurveyData = data
		}
		session.CurrentState = StatePreSurvey

	case "STATE_PRE_SURVEY": // Or your StatePreSurvey constant
		if payload.PreSurveyData != nil {
			// Reverse AIAS "Risks" factor. (Items 10, 11, 12, 13)
			reverseScoreMap(payload.PreSurveyData, "AIAS", []int{10, 11, 12, 13}, 5.0)
			
			// Carry over the identifiers written at onboarding, which would
			// otherwise be lost when this payload replaces PreSurveyData.
			var existingData map[string]any
			if len(session.PreSurveyData) > 0 {
				json.Unmarshal(session.PreSurveyData, &existingData)
				for _, key := range []string{"prolific_id", "display_name"} {
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
		// Handle the full chat transcript dump from the frontend
		if payload.ChatTranscript != nil {
			session.ChatTranscript = payload.ChatTranscript
		}
		// Save the mid-chat assessment scores. These stay nil when the
		// between-stage assessments are disabled in chat-interface.tsx.
		session.Stage1ComfortScore = payload.Stage1ComfortScore
		session.Stage2ComfortScore = payload.Stage2ComfortScore

		session.CurrentState = StatePostSurvey

	case "STATE_POST_SURVEY":
		if payload.PostSurveyData != nil {
			// Reverse BFNE items 2, 4, 7, and 10
			reverseScoreMap(payload.PostSurveyData, "BFNE", []int{2, 4, 7, 10}, 5.0)

			data, _ := json.Marshal(payload.PostSurveyData)
			session.PostSurveyData = data
		}
		session.CurrentState = StateComplete

	default:
		app.logger.Warn("submit for unrecognized state",
			"sessionId", payload.SessionID, "state", payload.CurrentState)
	}

	// 3. Save updated session to SQLite
	if err := saveSession(app.db, session); err != nil {
		app.logger.Error("failed to save session", "error", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// 4. Build and return the next stage configuration (Injects the Prolific Code if complete)
	nextStage := app.buildStageResponse(session)

	writeJSON(w, map[string]any{
		"sessionId":    session.UserID,
		"currentState": session.CurrentState,
		"nextStage":    nextStage,
	}, app.logger)
}

func (app *App) exportHandler(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	expectedSecret := os.Getenv("EXPORT_SECRET")

	if expectedSecret == "" || secret != expectedSecret {
		app.logger.Warn("unauthorized export attempt", "ip", r.RemoteAddr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := app.db.Query(`SELECT user_id, condition, current_state, pre_survey_data, chat_transcript, stage_1_comfort_score, stage_2_comfort_score, post_survey_data, created_at FROM sessions`)
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
		var s1, s2 sql.NullInt64
		if err := rows.Scan(&s.UserID, &s.Condition, &s.CurrentState, &pre, &chat, &s1, &s2, &post, &created); err != nil {
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

func (app *App) randomCondition() string {
	if len(app.conditions) == 0 {
		return "1-1" // Fallback safety
	}
	return app.conditions[rand.Intn(len(app.conditions))]
}

func (app *App) buildStageResponse(session *Session) StageConfig {
	response := StageConfig{
		CurrentState: string(session.CurrentState),
		Condition:    session.Condition,
	}

	switch session.CurrentState {
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
		} else {
			app.logger.Error("unknown condition for session",
				"sessionId", session.UserID, "condition", session.Condition)
		}
	case StatePostSurvey:
		response.Type = "survey"
		response.Title = "Post-interaction Survey"
	case StateComplete:
		response.Type = "complete"
		response.Title = "Session Complete"
		response.CompletionCode = getEnvOrDefault("PROLIFIC_COMPLETION_CODE", "DEFAULT_CODE")
	default:
		response.Type = "unknown"
		response.Title = "Unknown Stage"
		app.logger.Error("unhandled session state",
			"sessionId", session.UserID, "state", session.CurrentState)
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
	return db, nil
}

func saveSession(db *sql.DB, session *Session) error {
	chatTranscript, _ := json.Marshal(session.ChatTranscript)
	preSurveyData := string(session.PreSurveyData)
	postSurveyData := string(session.PostSurveyData)
	_, err := db.Exec(`
		INSERT INTO sessions (user_id, condition, current_state, pre_survey_data, chat_transcript, stage_1_comfort_score, stage_2_comfort_score, post_survey_data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			condition=excluded.condition,
			current_state=excluded.current_state,
			pre_survey_data=excluded.pre_survey_data,
			chat_transcript=excluded.chat_transcript,
			stage_1_comfort_score=excluded.stage_1_comfort_score,
			stage_2_comfort_score=excluded.stage_2_comfort_score,
			post_survey_data=excluded.post_survey_data,
			created_at=excluded.created_at
	`, session.UserID, session.Condition, session.CurrentState, preSurveyData, chatTranscript, session.Stage1ComfortScore, session.Stage2ComfortScore, postSurveyData, session.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func loadSession(db *sql.DB, userID string) (*Session, error) {
	var session Session
	var preSurveyData, chatTranscript, postSurveyData, createdAt string
	var stage1Score, stage2Score sql.NullInt64
	err := db.QueryRow(`
		SELECT user_id, condition, current_state, pre_survey_data, chat_transcript, stage_1_comfort_score, stage_2_comfort_score, post_survey_data, created_at
		FROM sessions WHERE user_id = ?
	`, userID).Scan(&session.UserID, &session.Condition, &session.CurrentState, &preSurveyData, &chatTranscript, &stage1Score, &stage2Score, &postSurveyData, &createdAt)
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

func getEnvOrDefault(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// reverseScoreMap finds specific keys in the JSON map and applies standard Likert reverse-scoring.
// Formula used: (MaxScore + 1) - CurrentScore
func reverseScoreMap(data map[string]any, prefix string, keysToReverse []int, maxScore float64) {
	if data == nil {
		return
	}
	for _, keyNum := range keysToReverse {
		key := fmt.Sprintf("%s_%d", prefix, keyNum)
		// Go unmarshals JSON numbers as float64
		if val, ok := data[key].(float64); ok {
			data[key] = (maxScore + 1.0) - val
		}
	}
}