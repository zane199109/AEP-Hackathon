package rule

import (
	"testing"
)

func TestCheckKeywordCoverage_AllPresent(t *testing.T) {
	result := CheckKeywordCoverage(
		"This report analyzes Aave and Compound protocols for TVL and APY trends.",
		map[string]interface{}{"keywords": []interface{}{"Aave", "Compound", "TVL", "APY"}},
	)
	if !result.Pass {
		t.Fatalf("expected PASS, got FAIL: %s", result.Detail)
	}
}

func TestCheckKeywordCoverage_Missing(t *testing.T) {
	result := CheckKeywordCoverage(
		"Just a simple note about DeFi.",
		map[string]interface{}{"keywords": []interface{}{"Aave", "Compound", "TVL"}},
	)
	if result.Pass {
		t.Fatalf("expected FAIL, got PASS: %s", result.Detail)
	}
	if result.Detail == "" {
		t.Fatal("expected non-empty detail")
	}
}

func TestCheckKeywordCoverage_NoKeywords(t *testing.T) {
	result := CheckKeywordCoverage("Some content", map[string]interface{}{})
	if !result.Pass {
		t.Fatal("expected PASS when no keywords specified")
	}
}

func TestCheckMinLength_AboveThreshold(t *testing.T) {
	words := "word word word word word" // 5 words
	result := CheckMinLength(words, map[string]interface{}{"min_words": float64(3)})
	if !result.Pass {
		t.Fatalf("expected PASS, got FAIL: %s", result.Detail)
	}
}

func TestCheckMinLength_BelowThreshold(t *testing.T) {
	result := CheckMinLength("short", map[string]interface{}{"min_words": float64(10)})
	if result.Pass {
		t.Fatalf("expected FAIL, got PASS: %s", result.Detail)
	}
}

func TestCheckMinLength_NoParam(t *testing.T) {
	result := CheckMinLength("short", map[string]interface{}{})
	if !result.Pass {
		t.Fatal("expected PASS when no min_words specified")
	}
}

func TestCheckStructure_AllSections(t *testing.T) {
	content := `## Introduction
This is the intro.

## Analysis  
Detailed analysis here.

## Conclusion
Final thoughts.`
	result := CheckStructure(content, map[string]interface{}{
		"expected_sections": []interface{}{"Introduction", "Analysis", "Conclusion"},
	})
	if !result.Pass {
		t.Fatalf("expected PASS, got FAIL: %s", result.Detail)
	}
}

func TestCheckStructure_MissingSections(t *testing.T) {
	content := `## Introduction only`
	result := CheckStructure(content, map[string]interface{}{
		"expected_sections": []interface{}{"Introduction", "Methodology", "Results"},
	})
	if result.Pass {
		t.Fatalf("expected FAIL, got PASS: %s", result.Detail)
	}
}

func TestEngine_EmptyParams(t *testing.T) {
	e := NewRuleEngine()
	results := e.Evaluate("some content", "")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty params, got %d", len(results))
	}
}

func TestEngine_ValidParams(t *testing.T) {
	e := NewRuleEngine()
	results := e.Evaluate("This report analyzes Aave protocol TVL trends and risk assessment.",
		`[{"name":"keyword_coverage","params":{"keywords":["Aave","TVL","risk"]}}]`)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Pass {
		t.Fatalf("expected PASS, got FAIL: %s", results[0].Detail)
	}
}

func TestEngine_MultipleRules(t *testing.T) {
	e := NewRuleEngine()
	results := e.Evaluate(
		"## Analysis\nThis report analyzes Aave and Compound protocols for TVL trends.",
		`[{"name":"keyword_coverage","params":{"keywords":["Aave","Compound","TVL"]}},{"name":"min_length","params":{"min_words":5}},{"name":"structure_check","params":{"expected_sections":["Analysis"]}}]`,
	)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Pass {
			t.Fatalf("rule %s FAILED: %s", r.Name, r.Detail)
		}
	}
	if !AllPassed(results) {
		t.Fatal("AllPassed returned false when all rules passed")
	}
}

func TestEngine_UnknownRuleSkipped(t *testing.T) {
	e := NewRuleEngine()
	results := e.Evaluate("some content", `[{"name":"nonexistent_rule","params":{}}]`)
	if len(results) != 0 {
		t.Fatalf("expected 0 results for unknown rule, got %d", len(results))
	}
}

func TestAllPassed_Empty(t *testing.T) {
	if !AllPassed([]RuleResult{}) {
		t.Fatal("AllPassed should return true for empty results")
	}
}
