package policy

import (
	"strings"
	"testing"
)

// assertNonEmptyReason verifies that the recommendation's Reason field is non-empty.
func assertNonEmptyReason(test *testing.T, rec Recommendation) {
	test.Helper()
	if strings.TrimSpace(rec.Reason) == "" {
		test.Errorf("expected non-empty reason, got empty string")
	}
}

// assertRecommendedNotInAlternatives verifies that the recommended model does
// not appear in the alternatives list.
func assertRecommendedNotInAlternatives(test *testing.T, rec Recommendation) {
	test.Helper()
	for altIndex, alt := range rec.Alternatives {
		if alt.Model == rec.RecommendedModel {
			test.Errorf("recommended model %q must not appear in alternatives (found at index %d)", rec.RecommendedModel, altIndex)
		}
	}
}

func TestRecommend_SpeedPriority(test *testing.T) {
	rec := Recommend("generate a banner image", prioritySpeed)

	if rec.RecommendedModel != modelOriginal {
		test.Errorf("expected %q, got %q", modelOriginal, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_QualityPriority(test *testing.T) {
	rec := Recommend("generate a banner image", priorityQuality)

	if rec.RecommendedModel != modelPro {
		test.Errorf("expected %q, got %q", modelPro, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_BalancedWithProKeyword(test *testing.T) {
	rec := Recommend("create a professional headshot", priorityBalanced)

	if rec.RecommendedModel != modelPro {
		test.Errorf("expected %q for pro keyword, got %q", modelPro, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_BalancedWithSpeedKeyword(test *testing.T) {
	rec := Recommend("make a quick thumbnail", priorityBalanced)

	if rec.RecommendedModel != modelOriginal {
		test.Errorf("expected %q for speed keyword, got %q", modelOriginal, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_BalancedDefault(test *testing.T) {
	rec := Recommend("generate a product image", priorityBalanced)

	if rec.RecommendedModel != modelBalanced {
		test.Errorf("expected %q for no-keyword balanced, got %q", modelBalanced, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_EmptyPriority(test *testing.T) {
	rec := Recommend("generate a product image", "")

	if rec.RecommendedModel != modelBalanced {
		test.Errorf("expected %q for empty priority, got %q", modelBalanced, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_UnrecognizedPriority(test *testing.T) {
	rec := Recommend("generate a product image", "foo")

	if rec.RecommendedModel != modelBalanced {
		test.Errorf("expected %q for unrecognized priority, got %q", modelBalanced, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_CaseInsensitiveKeyword(test *testing.T) {
	rec := Recommend("create a PROFESSIONAL portrait", priorityBalanced)

	if rec.RecommendedModel != modelPro {
		test.Errorf("expected %q for uppercase keyword, got %q", modelPro, rec.RecommendedModel)
	}
	assertNonEmptyReason(test, rec)
	assertRecommendedNotInAlternatives(test, rec)
}

func TestRecommend_AlternativeOrdering_Pro(test *testing.T) {
	rec := Recommend("", priorityQuality)

	if len(rec.Alternatives) < 2 {
		test.Fatalf("expected at least 2 alternatives, got %d", len(rec.Alternatives))
	}
	if rec.Alternatives[0].Model != modelBalanced {
		test.Errorf("expected first alternative %q, got %q", modelBalanced, rec.Alternatives[0].Model)
	}
	if rec.Alternatives[1].Model != modelOriginal {
		test.Errorf("expected second alternative %q, got %q", modelOriginal, rec.Alternatives[1].Model)
	}
}

func TestRecommend_AlternativeOrdering_Original(test *testing.T) {
	rec := Recommend("", prioritySpeed)

	if len(rec.Alternatives) < 2 {
		test.Fatalf("expected at least 2 alternatives, got %d", len(rec.Alternatives))
	}
	if rec.Alternatives[0].Model != modelBalanced {
		test.Errorf("expected first alternative %q, got %q", modelBalanced, rec.Alternatives[0].Model)
	}
	if rec.Alternatives[1].Model != modelPro {
		test.Errorf("expected second alternative %q, got %q", modelPro, rec.Alternatives[1].Model)
	}
}

func TestRecommend_AlternativeOrdering_Balanced(test *testing.T) {
	rec := Recommend("generate an image", priorityBalanced)

	if len(rec.Alternatives) < 2 {
		test.Fatalf("expected at least 2 alternatives, got %d", len(rec.Alternatives))
	}
	if rec.Alternatives[0].Model != modelPro {
		test.Errorf("expected first alternative %q, got %q", modelPro, rec.Alternatives[0].Model)
	}
	if rec.Alternatives[1].Model != modelOriginal {
		test.Errorf("expected second alternative %q, got %q", modelOriginal, rec.Alternatives[1].Model)
	}
}

func TestRecommend_ReasonContainsPriorityContext_Speed(test *testing.T) {
	rec := Recommend("make an image", prioritySpeed)

	if !strings.Contains(rec.Reason, "speed") {
		test.Errorf("expected reason to contain \"speed\", got: %q", rec.Reason)
	}
}

func TestRecommend_ReasonContainsPriorityContext_Quality(test *testing.T) {
	rec := Recommend("make an image", priorityQuality)

	if !strings.Contains(rec.Reason, "quality") {
		test.Errorf("expected reason to contain \"quality\", got: %q", rec.Reason)
	}
}

func TestRecommend_ReasonContainsKeywordContext_Balanced(test *testing.T) {
	rec := Recommend("create a detailed illustration", priorityBalanced)

	if !strings.Contains(rec.Reason, "detailed") {
		test.Errorf("expected reason to contain keyword \"detailed\", got: %q", rec.Reason)
	}
}

func TestRecommend_ProKeywordsFirstMatchWins(test *testing.T) {
	// "detailed" is a pro keyword; "draft" is a speed keyword — pro should win
	rec := Recommend("detailed draft sketch", priorityBalanced)

	if rec.RecommendedModel != modelPro {
		test.Errorf("expected pro model when pro keyword appears before speed keyword, got %q", rec.RecommendedModel)
	}
}

func TestRecommend_AllPathsHaveNonEmptyReason(test *testing.T) {
	testCases := []struct {
		description string
		priority    string
	}{
		{"image", prioritySpeed},
		{"image", priorityQuality},
		{"image", priorityBalanced},
		{"professional portrait", priorityBalanced},
		{"quick thumbnail", priorityBalanced},
		{"image", ""},
		{"image", "foo"},
	}

	for caseIndex, testCase := range testCases {
		rec := Recommend(testCase.description, testCase.priority)
		if strings.TrimSpace(rec.Reason) == "" {
			test.Errorf("case %d (desc=%q, priority=%q): expected non-empty reason", caseIndex, testCase.description, testCase.priority)
		}
	}
}
