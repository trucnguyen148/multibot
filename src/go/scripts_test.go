package main

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// loadScripts reads the same data.json the server reads at startup.
func loadScripts(t *testing.T) experimentData {
	t.Helper()
	content, err := os.ReadFile("data.json")
	if err != nil {
		t.Fatalf("failed to read data.json: %v", err)
	}
	var data experimentData
	if err := json.Unmarshal(content, &data); err != nil {
		t.Fatalf("failed to parse data.json: %v", err)
	}
	return data
}

func turnsWithTag(turns []BotScript, tag string) []BotScript {
	var out []BotScript
	for _, turn := range turns {
		if turn.Tag == tag {
			out = append(out, turn)
		}
	}
	return out
}

func wordCount(turns []BotScript) int {
	total := 0
	for _, turn := range turns {
		total += len(strings.Fields(turn.Text))
	}
	return total
}

// The participant answers the same question in every condition. If this test
// fails, the between-subjects comparison is confounded with question wording
// and the study cannot separate condition effects from prompt effects.
func TestParticipantQuestionIsIdenticalAcrossConditions(t *testing.T) {
	data := loadScripts(t)
	stages := []string{"STATE_CHAT_STAGE_1", "STATE_CHAT_STAGE_2", "STATE_CHAT_STAGE_3"}

	for _, stage := range stages {
		expected := ""
		for _, condition := range []string{"1-1", "2-1", "3-1"} {
			questions := turnsWithTag(data.Conditions[condition].Stages[stage], "question")
			if len(questions) != 1 {
				t.Fatalf("%s/%s: expected exactly 1 question turn, got %d", condition, stage, len(questions))
			}
			if expected == "" {
				expected = questions[0].Text
				continue
			}
			if questions[0].Text != expected {
				t.Errorf("%s/%s: question differs across conditions\n got: %q\nwant: %q",
					condition, stage, questions[0].Text, expected)
			}
		}
	}
}

// The baseline cell must contain no modelled vulnerability at all. That is the
// entire reason it exists.
func TestBaselineConditionHasNoDisclosure(t *testing.T) {
	data := loadScripts(t)
	for stage, turns := range data.Conditions["1-1"].Stages {
		if disclosures := turnsWithTag(turns, "disclosure"); len(disclosures) != 0 {
			t.Errorf("1-1/%s: baseline must not contain disclosure turns, found %d", stage, len(disclosures))
		}
	}
}

// 2-1 and 3-1 differ in how many peers deliver the disclosure, not in how much
// disclosure there is. A 10% tolerance leaves room for the connective words a
// second speaker needs without letting the dose drift.
func TestDisclosureVolumeMatchedBetweenPeerConditions(t *testing.T) {
	data := loadScripts(t)
	const tolerance = 0.10

	for _, stage := range []string{"STATE_CHAT_STAGE_1", "STATE_CHAT_STAGE_2", "STATE_CHAT_STAGE_3"} {
		one := wordCount(turnsWithTag(data.Conditions["2-1"].Stages[stage], "disclosure"))
		two := wordCount(turnsWithTag(data.Conditions["3-1"].Stages[stage], "disclosure"))

		if one == 0 && two == 0 {
			continue
		}
		if one == 0 || two == 0 {
			t.Errorf("%s: one peer condition has disclosure and the other does not (2-1=%d, 3-1=%d)", stage, one, two)
			continue
		}
		drift := math.Abs(float64(two-one)) / float64(one)
		if drift > tolerance {
			t.Errorf("%s: disclosure word count drifted %.1f%% (2-1=%d, 3-1=%d), tolerance %.0f%%",
				stage, drift*100, one, two, tolerance*100)
		}
	}
}

// lastSentence returns the final sentence of a turn, which is the part addressed
// to the participant in every hand-off.
func lastSentence(text string) string {
	trimmed := strings.TrimSpace(text)
	body := strings.TrimSuffix(trimmed, ".")
	body = strings.TrimSuffix(body, "?")
	body = strings.TrimSuffix(body, "!")
	if cut := strings.LastIndexAny(body, ".?!"); cut != -1 {
		return strings.TrimSpace(trimmed[cut+1:])
	}
	return trimmed
}

// Vieno now says more, and says different things to different peers, so the one
// sentence that reaches the participant has to be held still. Everything before
// it is addressed to the peers and necessarily differs, since the baseline has
// no peers to address. If this fails, the participant was invited to speak in
// different words depending on their condition, which is the same confound
// TestParticipantQuestionIsIdenticalAcrossConditions exists to prevent.
func TestHandoffToParticipantIsIdenticalAcrossConditions(t *testing.T) {
	data := loadScripts(t)

	for _, stage := range []string{"STATE_CHAT_STAGE_1", "STATE_CHAT_STAGE_2", "STATE_CHAT_STAGE_3"} {
		expected := ""
		for _, condition := range []string{"1-1", "2-1", "3-1"} {
			turns := data.Conditions[condition].Stages[stage]
			if len(turns) == 0 {
				t.Fatalf("%s/%s: no turns", condition, stage)
			}
			last := turns[len(turns)-1]
			if last.Tag != "ack" {
				t.Errorf("%s/%s: final turn is tagged %q, want ack", condition, stage, last.Tag)
				continue
			}
			got := lastSentence(last.Text)
			if expected == "" {
				expected = got
				continue
			}
			if got != expected {
				t.Errorf("%s/%s: the sentence handing the floor to the participant differs\n got: %q\nwant: %q",
					condition, stage, got, expected)
			}
		}
	}
}

// The host facilitates the same amount in both peer cells. She responds to a
// peer once per stage in each, so 3-1 differs from 2-1 in who is in the room and
// not in how attentive the host is. The baseline has no peers to respond to, so
// it is excluded rather than padded: the host behaves identically everywhere and
// simply has less to do.
func TestHostFacilitatesEquallyInBothPeerConditions(t *testing.T) {
	data := loadScripts(t)

	for _, stage := range []string{"STATE_CHAT_STAGE_1", "STATE_CHAT_STAGE_2", "STATE_CHAT_STAGE_3"} {
		one := len(turnsWithTag(data.Conditions["2-1"].Stages[stage], "facilitation"))
		two := len(turnsWithTag(data.Conditions["3-1"].Stages[stage], "facilitation"))
		if one != two {
			t.Errorf("%s: host facilitation turns differ between peer cells (2-1=%d, 3-1=%d)", stage, one, two)
		}
		if one == 0 {
			t.Errorf("%s: peer cells have no host facilitation turn, so the peer exchange is a round robin again", stage)
		}
	}

	if facilitation := turnsWithTag(data.Conditions["1-1"].Stages["STATE_CHAT_STAGE_2"], "facilitation"); len(facilitation) != 0 {
		t.Errorf("1-1: baseline has %d facilitation turns, but there is no peer to facilitate", len(facilitation))
	}
}

// Only peers disclose. If the host ever discloses, her role stops being
// constant across conditions and the role confound comes back.
func TestHostNeverDiscloses(t *testing.T) {
	data := loadScripts(t)
	for condition, config := range data.Conditions {
		for stage, turns := range config.Stages {
			for _, turn := range turnsWithTag(turns, "disclosure") {
				if turn.Sender == "Vieno" {
					t.Errorf("%s/%s: host Vieno must not disclose, found %q", condition, stage, turn.Text)
				}
			}
		}
	}
}

// Vieno speaks last in every stage, so the participant always answers after
// hearing every peer and is always the one handed the floor.
func TestHostSpeaksLastInEveryStage(t *testing.T) {
	data := loadScripts(t)
	for condition, config := range data.Conditions {
		for stage, turns := range config.Stages {
			if len(turns) == 0 {
				t.Errorf("%s/%s: no turns", condition, stage)
				continue
			}
			if last := turns[len(turns)-1]; last.Sender != "Vieno" {
				t.Errorf("%s/%s: last speaker is %q, want Vieno", condition, stage, last.Sender)
			}
		}
	}
}

// Peers disclose in response to the question, never ahead of it. Every stage
// therefore runs question, then disclosure, then hand-off. Holding that
// discourse structure constant across stages is what makes the stage-to-stage
// comparison within a cell interpretable. Peer greetings before the stage 1
// question are fine; only disclosure ordering is constrained.
func TestDisclosureNeverPrecedesTheQuestion(t *testing.T) {
	data := loadScripts(t)
	for condition, config := range data.Conditions {
		for stage, turns := range config.Stages {
			questionAt := -1
			for i, turn := range turns {
				if turn.Tag == "question" {
					questionAt = i
					break
				}
			}
			if questionAt == -1 {
				t.Errorf("%s/%s: no question turn", condition, stage)
				continue
			}
			for i := 0; i < questionAt; i++ {
				if turns[i].Tag == "disclosure" {
					t.Errorf("%s/%s: turn %d (%s) discloses before the question is asked",
						condition, stage, i, turns[i].Sender)
				}
			}
		}
	}
}

// Every turn carries a tag from the known set. An untagged turn silently opts
// out of every invariant above, which is the failure mode these tests exist to
// prevent.
func TestEveryTurnCarriesAKnownTag(t *testing.T) {
	known := map[string]bool{
		"open": true, "peer-neutral": true, "question": true,
		"disclosure": true, "ack": true, "facilitation": true,
	}
	data := loadScripts(t)
	for condition, config := range data.Conditions {
		for stage, turns := range config.Stages {
			for i, turn := range turns {
				if !known[turn.Tag] {
					t.Errorf("%s/%s: turn %d has unknown tag %q", condition, stage, i, turn.Tag)
				}
			}
		}
	}
}
