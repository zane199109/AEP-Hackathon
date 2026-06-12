package rule

import (
	"fmt"
	"strings"
)

// RuleResult holds the outcome of a single rule check.
type RuleResult struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// RuleFunc is the signature for a rule template implementation.
type RuleFunc func(deliverable string, params map[string]interface{}) RuleResult

// ==================== Rule Templates ====================

// CheckKeywordCoverage verifies the deliverable contains all required keywords.
func CheckKeywordCoverage(deliverable string, params map[string]interface{}) RuleResult {
	raw, ok := params["keywords"]
	if !ok {
		return RuleResult{Name: "keyword_coverage", Pass: true, Detail: "No keywords specified — skipped"}
	}
	keywords, ok := raw.([]interface{})
	if !ok {
		return RuleResult{Name: "keyword_coverage", Pass: true, Detail: "Invalid keywords format — skipped"}
	}
	if len(keywords) == 0 {
		return RuleResult{Name: "keyword_coverage", Pass: true, Detail: "Empty keyword list — skipped"}
	}

	missing := []string{}
	lowerDel := strings.ToLower(deliverable)
	for _, kw := range keywords {
		kwStr := strings.TrimSpace(fmt.Sprintf("%v", kw))
		if kwStr == "" {
			continue
		}
		if !strings.Contains(lowerDel, strings.ToLower(kwStr)) {
			missing = append(missing, kwStr)
		}
	}

	pass := len(missing) == 0
	detail := fmt.Sprintf("All %d keywords found", len(keywords))
	if !pass {
		detail = fmt.Sprintf("Missing %d/%d keywords: %v", len(missing), len(keywords), missing)
	}
	return RuleResult{Name: "keyword_coverage", Pass: pass, Detail: detail}
}

// CheckMinLength verifies the deliverable word count meets the minimum.
func CheckMinLength(deliverable string, params map[string]interface{}) RuleResult {
	raw, ok := params["min_words"]
	if !ok {
		return RuleResult{Name: "min_length", Pass: true, Detail: "No min_words specified — skipped"}
	}

	var minWords int
	switch v := raw.(type) {
	case float64:
		minWords = int(v)
	case int:
		minWords = v
	default:
		return RuleResult{Name: "min_length", Pass: true, Detail: "Invalid min_words format — skipped"}
	}

	if minWords <= 0 {
		return RuleResult{Name: "min_length", Pass: true, Detail: "min_words <= 0 — skipped"}
	}

	wordCount := len(strings.Fields(deliverable))
	pass := wordCount >= minWords
	detail := fmt.Sprintf("Word count: %d (min: %d)", wordCount, minWords)
	if !pass {
		detail = fmt.Sprintf("Word count: %d, below minimum of %d", wordCount, minWords)
	}
	return RuleResult{Name: "min_length", Pass: pass, Detail: detail}
}

// CheckStructure verifies the deliverable contains expected section headings.
func CheckStructure(deliverable string, params map[string]interface{}) RuleResult {
	raw, ok := params["expected_sections"]
	if !ok {
		return RuleResult{Name: "structure_check", Pass: true, Detail: "No expected sections specified — skipped"}
	}
	sections, ok := raw.([]interface{})
	if !ok {
		return RuleResult{Name: "structure_check", Pass: true, Detail: "Invalid sections format — skipped"}
	}
	if len(sections) == 0 {
		return RuleResult{Name: "structure_check", Pass: true, Detail: "Empty section list — skipped"}
	}

	missing := []string{}
	lowerDel := strings.ToLower(deliverable)
	for _, sec := range sections {
		secStr := strings.TrimSpace(fmt.Sprintf("%v", sec))
		if secStr == "" {
			continue
		}
		// Check for the section heading in common markdown forms
		found := strings.Contains(lowerDel, strings.ToLower(secStr)) ||
			strings.Contains(lowerDel, "## "+strings.ToLower(secStr)) ||
			strings.Contains(lowerDel, "# "+strings.ToLower(secStr))
		if !found {
			missing = append(missing, secStr)
		}
	}

	pass := len(missing) == 0
	detail := fmt.Sprintf("All %d sections found", len(sections))
	if !pass {
		detail = fmt.Sprintf("Missing %d/%d sections: %v", len(missing), len(sections), missing)
	}
	return RuleResult{Name: "structure_check", Pass: pass, Detail: detail}
}
