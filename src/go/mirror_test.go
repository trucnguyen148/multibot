package main

import (
	"strings"
	"testing"
)

// The host acknowledges. It does not interpret, advise, or introduce content.
// Interpretation would be a second manipulation layered on the peer-count one,
// and published work finds interpretative agent personas make users more
// guarded rather than less.
func TestMirrorSystemPromptConstrainsTheHost(t *testing.T) {
	prompt := mirrorSystemPrompt(nil)

	required := []string{
		"one sentence",
		"only what the participant",
		"Do not evaluate",
		"do not introduce topics",
		"internal or system XML tags",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("system prompt is missing the constraint %q", phrase)
		}
	}
}

// The prompt is fixed once at the start of the conversation: it must not name
// a live turn count or stage, since both change every call and the host is
// meant to read them off the conversation history instead.
func TestMirrorSystemPromptCarriesNoLiveState(t *testing.T) {
	prompt := mirrorSystemPrompt(nil)
	forbidden := []string{"exchange 1", "exchange 2", "STATE_CHAT_STAGE"}
	for _, phrase := range forbidden {
		if strings.Contains(prompt, phrase) {
			t.Errorf("system prompt embeds live state %q, want it derived from history instead", phrase)
		}
	}
}

// A condition with no peer must read as a one-on-one conversation, not as a
// group with an unnamed silent member.
func TestMirrorSystemPromptWithNoPeerIsOneOnOne(t *testing.T) {
	prompt := mirrorSystemPrompt(nil)
	if !strings.Contains(prompt, "only parties in this conversation are you and the human participant") {
		t.Errorf("system prompt with no peers does not say this is a one-on-one conversation:\n%s", prompt)
	}
}

// A named peer must actually be named, or the host cannot acknowledge her by
// name when the participant's own reply does not.
func TestMirrorSystemPromptNamesPresentPeers(t *testing.T) {
	prompt := mirrorSystemPrompt([]string{"Sam", "Charlie"})
	for _, name := range []string{"Sam", "Charlie"} {
		if !strings.Contains(prompt, name) {
			t.Errorf("system prompt with peers present does not mention %q:\n%s", name, prompt)
		}
	}
}

func TestPresentPeersMatchesConditionRoster(t *testing.T) {
	cases := map[string][]string{
		"1-1": nil,
		"2-1": {"Sam"},
		"3-1": {"Sam", "Charlie"},
	}
	for condition, want := range cases {
		got := presentPeers(condition)
		if len(got) != len(want) {
			t.Errorf("presentPeers(%q) = %v, want %v", condition, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("presentPeers(%q) = %v, want %v", condition, got, want)
				break
			}
		}
	}
}

// Anthropic rejects a request outright unless roles strictly alternate. The
// scripts put two peers back to back, and a stage can open with two
// consecutive host turns, so this has to actually hold for real transcripts,
// not just contrived ones.
func TestChatCompletionMessagesAlternatesRoles(t *testing.T) {
	history := []mirrorTurn{
		{Sender: "Vieno", Text: "Hi everyone, welcome."},
		{Sender: "Vieno", Text: "Sam, Charlie, would you like to say hello?"},
		{Sender: "Sam", Text: "Hey, glad to be here."},
		{Sender: "Charlie", Text: "Hi all."},
		{Sender: "Vieno", Text: "What's been challenging lately?"},
		{Sender: "Sam", Text: "Mostly juggling small tasks."},
		{Sender: "Charlie", Text: "Same, keeping the calendar under control."},
	}
	messages := chatCompletionMessages([]string{"Sam", "Charlie"}, history, "For me it's the deadlines.")

	if messages[0]["role"] != "system" {
		t.Fatalf("messages[0] role = %q, want system", messages[0]["role"])
	}
	for i := 2; i < len(messages); i++ {
		if messages[i]["role"] == messages[i-1]["role"] {
			t.Errorf("messages[%d] and messages[%d] both have role %q, roles must alternate",
				i-1, i, messages[i]["role"])
		}
	}

	// The two peers precede the participant's own text and share its role, so
	// all three fold into one trailing user message; the participant's words
	// must still be in it, in full and unaltered.
	last := messages[len(messages)-1]
	if last["role"] != "user" || !strings.Contains(last["content"], "For me it's the deadlines.") {
		t.Errorf("last message = %v, want it to contain the participant's own text", last)
	}
}

// The marker tells the server the stage is over. It must never survive into
// what the participant reads, whatever surrounds it in the raw reply.
func TestExtractStageCompleteStripsTheMarker(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantText    string
		wantAdvance bool
	}{
		{"no marker", "That sounds like a lot to carry.", "That sounds like a lot to carry.", false},
		{"marker at the end", "That sounds hard. [STAGE_COMPLETE]", "That sounds hard. ", true},
		{"marker with no other text", "[STAGE_COMPLETE]", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, advance := extractStageComplete(tc.in)
			if text != tc.wantText || advance != tc.wantAdvance {
				t.Errorf("extractStageComplete(%q) = (%q, %v), want (%q, %v)",
					tc.in, text, advance, tc.wantText, tc.wantAdvance)
			}
			if strings.Contains(sanitizeMirror(text), stageCompleteMarker) {
				t.Errorf("marker survived sanitizeMirror for input %q", tc.in)
			}
		})
	}
}

func TestSanitizeMirrorCollapsesToOneSentence(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already one sentence", "That sounds like a lot to carry.", "That sounds like a lot to carry."},
		{"trims whitespace", "  That sounds hard.  ", "That sounds hard."},
		{"keeps only the first sentence", "That sounds hard. You should talk to someone.", "That sounds hard."},
		{"drops a leaked reasoning block", "<thinking>hm</thinking>That sounds hard.", "That sounds hard."},
		{"drops a stray unpaired tag", "That sounds <b>hard.", "That sounds hard."},
		{"empty falls back", "   ", mirrorFallback},
		{"tags only falls back", "<thinking>hm</thinking>", mirrorFallback},
		// Truncation is deliberately aggressive. Clipping an abbreviation is
		// cosmetic; letting a tacked-on second sentence of advice reach a
		// participant changes what the study measured.
		{"cuts at an abbreviation rather than risk a second sentence", "That sounds like a lot, esp. right now.", "That sounds like a lot, esp."},
		// A terminator inside a word is not a sentence end, so a decimal or a
		// URL does not split the reply mid-token.
		{"a terminator inside a word is not a sentence end", "You lost 2.5 days to that.", "You lost 2.5 days to that."},
		// Too short to be a sentence on its own, so the reply continues.
		{"a leading fragment does not end the sentence", "Ah. That sounds really hard.", "Ah. That sounds really hard."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeMirror(tc.in); got != tc.want {
				t.Errorf("sanitizeMirror(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeMirrorBoundsLength(t *testing.T) {
	long := strings.Repeat("word ", 200) + "."
	got := sanitizeMirror(long)
	if len(strings.Fields(got)) > 30 {
		t.Errorf("sanitizeMirror returned %d words, want at most 30", len(strings.Fields(got)))
	}
}

// Participant text is forwarded to a third party and billed per token. A chat
// message cannot legitimately be book length, so an oversized body is trimmed
// rather than passed through or rejected: a participant must never be blocked.
func TestClampMirrorInputTrimsOversizedText(t *testing.T) {
	long := strings.Repeat("a", maxMirrorInputChars+500)
	if got := clampMirrorInput(long); len(got) != maxMirrorInputChars {
		t.Errorf("clampMirrorInput kept %d chars, want %d", len(got), maxMirrorInputChars)
	}
	if got := clampMirrorInput("  short  "); got != "short" {
		t.Errorf("clampMirrorInput(%q) = %q, want %q", "  short  ", got, "short")
	}
}
