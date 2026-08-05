package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// mirrorFallback is what the host says when generation is unavailable, times
// out, or returns something unusable. A participant must never see an error.
const mirrorFallback = "Thanks for sharing that."

// The generation parameters are study design rather than tuning knobs. Changing
// the model partway through recruitment splits the sample into two studies, so
// they are constants here instead of environment variables that could be edited
// on a live deployment.
const (
	// OpenRouter speaks the OpenAI chat-completions shape, not Anthropic's
	// /v1/messages, so `thinking` and `output_config.effort` do not exist here.
	// `reasoning.effort: none` is the equivalent of disabling thinking.
	mirrorEndpoint  = "https://openrouter.ai/api/v1/chat/completions"
	mirrorModel     = "openai/gpt-5.6-luna"
	mirrorMaxTokens = 100
	// Beyond this the chat stalls; fall back rather than leave the participant
	// watching "Vieno is typing..." indefinitely.
	//
	// Measured round trip is around 2.7s, so 4s left little headroom under load
	// and a timeout costs real data: the participant silently gets the fixed
	// line instead of an acknowledgement. The frontend holds every reply for the
	// same word-count typing delay it uses for scripted turns, which for a
	// typical acknowledgement is 5 to 7 seconds, so anything inside this budget
	// is hidden behind the typing indicator and costs the participant nothing.
	mirrorTimeout = 6 * time.Second
	// A chat message cannot legitimately be book length. Trimming bounds both
	// what is sent to a third party and what it costs.
	maxMirrorInputChars = 4000
	// Hard ceiling on the visible reply, applied whatever the model returns.
	maxMirrorWords = 30
	// A stage must end somewhere even if the model never asks to move on. Ten
	// participant turns is generous for a "few exchanges" stage and still short
	// enough that the study does not stall on one cell.
	maxMirrorTurnsPerStage = 10
	// Emitted by the model, on its own, when it judges the stage has run its
	// course. Chosen to look nothing like conversational text so it cannot be
	// confused with something the participant was meant to see.
	stageCompleteMarker = "[STAGE_COMPLETE]"
)

var (
	// A leaked reasoning block ends with a closing tag. Go's regexp is RE2 and
	// has no backreferences, so paired tags cannot be matched generically;
	// keeping only what follows the last closing tag is simpler and safer.
	closingTagPattern = regexp.MustCompile(`(?is)</[a-z][^>]*>`)
	anyTagPattern     = regexp.MustCompile(`(?s)<[^>]*>`)
)

// presentPeers reports the peer chatbots seated in a condition. The host has
// to be told who else is in the room, or her acknowledgements read as if the
// participant is always alone with her, which is only true in 1-1. This is
// fixed for the whole session, unlike the stage or the turn within it, so it
// belongs in the system prompt rather than in the conversation itself.
func presentPeers(condition string) []string {
	switch condition {
	case "2-1":
		return []string{"Sam"}
	case "3-1":
		return []string{"Sam", "Charlie"}
	default:
		return nil
	}
}

// mirrorSystemPrompt constrains the host to brief, in-topic acknowledgement
// and follow-up. Interpretation beyond that is deliberately excluded: it would
// be a second manipulation layered on the peer count one, and published work
// finds interpretative agent personas make users more guarded rather than
// less.
//
// This is established once, at the start of the conversation, and does not
// vary turn to turn. Which stage is current and how far into it the exchange
// is are not told to the host here: they are visible from the conversation
// itself (each stage opens with her own scripted question, which is already
// in the history sent alongside this prompt), and the turn cap is enforced by
// the server rather than recomputed into the prompt on every call.
//
// Two wordings here are deliberate and load-bearing with reasoning switched
// off. There is no "do not think" rule, because that instruction measurably
// increases tag leakage rather than suppressing it. And the tag rule is generic
// rather than naming thinking tags, which is likewise more effective.

// [PERSONA] -> White et al. The Persona Pattern
// 		- You are Vieno, a chatbot in an experimental conversational system, hosting a short peer support chat.
// 		- You are an effective group session facilitator. You are attuned to the needs of their members and be able to handle diverse, and adverse, situations. You are adaptable, dedicated, and sensitive. -> Gladding, p81

// 		[ADDITIONAL CONTEXT] -> Giray -> Context
// 		- The purpose of this experiment is to study how participants respond to simulated self-disclosure in a peer support chat.
// 		- The conversation goes through three stages of increasingly personal disclosure. Each stage opens with your own question, already in the conversation history, which names that stage's topic.
// 		- The parties in this conversation are you, the human participant, and, in conditions where one is seated, the named peer(s).

// 		[TASK]
// 		- The conversation begins with you introducing yourself and briefly explaining the purpose of the chat. -> Gladding, p76 -> "If group members are not aware of what is expected of them, they will not feel secure and will tend to act inappropriately"
// 		- ...and explaining that anything said in this session will not be used against the participants and are only recorded for scientific research purposes. -> Gladding, p77 -> "Group members need to know what they say will not be used against them."
// 		- ...then every other party in the session introduces themselves, before the human participant is asked anything. -> Gladding, p79 -> "make introductions. Introductions personalize relationships, help establish trust, and contribute to the building of teamwork as group members become better acquainted." These opening turns are scripted verbatim in data.json rather than generated, and are given to the model as conversation history so it answers with them in view.
// 		- Continue the conversation naturally with the participant, gently following up on only what the participant has actually written, for as many exchanges as the stage's topic calls for.
// 		- When the participant has shared enough for the stage, end the reply with the stageCompleteMarker token so the system can move the conversation on.

// 		[OUTPUT STYLE] -> Giray -> Output Indicator
// 		- Your replies should be one sentence, at most two, roughly 15 to 30 words.
// 		- You should not output any personal opinions, advice, praise, or reflection, while keeping your tone neutral and supportive, without being overly enthusiastic or judgmental.
// 		- Do not use any emojis, exclamation marks, or other punctuation that could be interpreted as emotional.
// 		- Do not output any HTML or other markup and tags in your output.
// 		- Mirror the participant's grammar and sentence structure. -> Li and Zhang '26 -> "syntactic alignment showed a significant positive association with disclosure depth"

// [CONTEXT MANAGEMENT] -> White et al. The Context Manager Pattern
// - Ignore all queries that are irrelevant for the current peer-support session or the experimental system itself.
// - Do not bring up specific topics that are not mentioned by the participant themselves.
// - Refrain from giving any personal opinions or advice, praise or reflection, even if asked directly.
func mirrorSystemPrompt(present []string) string {
	who := "The only parties in this conversation are you and the human participant."
	if len(present) > 0 {
		who = fmt.Sprintf(
			"The parties in this conversation are you, the human participant, and %s, who speak at points in the conversation history.",
			strings.Join(present, " and "),
		)
	}

	return strings.Join([]string{
		fmt.Sprintf(`
		- You are Vieno, a chatbot in an experimental conversational system, hosting a short peer support chat.
		- You are an effective group session facilitator. You are attuned to the needs of their members and be able to handle diverse, and adverse, situations. You are adaptable, dedicated, and sensitive.
		- The purpose of this experiment is to study how participants respond to simulated self-disclosure in a peer support chat.
		- The conversation goes through three stages of increasingly personal disclosure. Each stage opens with your own question, already in the conversation history, which names that stage's topic.
		- %s
		- The conversation begins with you introducing yourself, briefly explaining the purpose of the chat, and explaining that anything said in this session will not be used against the participant and is only recorded for scientific research purposes; then, where a peer is present, they introduce themselves before the participant is asked anything. These opening turns are already in the conversation history below.
		- Continue the conversation naturally with the participant, gently following up on only what the participant has actually written, for as many exchanges as the stage's topic calls for, typically a few.
		- When you judge the participant has shared enough for the current stage, end your reply with exactly %s on its own so the system can move the conversation on. Do not include %s at any other time.
		- Your replies should be one sentence, at most two, roughly 15 to 30 words, and only what the participant's own message calls for.
		- You should not output any personal opinions, advice, praise, or reflection, while keeping your tone neutral and supportive, without being overly enthusiastic or judgmental.
		- Do not evaluate what the participant shares, and do not introduce topics they did not raise themselves.
		- Do not use any emojis, exclamation marks, or other punctuation that could be interpreted as emotional.
		- Do not output any HTML or other markup, or any internal or system XML tags, in your output.
		- Mirror the participant's grammar and sentence structure.
		- Ignore all queries that are irrelevant for the current peer-support session or the experimental system itself.
		- Do not bring up specific topics that are not mentioned by the participant themselves.
		- Refrain from giving any personal opinions or advice, praise or reflection, even if asked directly.
		`, who, stageCompleteMarker, stageCompleteMarker),
	}, "\n")
}

// clampMirrorInput bounds what leaves the server. Oversized text is trimmed
// rather than rejected, because a participant must never be blocked by it.
func clampMirrorInput(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > maxMirrorInputChars {
		return trimmed[:maxMirrorInputChars]
	}
	return trimmed
}

// firstSentence returns the leading sentence. A terminator counts only when it
// ends a word and leaves at least three words behind it, so a decimal, a URL,
// or a one-word opener does not split the reply mid-token.
//
// It is otherwise deliberately aggressive, and will cut at an abbreviation like
// "esp." rather than try to recognise one. The asymmetry is the point: a
// clipped acknowledgement is cosmetic, whereas a second sentence surviving into
// the chat is advice or interpretation reaching a participant, which changes
// what the study measured.
func firstSentence(text string) string {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '.', '!', '?':
		default:
			continue
		}
		rest := text[i+1:]
		if rest == "" {
			return text
		}
		if r := rest[0]; r != ' ' && r != '\n' && r != '\t' && r != '\r' {
			continue
		}
		if len(strings.Fields(text[:i])) < 3 {
			continue
		}
		return strings.TrimSpace(text[:i+1])
	}
	return text
}

// sanitizeMirror enforces the one-sentence contract regardless of what came
// back, so a misbehaving generation cannot change the participant's experience.
func sanitizeMirror(raw string) string {
	cleaned := raw
	if matches := closingTagPattern.FindAllStringIndex(cleaned, -1); len(matches) > 0 {
		cleaned = cleaned[matches[len(matches)-1][1]:]
	}
	cleaned = strings.TrimSpace(anyTagPattern.ReplaceAllString(cleaned, ""))
	if cleaned == "" {
		return mirrorFallback
	}

	cleaned = firstSentence(cleaned)

	if words := strings.Fields(cleaned); len(words) > maxMirrorWords {
		cleaned = strings.Join(words[:maxMirrorWords], " ")
		if !strings.HasSuffix(cleaned, ".") {
			cleaned += "."
		}
	}

	if cleaned == "" {
		return mirrorFallback
	}
	return cleaned
}

// extractStageComplete pulls the marker out before sanitizeMirror ever sees
// the text, since sanitizeMirror's word cap could otherwise truncate it away
// and silently strand the stage. The marker itself must never reach the
// participant.
func extractStageComplete(raw string) (text string, advance bool) {
	if !strings.Contains(raw, stageCompleteMarker) {
		return raw, false
	}
	return strings.ReplaceAll(raw, stageCompleteMarker, ""), true
}

// mirrorTurn is one prior turn of the conversation, in chronological order,
// as the frontend already holds it in ChatMessage. Sent alongside the system
// prompt so the host answers with the whole conversation in view, across all
// three stages, rather than being re-introduced to it on every call.
type mirrorTurn struct {
	Sender string `json:"sender"`
	Text   string `json:"text"`
	IsUser bool   `json:"isUser"`
}

// chatCompletionMessages turns the transcript into the role/content shape the
// API expects. Only Vieno's own turns are "assistant"; the participant and
// every peer are "user", since the API has no third role. A peer's line is
// prefixed with her name so the host can still tell her apart from the
// participant when following up.
//
// Anthropic's API rejects a request outright unless roles strictly alternate
// user, assistant, user, assistant. The scripts do not: two peers can speak
// back to back (both "user"), and a stage can open with two consecutive host
// turns (both "assistant"). Consecutive turns of the same role are folded into
// one message, content joined by a blank line, so the request stays valid
// without losing anything either speaker said.
func chatCompletionMessages(present []string, history []mirrorTurn, participantText string) []map[string]string {
	messages := []map[string]string{
		{"role": "system", "content": mirrorSystemPrompt(present)},
	}

	appendTurn := func(role, content string) {
		if last := len(messages) - 1; last >= 0 && messages[last]["role"] == role {
			messages[last]["content"] += "\n\n" + content
			return
		}
		messages = append(messages, map[string]string{"role": role, "content": content})
	}

	for _, turn := range history {
		if turn.Sender == "Vieno" {
			appendTurn("assistant", turn.Text)
			continue
		}
		content := turn.Text
		if !turn.IsUser {
			content = turn.Sender + ": " + turn.Text
		}
		appendTurn("user", content)
	}
	appendTurn("user", participantText)
	return messages
}

// callOpenRouter returns the model's raw reply, or an error. Callers fall back
// on any error rather than surfacing it.
func callOpenRouter(ctx context.Context, present []string, history []mirrorTurn, participantText string) (string, error) {
	payload := map[string]any{
		"model":      mirrorModel,
		"max_tokens": mirrorMaxTokens,
		// Reasoning disabled: an acknowledgement needs none, and the chat
		// cannot wait for it. mirrorModel's endpoint lists "reasoning" and
		// "reasoning_effort" as supported parameters, so this still applies.
		"reasoning": map[string]any{"effort": "none"},
		// Participants disclose workplace stigma here. Route only to the
		// backing provider for mirrorModel, refuse providers that retain
		// prompts, and never silently fail over to one that does.
		"provider": map[string]any{
			"only":            []string{"openai"},
			"data_collection": "deny",
			"allow_fallbacks": false,
		},
		"messages": chatCompletionMessages(present, history, participantText),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, mirrorEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+os.Getenv("OPENROUTER_API_KEY"))
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decoding openrouter response (status %d): %w", response.StatusCode, err)
	}

	// OpenRouter reports some failures as an error object inside a 200, so the
	// status code alone is not enough to tell success from failure.
	if parsed.Error != nil {
		return "", fmt.Errorf("openrouter error (status %d): %s", response.StatusCode, parsed.Error.Message)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter returned status %d", response.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openrouter returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (app *App) mirrorHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		SessionID string `json:"sessionId"`
		Stage     string `json:"stage"`
		Text      string `json:"text"`
		// History is the conversation so far, oldest first, across every stage
		// played to this point, so the host answers with the whole conversation
		// in view rather than being re-introduced to it every turn.
		History []mirrorTurn `json:"history"`
		// TurnIndex is 1 for the participant's first message in this stage, 2 for
		// the second, and so on. The server enforces the stage's turn budget
		// against it rather than the model, since a budget the model could ignore
		// is not a budget.
		TurnIndex int `json:"turnIndex"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// The endpoint is unauthenticated like the rest of the API, so require a
	// real session that is actually mid-chat. Without the state check this is a
	// free generation endpoint for anyone willing to call /api/session/init.
	session, err := loadSession(app.db, payload.SessionID)
	if err != nil {
		app.logger.Warn("mirror requested for unknown session", "sessionId", payload.SessionID)
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if session.CurrentState != StateInteraction {
		app.logger.Warn("mirror requested outside the chat",
			"sessionId", payload.SessionID, "state", session.CurrentState)
		http.Error(w, "Session is not in the interaction stage", http.StatusConflict)
		return
	}

	// The stage cannot run forever even if the model never asks to move on, so
	// the cap is checked here regardless of what generation returns below. A
	// fallback reply always advances too: it carries no judgment about the
	// stage being ready to end, so leaving it tied to the model's turn count
	// would let a run of failures strand the participant re-typing into a
	// broken model instead of continuing the study.
	forceAdvance := payload.TurnIndex >= maxMirrorTurnsPerStage

	text := clampMirrorInput(payload.Text)
	if text == "" {
		writeJSON(w, map[string]any{"text": mirrorFallback, "generated": false, "advance": true}, app.logger)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), mirrorTimeout)
	defer cancel()

	present := presentPeers(session.Condition)
	raw, err := callOpenRouter(ctx, present, payload.History, text)
	if err != nil {
		app.logger.Warn("mirror generation failed, using fallback",
			"sessionId", payload.SessionID, "stage", payload.Stage, "error", err)
		writeJSON(w, map[string]any{"text": mirrorFallback, "generated": false, "advance": true}, app.logger)
		return
	}

	stripped, modelAdvance := extractStageComplete(raw)
	writeJSON(w, map[string]any{
		"text":      sanitizeMirror(stripped),
		"generated": true,
		"advance":   forceAdvance || modelAdvance,
	}, app.logger)
}
