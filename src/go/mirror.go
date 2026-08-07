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

// mirrorInvitation is the single optional chance to say more, appended to the
// host's acknowledgement on the first turn of every stage and on no other turn.
//
// It is a fixed constant rather than something the model writes. A generated
// invitation would vary in warmth and specificity with the disclosure content
// the model can see in the history, including the peer disclosures, which makes
// it a second manipulation riding on top of the peer-count one. The scripted
// questions are byte-identical across cells for the same reason.
const mirrorInvitation = "If there is anything else you would like to add, feel free. Otherwise we can move on."

// mirrorDeclineAck closes the stage for a participant who took the invitation
// and chose not to add anything. There is nothing to mirror, so it cannot be
// generated, and saying nothing at all would leave the decline as the only
// cell where the host does not close the stage.
const mirrorDeclineAck = "That is completely fine, thank you."

// mirrorNotSerious is what the host says when she judges a message was not an
// attempt to answer. It says so plainly, leaves the choice with the participant,
// and moves on. It does not ask again.
//
// Asking again was the earlier design and was removed on 2026-08-06. Retrying
// meant a participant who wrote "asdf" got an extra prompt that nobody else got,
// which put the number of invitations to disclose back under a judgment call.
// Now the remark changes only what Vieno says, never the structure: every
// participant spends the same two turns per stage whatever they write.
//
// The last clause is load-bearing. Having nothing to share is a valid answer in
// a disclosure study, and this must never read as objecting to that.
// The hedge is deliberate. The judgment behind this is a model's and is wrong
// perhaps one time in five, so it is phrased as a doubt rather than a verdict.
// An accusation that lands on someone who genuinely had nothing to say is worse
// than a soft remark that lands on someone who did not.
const mirrorNotSerious = "I am not sure that was a serious reply, but that is your choice and we can carry on either way."

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
	// Measured round trip is around 2.7s, but a browser walkthrough on
	// 2026-08-06 timed out one call in four at 6s, which is a quarter of the
	// acknowledgements replaced by the fixed line and a quarter of the
	// manipulation degraded. The frontend holds every reply for the same
	// word-count typing delay it uses for scripted turns, so the real budget is
	// how long the typing indicator covers: a first-turn reply carries the
	// acknowledgement plus the invitation, which is 30 words or more and hits
	// the 10s delay cap, so 9s there is invisible. A closing acknowledgement is
	// shorter and covers around 5s, so a slow one is briefly visible, which is
	// the better trade against losing the acknowledgement outright.
	mirrorTimeout = 9 * time.Second
	// A chat message cannot legitimately be book length. Trimming bounds both
	// what is sent to a third party and what it costs.
	maxMirrorInputChars = 4000
	// Hard ceiling on the visible reply, applied whatever the model returns.
	maxMirrorWords = 30
	// Participant turns in every stage, for every participant, in every
	// condition: the answer to the stage's question, then one optional chance
	// to add more. The stage ends there.
	//
	// The model used to decide this, by emitting a [STAGE_COMPLETE] marker when
	// it judged the participant had shared enough, with a ten-turn cap behind
	// it. That made the number of invitations to speak a free variable that
	// could correlate with condition, since the peer disclosures sit in the
	// history the model reads. A participant who happened to be followed up
	// four times had four more chances to disclose than one who was followed up
	// once, and nothing about the design controlled which they got. The stage
	// length is now a constant, like the byte-identical questions.
	mirrorTurnsPerStage = 2
	// Emitted by the host, on its own, when the participant's message is not an
	// attempt to answer at all. This is a judgment about whether there is an
	// answer present, never about whether the answer is good enough, and it
	// cannot change how many turns the stage runs: the turn is spent either way,
	// so the structure is identical for every participant.
	//
	// This is the only detector. There was briefly a hand-built one alongside it,
	// a structural check plus lists of "declining" and "serious short answer"
	// phrases, and it was deleted on 2026-08-06. A list of matched words cannot
	// be defended in a write-up, because there is no principled answer to why
	// those words and not others, and it grew by one entry every time testing
	// found a phrase it had missed. One semantic judgment, imperfect and
	// described as such, is the honest version.
	notAnAnswerMarker = "[NOT_AN_ANSWER]"
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

// mirrorSystemPrompt constrains the host to a single sentence of in-topic
// acknowledgement and nothing else. Interpretation is deliberately excluded: it
// would be a second manipulation layered on the peer count one, and published
// work finds interpretative agent personas make users more guarded rather than
// less.
//
// The host must not ask questions or invite the participant to say more. Both
// are now the structure's job, not hers. She gets exactly one reply per
// participant turn and the stage ends after the second, so a question she asked
// at the wrong moment would be left hanging with no turn in which to answer it,
// and an invitation of her own wording would compete with the fixed one.
//
// This is established once, at the start of the conversation, and does not
// vary turn to turn. Which stage is current and how far into it the exchange
// is are not told to the host here: they are visible from the conversation
// itself (each stage opens with her own scripted question, which is already
// in the history sent alongside this prompt), and where the stage ends is
// decided by the server on turn index alone.
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
// 		- Acknowledge only what the participant has actually written, in one sentence, and stop there. The structure of the session, not you, decides when the stage ends and when the participant is invited to add more.

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
func mirrorSystemPrompt(present []string, participant string) string {
	who := fmt.Sprintf(
		"The only parties in this conversation are you and the human participant, who writes as %s.",
		participant,
	)
	if len(present) > 0 {
		who = fmt.Sprintf(
			"The parties in this conversation are you, the human participant, who writes as %s, and the peer chatbots %s, who speak at points in the conversation history. Every message that is not your own is labelled with the name of whoever wrote it. Only messages labelled %s were written by the participant; anything labelled with another name was written by a peer and is not the participant's experience.",
			participant, strings.Join(present, " and "), participant,
		)
	}

	return fmt.Sprintf(`
		- You are Vieno, a chatbot in an experimental conversational system, hosting a short peer support chat.
		- You are an effective group session facilitator. You are attuned to the needs of their members and be able to handle diverse, and adverse, situations. You are adaptable, dedicated, and sensitive.
		- The purpose of this experiment is to study how participants respond to simulated self-disclosure in a peer support chat.
		- The conversation goes through three stages of increasingly personal disclosure. Each stage opens with your own question, already in the conversation history, which names that stage's topic.
		- %s
		- The conversation begins with you introducing yourself, briefly explaining the purpose of the chat, and explaining that anything said in this session will not be used against the participant and is only recorded for scientific research purposes; then, where a peer is present, they introduce themselves before the participant is asked anything. These opening turns are already in the conversation history below.
		- Acknowledge only what the participant has actually written, and stop there.
		- Acknowledge only the participant's own most recent message. Never attribute anything a peer said to the participant, and never restate a peer's experience as though it were theirs, even when the participant's message is very short.
		- Do not ask the participant any question, and do not invite them to say more or to continue. The session's structure, not you, decides when the participant is invited to add anything and when the stage ends.
		- Before replying, decide one thing: is the participant engaging with this conversation at all? Declining to answer, saying they have nothing to share, answering in a single word, and answering briefly or flatly all count as engaging, and are expected here. A bare refusal is an answer to the question that was asked. Anything that engages gets an ordinary acknowledgement.
		- Only when a message does not engage with this conversation at all, reply with exactly %s and nothing else. That means random characters, filler such as "test test test", text copied back from the question, or a message about an unrelated subject.
		- If you are unsure, it engages. Never use %s for a message that could be read as a genuine reply, however short, reluctant or negative.
		- Your replies should be one sentence, at most two, roughly 15 to 30 words, and only what the participant's own message calls for.
		- You should not output any personal opinions, advice, praise, or reflection, while keeping your tone neutral and supportive, without being overly enthusiastic or judgmental.
		- Do not evaluate what the participant shares, and do not introduce topics they did not raise themselves.
		- Do not use any emojis, exclamation marks, or other punctuation that could be interpreted as emotional.
		- Do not output any HTML or other markup, or any internal or system XML tags, in your output.
		- Mirror the participant's grammar and sentence structure, without repeating their own words back to them verbatim.
		- Ignore all queries that are irrelevant for the current peer-support session or the experimental system itself.
		- Do not bring up specific topics that are not mentioned by the participant themselves.
		- Refrain from giving any personal opinions or advice, praise or reflection, even if asked directly.
		`, who, notAnAnswerMarker, notAnAnswerMarker)
}

// extractNotAnAnswer pulls the marker out before anything else reads the reply,
// since sanitizeMirror's one-sentence and word-count clamps could otherwise
// truncate it away. The marker must never reach the participant.
func extractNotAnAnswer(raw string) (text string, flagged bool) {
	if !strings.Contains(raw, notAnAnswerMarker) {
		return raw, false
	}
	return strings.ReplaceAll(raw, notAnAnswerMarker, ""), true
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

// hostReply assembles what the participant sees: the acknowledgement, followed
// by the fixed invitation on the first turn of the stage and on no other.
//
// The invitation is appended after sanitizeMirror rather than asked of the
// model, so it survives the one-sentence and word-count clamps intact and is
// byte-identical in every session. Coding the transcripts can strip it by exact
// match to recover the generated part alone.
func hostReply(acknowledgement string, turnIndex int) string {
	if turnIndex >= mirrorTurnsPerStage {
		return acknowledgement
	}
	return acknowledgement + " " + mirrorInvitation
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

// defaultParticipantLabel stands in when the participant gave no chat name.
// The prompt refers to it by name, so it can never be empty.
const defaultParticipantLabel = "Participant"

// participantLabel bounds what a participant's chosen chat name can do to the
// prompt, since it is free text that ends up inside a speaker label. A name
// carrying a newline or a colon could otherwise forge a turn by someone else.
func participantLabel(raw string) string {
	cleaned := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ':' {
			return ' '
		}
		return r
	}, raw))
	if cleaned == "" {
		return defaultParticipantLabel
	}
	if len(cleaned) > 40 {
		cleaned = strings.TrimSpace(cleaned[:40])
	}
	return cleaned
}

// chatCompletionMessages turns the transcript into the role/content shape the
// API expects. Only Vieno's own turns are "assistant"; the participant and
// every peer are "user", since the API has no third role.
//
// Every turn that is not Vieno's carries a speaker label, the participant's
// included. Labelling only the peers was not enough: consecutive same-role
// turns are folded into one message, so a peer's labelled disclosure and the
// participant's unlabelled reply arrived as one block with a name on the first
// half only. The host then acknowledged the peer's experience as though the
// participant had lived it, which testing reproduced on the first try and which
// can only happen in the two peer conditions, never in the baseline.
//
// Anthropic's API rejects a request outright unless roles strictly alternate
// user, assistant, user, assistant. The scripts do not: two peers can speak
// back to back (both "user"), and a stage can open with two consecutive host
// turns (both "assistant"). Consecutive turns of the same role are folded into
// one message, content joined by a blank line, so the request stays valid
// without losing anything either speaker said.
func chatCompletionMessages(present []string, participant string, history []mirrorTurn, participantText string) []map[string]string {
	messages := []map[string]string{
		{"role": "system", "content": mirrorSystemPrompt(present, participant)},
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
		speaker := turn.Sender
		if turn.IsUser {
			speaker = participant
		}
		appendTurn("user", speaker+": "+turn.Text)
	}
	appendTurn("user", participant+": "+participantText)
	return messages
}

// mirrorUsage is what OpenRouter reports about a call. It arrives in every
// response without being asked for. Recorded per call because the paper has to
// state inference cost, and a dashboard total cannot be broken down by
// participant or by condition after the fact. Cost is in USD as charged to the
// account.
type mirrorUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Cost             float64 `json:"cost"`
}

// Outcomes recorded in mirror_usage. They match the transcript's `mirror` field
// with two differences. A declined turn is not recorded at all, since nothing was
// ever going to be generated for it. And an empty submission is recorded as
// empty-input rather than fallback, because it never reached OpenRouter, so
// counting it as a generation failure would overstate how often the model fails.
const (
	mirrorOutcomeGenerated  = "generated"
	mirrorOutcomeNotSerious = "not-serious"
	mirrorOutcomeFallback   = "fallback"
	mirrorOutcomeEmptyInput = "empty-input"
)

// recordMirrorCall writes one row per attempted acknowledgement and logs it.
// It reports failure to the log and never to the caller: a participant's turn
// must not depend on whether the accounting write succeeded.
func (app *App) recordMirrorCall(sessionID, stage string, turnIndex int, outcome string, usage mirrorUsage, callErr error) {
	detail := ""
	if callErr != nil {
		detail = callErr.Error()
	}

	app.logger.Info("mirror call",
		"sessionId", sessionID, "stage", stage, "turnIndex", turnIndex,
		"outcome", outcome, "model", mirrorModel,
		"promptTokens", usage.PromptTokens, "completionTokens", usage.CompletionTokens,
		"costUSD", usage.Cost)

	_, err := app.db.Exec(`
        INSERT INTO mirror_usage
            (session_id, stage, turn_index, outcome, model,
             prompt_tokens, completion_tokens, total_tokens, cost, error, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, stage, turnIndex, outcome, mirrorModel,
		usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens, usage.Cost,
		detail, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		app.logger.Error("failed to record mirror usage",
			"sessionId", sessionID, "stage", stage, "error", err)
	}
}

// callOpenRouter returns the model's raw reply and what the call cost, or an
// error. Callers fall back on any error rather than surfacing it. The usage
// return is zero on every error path, since a failed call is not billed.
func callOpenRouter(ctx context.Context, present []string, participant string, history []mirrorTurn, participantText string) (string, mirrorUsage, error) {
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
		"messages": chatCompletionMessages(present, participant, history, participantText),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", mirrorUsage{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, mirrorEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", mirrorUsage{}, err
	}
	request.Header.Set("Authorization", "Bearer "+os.Getenv("OPENROUTER_API_KEY"))
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", mirrorUsage{}, err
	}
	defer response.Body.Close()

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage mirrorUsage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return "", mirrorUsage{}, fmt.Errorf("decoding openrouter response (status %d): %w", response.StatusCode, err)
	}

	// OpenRouter reports some failures as an error object inside a 200, so the
	// status code alone is not enough to tell success from failure.
	if parsed.Error != nil {
		return "", mirrorUsage{}, fmt.Errorf("openrouter error (status %d): %s", response.StatusCode, parsed.Error.Message)
	}
	if response.StatusCode != http.StatusOK {
		return "", mirrorUsage{}, fmt.Errorf("openrouter returned status %d", response.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", mirrorUsage{}, fmt.Errorf("openrouter returned no choices")
	}
	return parsed.Choices[0].Message.Content, parsed.Usage, nil
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
		// TurnIndex is 1 for the participant's first message in this stage and 2
		// for the second. It is the only thing that decides whether the stage
		// continues, and whether the invitation to add more is appended.
		TurnIndex int `json:"turnIndex"`
		// Declined is set when the participant took the invitation and chose to
		// add nothing. There is no text to acknowledge, so nothing is generated.
		Declined bool `json:"declined"`
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

	// Where the stage ends depends on the turn index and on nothing else. In
	// particular it must not depend on whether generation succeeded: a
	// participant who hits an OpenRouter blip on their first turn would
	// otherwise get a one-turn stage while everyone else gets two, which
	// reintroduces the very variability this structure removes. A failure costs
	// one acknowledgement, never a turn.
	advance := payload.TurnIndex >= mirrorTurnsPerStage

	// A decline has no text to mirror, so it is answered with the fixed closing
	// line without calling the model at all.
	if payload.Declined {
		writeJSON(w, map[string]any{
			"text":    mirrorDeclineAck,
			"mirror":  "declined",
			"advance": true,
		}, app.logger)
		return
	}

	text := clampMirrorInput(payload.Text)
	if text == "" {
		app.recordMirrorCall(payload.SessionID, payload.Stage, payload.TurnIndex,
			mirrorOutcomeEmptyInput, mirrorUsage{}, nil)
		writeJSON(w, map[string]any{
			"text":    hostReply(mirrorFallback, payload.TurnIndex),
			"mirror":  "fallback",
			"advance": advance,
		}, app.logger)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), mirrorTimeout)
	defer cancel()

	// Read off the session rather than taken from the request, so the label the
	// host is told to trust is the one the participant actually registered with
	// and cannot be set per call.
	participant := participantLabel(stringField(string(session.PreSurveyData), "display_name"))

	present := presentPeers(session.Condition)
	raw, usage, err := callOpenRouter(ctx, present, participant, payload.History, text)
	if err != nil {
		app.logger.Warn("mirror generation failed, using fallback",
			"sessionId", payload.SessionID, "stage", payload.Stage, "error", err)
		app.recordMirrorCall(payload.SessionID, payload.Stage, payload.TurnIndex,
			mirrorOutcomeFallback, mirrorUsage{}, err)
		writeJSON(w, map[string]any{
			"text":    hostReply(mirrorFallback, payload.TurnIndex),
			"mirror":  "fallback",
			"advance": advance,
		}, app.logger)
		return
	}

	// The host's own judgment about whether there is an answer here at all. It
	// catches what no rule can: a fluent sentence about nothing, the question
	// pasted back, "test test test". It is imperfect and non-deterministic, which
	// is affordable precisely because it cannot change the structure. It changes
	// one sentence Vieno says, and marks the turn so the row is findable when
	// exclusions are decided.
	stripped, notAnAnswer := extractNotAnAnswer(raw)
	if notAnAnswer {
		app.logger.Info("host judged a submission not a serious answer",
			"sessionId", payload.SessionID, "stage", payload.Stage, "turnIndex", payload.TurnIndex)
		// Billed like any other call, so it carries its usage.
		app.recordMirrorCall(payload.SessionID, payload.Stage, payload.TurnIndex,
			mirrorOutcomeNotSerious, usage, nil)
		writeJSON(w, map[string]any{
			"text":    hostReply(mirrorNotSerious, payload.TurnIndex),
			"mirror":  "not-serious",
			"advance": advance,
		}, app.logger)
		return
	}

	app.recordMirrorCall(payload.SessionID, payload.Stage, payload.TurnIndex,
		mirrorOutcomeGenerated, usage, nil)
	writeJSON(w, map[string]any{
		"text":    hostReply(sanitizeMirror(stripped), payload.TurnIndex),
		"mirror":  "generated",
		"advance": advance,
	}, app.logger)
}
