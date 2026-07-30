package main

import (
	"reflect"
	"testing"
)

func TestSanitizeSurveyDataPreservesRawValues(t *testing.T) {
	input := map[string]any{
		"AIAS_1":  1.0,
		"AIAS_2":  2.0,
		"DDI_12":  1.0,
		"BFNE_2":  1.0,
		"Other":   9.0,
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