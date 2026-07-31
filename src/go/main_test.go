package main

import (
	"reflect"
	"testing"
)

func TestSanitizeSurveyDataPreservesRawValues(t *testing.T) {
	input := map[string]any{
		"AIAS_1": 1.0,
		"AIAS_2": 2.0,
		"DDI_12": 1.0,
		"BFNE_2": 1.0,
		"Other":  9.0,
	}

	// The updated sanitizeSurveyData only takes the data map
	result := sanitizeSurveyData(input)

	if result == nil {
		t.Fatal("expected a valid map, got nil")
	}

	// 1. Check that the exact raw values are preserved
	if result["AIAS_1"] != 1.0 {
		t.Fatalf("expected raw AIAS_1 to be preserved, got %#v", result["AIAS_1"])
	}
	if result["DDI_12"] != 1.0 {
		t.Fatalf("expected raw DDI_12 to be preserved, got %#v", result["DDI_12"])
	}
	if result["BFNE_2"] != 1.0 {
		t.Fatalf("expected raw BFNE_2 to be preserved, got %#v", result["BFNE_2"])
	}

	// 2. Ensure it returns a safe copy, not the exact same memory reference
	if reflect.ValueOf(input).Pointer() == reflect.ValueOf(result).Pointer() {
		t.Fatal("expected sanitizeSurveyData to return a new map copy, but got the original reference")
	}
}

func TestIsTestSession(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{"empty", "", false},
		{"malformed json", "{not json", false},
		{"real participant", `{"prolific_id":"abc"}`, false},
		{"test walkthrough", `{"prolific_id":"abc","test_mode":true}`, true},
		{"explicitly false", `{"test_mode":false}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTestSession(tc.data); got != tc.want {
				t.Fatalf("isTestSession(%q) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}

func TestSelectConditionBalancesOnStartedSessions(t *testing.T) {
	app := &App{conditions: []string{"1-1", "2-1", "3-1"}}

	tally := conditionTally{
		started:   map[string]int{"1-1": 5, "2-1": 2, "3-1": 9},
		completed: map[string]int{"1-1": 1, "2-1": 1, "3-1": 4},
	}

	got, err := app.selectCondition(tally)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2-1" {
		t.Fatalf("expected the least-started condition 2-1, got %q", got)
	}
}

func TestSelectConditionSkipsCompletedCells(t *testing.T) {
	app := &App{conditions: []string{"1-1", "2-1", "3-1"}}

	// 1-1 has the fewest started sessions but has already met its target, so it
	// must not be handed out again.
	tally := conditionTally{
		started:   map[string]int{"1-1": 0, "2-1": 3, "3-1": 8},
		completed: map[string]int{"1-1": targetUsersPerCondition, "2-1": 3, "3-1": 8},
	}

	got, err := app.selectCondition(tally)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2-1" {
		t.Fatalf("expected 2-1, got %q", got)
	}
}

func TestSelectConditionFullOnlyWhenEveryCellComplete(t *testing.T) {
	app := &App{conditions: []string{"1-1", "2-1", "3-1"}}

	// Plenty of abandoned attempts, but no cell has met its completion target,
	// so recruitment must stay open. This is the regression that previously
	// locked participants out.
	open := conditionTally{
		started:   map[string]int{"1-1": 200, "2-1": 200, "3-1": 200},
		completed: map[string]int{"1-1": 5, "2-1": 5, "3-1": 5},
	}
	if _, err := app.selectCondition(open); err != nil {
		t.Fatalf("expected recruitment to stay open, got error: %v", err)
	}

	full := conditionTally{
		started: map[string]int{"1-1": 60, "2-1": 60, "3-1": 60},
		completed: map[string]int{
			"1-1": targetUsersPerCondition,
			"2-1": targetUsersPerCondition,
			"3-1": targetUsersPerCondition,
		},
	}
	if _, err := app.selectCondition(full); err == nil {
		t.Fatal("expected allocation to be full once every cell met its target")
	}
}
