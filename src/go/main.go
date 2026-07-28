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
	StatePreSurvey   SessionState = "STATE_PRE_SURVEY"
	StateChatStage1  SessionState = "STATE_CHAT_STAGE_1"
	StateAssessment1 SessionState = "STATE_ASSESSMENT_1"
	StateChatStage2  SessionState = "STATE_CHAT_STAGE_2"
	StateAssessment2 SessionState = "STATE_ASSESSMENT_2"
	StateChatStage3  SessionState = "STATE_CHAT_STAGE_3"
	StatePostSurvey  SessionState = "STATE_POST_SURVEY"
	StateComplete    SessionState = "STATE_COMPLETE"
)

type ChatMessage struct {
	Sender    string    `json:"sender"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	Stage     string    `json:"stage"`
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

type StageConfig struct {
	Type           string      `json:"type"`
	CurrentState   string      `json:"currentState"`
	Condition      string      `json:"condition"`
	Title          string      `json:"title"`
	BotScripts     []BotScript `json:"botScripts,omitempty"`
	CompletionCode string      `json:"completionCode,omitempty"`
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

	rand.Seed(time.Now().UnixNano())

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
		CurrentState: StatePreSurvey,
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
		ProlificID         string         `json:"prolificId"` // From Onboarding
		PreSurveyData      map[string]any `json:"preSurveyData"`
		ChatTranscript     []any          `json:"chatTranscript"` // Array of chat messages
		Stage1ComfortScore int            `json:"stage1ComfortScore"`
		Stage2ComfortScore int            `json:"stage2ComfortScore"`
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
			data, _ := json.Marshal(initialData)
			session.PreSurveyData = data
		}
		session.CurrentState = "STATE_PRE_SURVEY"

	case "STATE_PRE_SURVEY": // Or your StatePreSurvey constant
		if payload.PreSurveyData != nil {
			// Reverse AIAS "Risks" factor. (Items 10, 11, 12, 13)
			reverseScoreMap(payload.PreSurveyData, "AIAS", []int{10, 11, 12, 13}, 5.0)
			
			// Extract and preserve the existing prolific_id 
			var existingData map[string]any
			if len(session.PreSurveyData) > 0 {
				json.Unmarshal(session.PreSurveyData, &existingData)
				if id, ok := existingData["prolific_id"]; ok {
					payload.PreSurveyData["prolific_id"] = id
				}
			}

			data, _ := json.Marshal(payload.PreSurveyData)
			session.PreSurveyData = data
		}
		session.CurrentState = "STATE_INTERACTION"

	case "STATE_INTERACTION":
		// Handle the full chat transcript dump from the frontend
		if payload.ChatTranscript != nil {
			data, _ := json.Marshal(payload.ChatTranscript)
			session.ChatTranscript = data // Assuming ChatTranscript is []byte in your Session struct
		}
		// Save the mid-chat assessment scores
		session.Stage1ComfortScore = payload.Stage1ComfortScore
		session.Stage2ComfortScore = payload.Stage2ComfortScore
		
		session.CurrentState = "STATE_POST_SURVEY"

	case "STATE_POST_SURVEY":
		if payload.PostSurveyData != nil {
			// Reverse BFNE items 2, 4, 7, and 10
			reverseScoreMap(payload.PostSurveyData, "BFNE", []int{2, 4, 7, 10}, 5.0)

			data, _ := json.Marshal(payload.PostSurveyData)
			session.PostSurveyData = data
		}
		session.CurrentState = "STATE_COMPLETE"
	}

	// 3. Save updated session to SQLite
	if err := saveSession(app.db, session); err != nil {
		app.logger.Error("failed to save session", "error", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// 4. Build and return the next stage configuration (Injects the Prolific Code if complete)
	nextStage := buildStageResponse(session) // Using your existing response builder

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sessionId":    session.ID,
		"currentState": session.CurrentState,
		"nextStage":    nextStage,
	})
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
		Type:         "survey",
		CurrentState: string(session.CurrentState),
		Condition:    session.Condition,
		Title:        "Pre-interaction survey",
	}

	switch SessionState(session.CurrentState) {
	case StatePreSurvey:
		response.Type = "survey"
		response.Title = "Pre-interaction survey"
	case StateChatStage1, StateChatStage2, StateChatStage3:
		response.Type = "chat"
		response.Title = stageTitle(session.CurrentState)
		if conditionData, ok := app.data.Conditions[session.Condition]; ok {
			if scripts, ok := conditionData.Stages[string(session.CurrentState)]; ok {
				response.BotScripts = scripts
			}
		}
	case StateAssessment1, StateAssessment2:
		response.Type = "assessment"
		response.Title = assessmentTitle(session.CurrentState)
	case StatePostSurvey:
		response.Type = "survey"
		response.Title = "Post-interaction survey"
	case StateComplete:
		response.Type = "complete"
		response.Title = "Session complete"
		response.CompletionCode = getEnvOrDefault("PROLIFIC_COMPLETION_CODE", "DEFAULT_CODE") 
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

func buildChatMessage(raw map[string]any, stage SessionState) ChatMessage {
	text := ""
	if v, ok := raw["text"].(string); ok {
		text = v
	}
	return ChatMessage{
		Sender:    "You",
		Text:      text,
		Timestamp: time.Now().UTC(),
		Stage:     string(stage),
	}
}

func reverseAIAS(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	copyData := make(map[string]any, len(raw))
	for k, v := range raw {
		copyData[k] = v
	}
	if aias, ok := copyData["AIAS"].(map[string]any); ok {
		reverseItems := []string{"item4", "item5", "item6", "item7"}
		for _, item := range reverseItems {
			if val, ok := aias[item].(float64); ok {
				aias[item] = 6 - val
			}
		}
	}
	return copyData
}

func assessmentTitle(state SessionState) string {
	switch state {
	case StateAssessment1, StateAssessment2:
		return "Between-stage assessment"
	default:
		return "Assessment"
	}
}

func stageTitle(state SessionState) string {
	switch state {
	case StateChatStage1:
		return "Stage 1 – Early baseline"
	case StateChatStage2:
		return "Stage 2 – Mid vulnerability"
	case StateChatStage3:
		return "Stage 3 – Late vulnerability"
	default:
		return "Chat"
	}
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