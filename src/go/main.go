package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SessionState string

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
	Type         string      `json:"type"`
	CurrentState string      `json:"currentState"`
	Condition    string      `json:"condition"`
	Title        string      `json:"title"`
	BotScripts   []BotScript `json:"botScripts,omitempty"`
}

type experimentData struct {
	Conditions map[string]struct {
		Label  string `json:"label"`
		Stages map[string][]BotScript `json:"stages"`
	} `json:"conditions"`
}

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

var appData experimentData

func main() {
	rand.Seed(time.Now().UnixNano())

	root, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	dataPath := filepath.Join(root, "data.json")
	content, err := os.ReadFile(dataPath)
	if err != nil {
		log.Fatalf("read data.json: %v", err)
	}
	if err := json.Unmarshal(content, &appData); err != nil {
		log.Fatalf("parse data.json: %v", err)
	}

	dbPath := filepath.Join(root, "sessions.db")
	db, err := initDB(dbPath)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/session/init", func(w http.ResponseWriter, r *http.Request) {
		createSessionHandler(w, r, db)
	})
	mux.HandleFunc("/api/stage", func(w http.ResponseWriter, r *http.Request) {
		stageHandler(w, r, db)
	})
	mux.HandleFunc("/api/submit", func(w http.ResponseWriter, r *http.Request) {
		submitHandler(w, r, db)
	})

	log.Println("research prototype backend listening on http://localhost:8080")
	if err := http.ListenAndServe(":8080", withCORS(mux)); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func createSessionHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	condition := randomCondition()
	session := &Session{
		UserID:       fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		Condition:    condition,
		CurrentState: StatePreSurvey,
		CreatedAt:    time.Now().UTC(),
	}
	if err := saveSession(db, session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, session)
}

func stageHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	if sessionID == "" {
		http.Error(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	session, err := loadSession(db, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	response := buildStageResponse(session)
	writeJSON(w, response)
}

func submitHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var payload struct {
		SessionID      string          `json:"sessionId"`
		CurrentState   string          `json:"currentState"`
		PreSurveyData  map[string]any  `json:"preSurveyData"`
		UserMessage    map[string]any  `json:"userMessage"`
		Score          *int            `json:"score"`
		PostSurveyData map[string]any  `json:"postSurveyData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	session, err := loadSession(db, payload.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	switch SessionState(payload.CurrentState) {
	case StatePreSurvey:
		if payload.PreSurveyData != nil {
			if reversed := reverseAIAS(payload.PreSurveyData); reversed != nil {
				payload.PreSurveyData = reversed
			}
			data, _ := json.Marshal(payload.PreSurveyData)
			session.PreSurveyData = data
		}
		session.CurrentState = StateChatStage1
	case StateChatStage1:
		if payload.UserMessage != nil {
			session.ChatTranscript = append(session.ChatTranscript, buildChatMessage(payload.UserMessage, session.CurrentState))
		}
		session.CurrentState = StateAssessment1
	case StateAssessment1:
		if payload.Score != nil {
			session.Stage1ComfortScore = payload.Score
		}
		session.CurrentState = StateChatStage2
	case StateChatStage2:
		if payload.UserMessage != nil {
			session.ChatTranscript = append(session.ChatTranscript, buildChatMessage(payload.UserMessage, session.CurrentState))
		}
		session.CurrentState = StateAssessment2
	case StateAssessment2:
		if payload.Score != nil {
			session.Stage2ComfortScore = payload.Score
		}
		session.CurrentState = StateChatStage3
	case StateChatStage3:
		if payload.UserMessage != nil {
			session.ChatTranscript = append(session.ChatTranscript, buildChatMessage(payload.UserMessage, session.CurrentState))
		}
		session.CurrentState = StatePostSurvey
	case StatePostSurvey:
		if payload.PostSurveyData != nil {
			data, _ := json.Marshal(payload.PostSurveyData)
			session.PostSurveyData = data
		}
		session.CurrentState = StateComplete
	}

	if err := saveSession(db, session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"sessionId": session.UserID,
		"currentState": session.CurrentState,
		"nextStage": buildStageResponse(session),
	})
}

func buildStageResponse(session *Session) StageConfig {
	response := StageConfig{
		Type:        "survey",
		CurrentState: string(session.CurrentState),
		Condition:   session.Condition,
		Title:       "Pre-interaction survey",
	}

	switch SessionState(session.CurrentState) {
	case StatePreSurvey:
		response.Type = "survey"
		response.Title = "Pre-interaction survey"
	case StateChatStage1, StateChatStage2, StateChatStage3:
		response.Type = "chat"
		response.Title = stageTitle(session.CurrentState)
		if conditionData, ok := appData.Conditions[session.Condition]; ok {
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
	}
	return response
}

func assessmentTitle(state SessionState) string {
	switch state {
	case StateAssessment1:
		return "Between-stage assessment"
	case StateAssessment2:
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

func randomCondition() string {
	options := []string{"1-1", "2-1", "3-1"}
	return options[rand.Intn(len(options))]
}

func buildChatMessage(raw map[string]any, stage SessionState) ChatMessage {
	text := ""
	if v, ok := raw["text"].(string); ok {
		text = v
	}
	return ChatMessage{
		Sender:    "User",
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

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
