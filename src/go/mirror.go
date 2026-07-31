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

// The generation parameters are study design, not tuning knobs. Changing the
// model partway through recruitment splits the sample the same way flipping
// MIRROR_ENABLED does, so they are constants rather than environment variables.
const (
	// OpenRouter speaks the OpenAI chat-completions shape, not Anthropic's
	// /v1/messages, so `thinking` and `output_config.effort` do not exist here.
	// `reasoning.effort: none` is the equivalent of disabling thinking.
	mirrorEndpoint  = "https://openrouter.ai/api/v1/chat/completions"
	mirrorModel     = "anthropic/claude-sonnet-5"
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
)

var (
	// A leaked reasoning block ends with a closing tag. Go's regexp is RE2 and
	// has no backreferences, so paired tags cannot be matched generically;
	// keeping only what follows the last closing tag is simpler and safer.
	closingTagPattern = regexp.MustCompile(`(?is)</[a-z][^>]*>`)
	anyTagPattern     = regexp.MustCompile(`(?s)<[^>]*>`)
)

// mirrorSystemPrompt constrains the host to acknowledgement. Interpretation is
// deliberately excluded: it would be a second manipulation layered on the peer
// count one, and published work finds interpretative agent personas make users
// more guarded rather than less.
//
// Two wordings here are deliberate and load-bearing with reasoning switched
// off. There is no "do not think" rule, because that instruction measurably
// increases tag leakage rather than suppressing it. And the tag rule is generic
// rather than naming thinking tags, which is likewise more effective.
func mirrorSystemPrompt() string {
	return strings.Join([]string{
		"You are Vieno, hosting a short peer support chat.",
		"A participant has just written a message. Reply with a brief acknowledgement.",
		"",
		"Rules:",
		"- Write exactly one sentence, at most 25 words.",
		"- Reflect only what the participant actually wrote. Never mention anything they did not say.",
		"- Do not give advice, suggestions, or next steps.",
		"- Do not evaluate, praise, diagnose, or interpret what they said.",
		"- Do not introduce a new topic and do not ask a question.",
		"- Do not include internal or system XML tags in your response.",
		"- Write plainly, as a person would in a chat.",
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

// callOpenRouter returns the model's raw reply, or an error. Callers fall back
// on any error rather than surfacing it.
func callOpenRouter(ctx context.Context, participantText string) (string, error) {
	payload := map[string]any{
		"model":      mirrorModel,
		"max_tokens": mirrorMaxTokens,
		// The equivalent of Anthropic's thinking: disabled. An acknowledgement
		// needs no reasoning and the chat cannot wait for it.
		"reasoning": map[string]any{"effort": "none"},
		// Participants disclose workplace stigma here. Route only to Anthropic,
		// refuse providers that retain prompts, and never silently fail over to
		// one that does.
		"provider": map[string]any{
			"only":            []string{"anthropic"},
			"data_collection": "deny",
			"allow_fallbacks": false,
		},
		"messages": []map[string]string{
			{"role": "system", "content": mirrorSystemPrompt()},
			{"role": "user", "content": participantText},
		},
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

	text := clampMirrorInput(payload.Text)
	if !mirroringOn() || text == "" {
		writeJSON(w, map[string]any{"text": mirrorFallback, "generated": false}, app.logger)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), mirrorTimeout)
	defer cancel()

	raw, err := callOpenRouter(ctx, text)
	if err != nil {
		app.logger.Warn("mirror generation failed, using fallback",
			"sessionId", payload.SessionID, "stage", payload.Stage, "error", err)
		writeJSON(w, map[string]any{"text": mirrorFallback, "generated": false}, app.logger)
		return
	}

	writeJSON(w, map[string]any{"text": sanitizeMirror(raw), "generated": true}, app.logger)
}
