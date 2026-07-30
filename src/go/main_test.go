package main

import "testing"

func TestBuildReverseScoredAnalysisPreservesRawValues(t *testing.T) {
	input := map[string]any{
		"AIAS_1": 1.0,
		"AIAS_2": 2.0,
		"AIAS_3": 3.0,
		"AIAS_4": 4.0,
		"DDI_2": 1.0,
		"DDI_4": 2.0,
		"DDI_5": 3.0,
		"DDI_8": 4.0,
		"DDI_9": 5.0,
		"DDI_12": 1.0,
		"BFNE_2": 1.0,
		"BFNE_4": 2.0,
		"BFNE_7": 3.0,
		"BFNE_10": 4.0,
		"Other": 9.0,
	}

	specs := []surveyScaleSpec{
		{prefix: "AIAS", reverseItems: []int{1, 2, 3, 4}, maxScore: 5.0},
		{prefix: "DDI", reverseItems: []int{2, 4, 5, 8, 9, 12}, maxScore: 5.0},
		{prefix: "BFNE", reverseItems: []int{2, 4, 7, 10}, maxScore: 5.0},
	}

	result := sanitizeSurveyData(input, specs)
	if result["AIAS_1"] != 1.0 {
		t.Fatalf("expected raw AIAS_1 to be preserved, got %#v", result["AIAS_1"])
	}
	if result["DDI_12"] != 1.0 {
		t.Fatalf("expected raw DDI_12 to be preserved, got %#v", result["DDI_12"])
	}
	if result["BFNE_2"] != 1.0 {
		t.Fatalf("expected raw BFNE_2 to be preserved, got %#v", result["BFNE_2"])
	}

	analysis, ok := result["reverse_scored_analysis"].(map[string]any)
	if !ok {
		t.Fatalf("expected reverse_scored_analysis block, got %#v", result["reverse_scored_analysis"])
	}

	aiasScale, ok := analysis["AIAS"].(map[string]any)
	if !ok {
		t.Fatalf("expected AIAS analysis block, got %#v", analysis["AIAS"])
	}
	reverseValues, ok := aiasScale["reverse_scored"].(map[string]any)
	if !ok {
		t.Fatalf("expected reverse_scored block for AIAS, got %#v", aiasScale["reverse_scored"])
	}
	if reverseValues["AIAS_1"] != 5.0 {
		t.Fatalf("expected AIAS_1 to be reverse-scored to 5, got %#v", reverseValues["AIAS_1"])
	}
	if reverseValues["AIAS_4"] != 2.0 {
		t.Fatalf("expected AIAS_4 to be reverse-scored to 2, got %#v", reverseValues["AIAS_4"])
	}

	bfnescale, ok := analysis["BFNE"].(map[string]any)
	if !ok {
		t.Fatalf("expected BFNE analysis block, got %#v", analysis["BFNE"])
	}
	bfneReverseValues, ok := bfnescale["reverse_scored"].(map[string]any)
	if !ok {
		t.Fatalf("expected reverse_scored block for BFNE, got %#v", bfnescale["reverse_scored"])
	}
	if bfneReverseValues["BFNE_2"] != 5.0 {
		t.Fatalf("expected BFNE_2 to be reverse-scored to 5, got %#v", bfneReverseValues["BFNE_2"])
	}

	ddiScale, ok := analysis["DDI"].(map[string]any)
	if !ok {
		t.Fatalf("expected DDI analysis block, got %#v", analysis["DDI"])
	}
	ddiReverseValues, ok := ddiScale["reverse_scored"].(map[string]any)
	if !ok {
		t.Fatalf("expected reverse_scored block for DDI, got %#v", ddiScale["reverse_scored"])
	}
	if ddiReverseValues["DDI_12"] != 5.0 {
		t.Fatalf("expected DDI_12 to be reverse-scored to 5, got %#v", ddiReverseValues["DDI_12"])
	}
}
