package rule

import (
	"encoding/json"
)

// RuleParam defines a single rule template with its LLM-extracted parameters.
type RuleParam struct {
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

// RuleEngine holds all registered rule templates and executes them.
type RuleEngine struct {
	templates map[string]RuleFunc
}

// NewRuleEngine creates a RuleEngine with all built-in templates registered.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		templates: map[string]RuleFunc{
			"keyword_coverage": CheckKeywordCoverage,
			"min_length":       CheckMinLength,
			"structure_check":  CheckStructure,
		},
	}
}

// Evaluate runs all applicable rule templates against the deliverable.
// ruleParamsJSON is the JSON string stored in the bounty's rule_params field.
// Returns a list of RuleResult, one per template that was specified.
// On parse error, returns an empty slice (rules are advisory, not blocking).
func (e *RuleEngine) Evaluate(deliverable string, ruleParamsJSON string) []RuleResult {
	if ruleParamsJSON == "" || ruleParamsJSON == "null" || ruleParamsJSON == "[]" || ruleParamsJSON == "{}" {
		return []RuleResult{}
	}

	var paramsList []RuleParam
	if err := json.Unmarshal([]byte(ruleParamsJSON), &paramsList); err != nil {
		return []RuleResult{}
	}

	results := make([]RuleResult, 0, len(paramsList))
	for _, p := range paramsList {
		if fn, ok := e.templates[p.Name]; ok {
			results = append(results, fn(deliverable, p.Params))
		}
	}
	return results
}

// AllPassed returns true only if every rule result passed.
// Empty results (no rules evaluated) returns true.
func AllPassed(results []RuleResult) bool {
	for _, r := range results {
		if !r.Pass {
			return false
		}
	}
	return true
}
