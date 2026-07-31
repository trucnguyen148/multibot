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
	prompt := mirrorSystemPrompt()

	required := []string{
		"one sentence",
		"only what the participant",
		"Do not give advice",
		"Do not evaluate",
		"Do not introduce",
		"internal or system XML tags",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("system prompt is missing the constraint %q", phrase)
		}
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
