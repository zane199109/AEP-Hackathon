package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
)

// ==================== Rule Evaluator ====================

// RuleEvaluator performs hard-coded rule checks on delivery content.
type RuleEvaluator struct {
	log *zap.Logger
}

// NewRuleEvaluator creates a rule evaluator.
func NewRuleEvaluator(log *zap.Logger) *RuleEvaluator {
	return &RuleEvaluator{log: log}
}

// EvaluationResult holds the outcome of a single evaluation track.
type EvaluationResult struct {
	Passed   bool    `json:"passed"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
	Track    string  `json:"track"` // "rule" or "llm"
	Fallback bool    `json:"fallback,omitempty"`
}

// DeliveryContent represents the submitted work for evaluation.
type DeliveryContent struct {
	JobID      uint64   `json:"job_id"`
	Seller     string   `json:"seller"`
	ResultHash string   `json:"result_hash"`
	RawData    string   `json:"raw_data"`
	Keywords   []string `json:"keywords,omitempty"`
}

// Evaluate runs hard rule checks against delivery content.
// Implements 5 rules:
//  1. Empty content – veto (Score 0, Fail)
//  2. Minimum length < 10 chars – veto (Score 0.2, Fail)
//  3. Keyword matching – uses delivery.Keywords (LLM-extracted from intent), default fallback if empty
//  4. Structure detection – sections (# headings) or numbered items
//  5. Length quality – bonus for 100–5000 chars, penalty for very short/long
func (r *RuleEvaluator) Evaluate(ctx context.Context, delivery *DeliveryContent) *EvaluationResult {
	r.log.Info("RuleEvaluator running",
		zap.Uint64("job_id", delivery.JobID),
		zap.String("result_hash", delivery.ResultHash),
	)

	// Rule 1: Empty content – veto
	if delivery.RawData == "" {
		return &EvaluationResult{
			Passed: false, Score: 0, Reason: "delivery content is empty", Track: "rule",
		}
	}

	// Rule 2: Too short – veto
	if len(delivery.RawData) < 10 {
		return &EvaluationResult{
			Passed: false, Score: 0.2,
			Reason: fmt.Sprintf("delivery too short (%d chars)", len(delivery.RawData)),
			Track:  "rule",
		}
	}

	content := delivery.RawData
	var reasons []string

	// ----- Rule 3: Keyword matching -----
	kwScore := computeKeywordScore(content, delivery.Keywords)
	kwPassed := kwScore >= 0.3 // threshold at 0.3
	reasons = append(reasons, fmt.Sprintf("keyword_match:%s(%.2f)", passFailStr(kwPassed), kwScore))

	// ----- Rule 4: Structure detection -----
	structScore := computeStructureScore(content)
	structPassed := structScore >= 0.3 // at least one structural element
	reasons = append(reasons, fmt.Sprintf("structure:%s(%.2f)", passFailStr(structPassed), structScore))

	// ----- Rule 5: Length quality -----
	lengthScore := computeLengthScore(len(content))
	lengthPassed := lengthScore >= 0.4
	reasons = append(reasons, fmt.Sprintf("length:%s(%.2f)", passFailStr(lengthPassed), lengthScore))

	// Weighted average of the 3 substantive rules (equal weights)
	finalScore := (kwScore + structScore + lengthScore) / 3.0
	finalScore = math.Round(finalScore*100) / 100

	// All non-veto rules must pass
	allPassed := kwPassed && structPassed && lengthPassed

	return &EvaluationResult{
		Passed: allPassed,
		Score:  finalScore,
		Reason: strings.Join(reasons, ", "),
		Track:  "rule",
	}
}

// passFailStr returns "PASS" or "FAIL" for a boolean.
func passFailStr(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

// computeKeywordScore returns a score [0.0, 1.0] based on how many
// of the given keywords appear in the content (case-insensitive, word-boundary check).
func computeKeywordScore(content string, keywords []string) float64 {
	lower := strings.ToLower(content)
	if len(keywords) == 0 {
		// Fallback: use a small set of generic quality indicators
		keywords = []string{"分析", "数据", "结果", "报告", "总结"}
	}
	found := 0
	for _, kw := range keywords {
		if containsWord(lower, kw) {
			found++
		}
	}
	// Score scales linearly: 0 keywords → 0.0, 2 keywords → 0.5, 4+ keywords → 1.0
	total := float64(len(keywords))
	if total == 0 {
		return 1.0
	}
	raw := float64(found) / total * 2.0 // normalize so 50% of keywords found = ~1.0
	return math.Min(1.0, raw)
}

// containsWord checks if needle appears as a standalone word in haystack.
// A word boundary is any non-letter/non-digit character.
func containsWord(haystack, needle string) bool {
	idx := strings.Index(haystack, needle)
	if idx < 0 {
		return false
	}
	// Check character before
	if idx > 0 {
		prev := rune(haystack[idx-1])
		if unicode.IsLetter(prev) || unicode.IsDigit(prev) {
			// Try next occurrence after the false match
			return containsWord(haystack[idx+len(needle):], needle)
		}
	}
	// Check character after
	end := idx + len(needle)
	if end < len(haystack) {
		next := rune(haystack[end])
		if unicode.IsLetter(next) || unicode.IsDigit(next) {
			// Try next occurrence
			return containsWord(haystack[end:], needle)
		}
	}
	return true
}

// computeStructureScore returns a score [0.0, 1.0] based on structural elements:
//   - Markdown-style headings (## or #)
//   - Numbered items (1. 2. 3. etc.)
//   - Bullet points (- item or * item)
func computeStructureScore(content string) float64 {
	lines := strings.Split(content, "\n")
	score := 0.0

	hasHeadings := false
	hasNumbered := false
	hasBullets := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Heading detection: starts with ## or #
		if strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "# ") {
			hasHeadings = true
		}
		// Numbered item detection: "1.", "2.", etc.
		if len(trimmed) >= 2 {
			first := rune(trimmed[0])
			if unicode.IsDigit(first) && trimmed[1] == '.' {
				hasNumbered = true
			}
		}
		// Bullet point detection: starts with "- " or "* "
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			hasBullets = true
		}
	}

	if hasHeadings {
		score += 0.4
	}
	if hasNumbered {
		score += 0.4
	}
	if hasBullets {
		score += 0.2
	}

	return math.Min(1.0, score)
}

// computeLengthScore returns a score [0.0, 1.0] based on content length:
//   - 100–5000 chars: bonus (1.0)
//   - 5001–10000 chars: decent (0.7)
//   - 50–99 chars: marginal (0.3)
//   - 10–49 chars: very short penalty (0.1)
//   - > 10000 chars: extremely long penalty (0.2)
func computeLengthScore(length int) float64 {
	switch {
	case length >= 100 && length <= 5000:
		return 1.0
	case length > 5000 && length <= 10000:
		return 0.7
	case length >= 50 && length < 100:
		return 0.3
	case length < 50:
		return 0.1 // < 50 but >= 10 (since < 10 is vetoed earlier)
	default: // > 10000
		return 0.2
	}
}

// ==================== LLM Evaluator ====================

// LLMEvaluator calls an OpenAI-compatible API (OpenAI, DeepSeek, etc.) to evaluate delivery quality.
// This is the secondary track — if it fails/timeouts, the rule result stands.
type LLMEvaluator struct {
	cfg     config.OpenAIConfig
	httpCli *http.Client
	log     *zap.Logger
}

// NewLLMEvaluator creates an LLM evaluator.
// baseURL defaults to "https://api.openai.com/v1" if empty.
func NewLLMEvaluator(cfg config.OpenAIConfig, log *zap.Logger) *LLMEvaluator {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &LLMEvaluator{
		cfg:     cfg,
		httpCli: &http.Client{Timeout: timeout},
		log:     log,
	}
}

func (l *LLMEvaluator) baseURL() string {
	if l.cfg.BaseURL != "" {
		return l.cfg.BaseURL
	}
	return "https://api.openai.com/v1"
}

func (l *LLMEvaluator) model() string {
	if l.cfg.Model != "" {
		return l.cfg.Model
	}
	return "gpt-4o"
}

// chatCompletionRequest mirrors the OpenAI-compatible chat completion request body.
type chatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

// chatCompletionResponse mirrors the OpenAI-compatible response.
type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// LLMJudgeResponse is the structured JSON expected from the LLM.
type LLMJudgeResponse struct {
	Score       float64 `json:"score"`
	Reason      string  `json:"reason"`
	SuggestPass bool    `json:"suggest_pass"`
}

// Evaluate calls the LLM to judge delivery quality.
// Red line: 10s timeout, on failure → return error so caller falls back to rule result.
func (l *LLMEvaluator) Evaluate(ctx context.Context, delivery *DeliveryContent) (*EvaluationResult, error) {
	ctx, cancel := context.WithTimeout(ctx, l.cfg.Timeout)
	defer cancel()

	prompt := fmt.Sprintf(`You are a quality evaluator for a bounty platform. Evaluate the following delivery content.

Delivery content:
%s

Respond in JSON format only:
{"score": <0.0 to 1.0>, "reason": "<brief reason>", "suggest_pass": <true or false>}`, delivery.RawData)

	reqBody := chatCompletionRequest{
		Model: l.model(),
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", l.baseURL()+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)

	resp, err := l.httpCli.Do(req)
	if err != nil {
		l.log.Error("LLM call failed", zap.Uint64("job_id", delivery.JobID), zap.Error(err))
		return nil, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		l.log.Warn("LLM returned non-200",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBytes)),
		)
		return nil, fmt.Errorf("llm api error (status %d)", resp.StatusCode)
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	content := chatResp.Choices[0].Message.Content
	if content == "" {
		return nil, fmt.Errorf("llm returned empty response")
	}

	// Red line: if LLM returns invalid JSON, ignore it
	var judge LLMJudgeResponse
	if err := json.Unmarshal([]byte(content), &judge); err != nil {
		l.log.Warn("LLM returned invalid JSON, ignoring result",
			zap.Uint64("job_id", delivery.JobID),
			zap.String("raw", content),
		)
		return nil, fmt.Errorf("llm json parse error: %w", err)
	}

	if judge.Score < 0 || judge.Score > 1.0 {
		judge.Score = 0.5
	}

	l.log.Info("LLM evaluation complete",
		zap.Uint64("job_id", delivery.JobID),
		zap.Float64("score", judge.Score),
		zap.Bool("suggest_pass", judge.SuggestPass),
		zap.String("reason", judge.Reason),
	)

	return &EvaluationResult{
		Passed: judge.SuggestPass,
		Score:  judge.Score,
		Reason: judge.Reason,
		Track:  "llm",
	}, nil
}

// ==================== Aggregator ====================

// Aggregator combines rule and LLM results into a final verdict.
// Decision matrix (方案D):
//   - Rule Fail → final Fail, Score = 0
//   - Rule Pass + LLM Pass → final Pass, Score = LLM score
//   - Rule Pass + LLM Error → final Pass, Score = 0.5 (neutral fallback)
//   - Rule Pass + LLM score < 0.6 → final PendingReview, Score = LLM score
type Aggregator struct {
	log *zap.Logger
}

// NewAggregator creates an aggregator.
func NewAggregator(log *zap.Logger) *Aggregator {
	return &Aggregator{log: log}
}

// FinalVerdict is the aggregate decision.
type FinalVerdict struct {
	Passed    bool    `json:"passed"`
	Status    string  `json:"status"` // "verified", "slashed", "pending_review"
	Score     float64 `json:"score"`
	RuleScore float64 `json:"rule_score"`
	LLMScore  float64 `json:"llm_score"`
	LLMReason string  `json:"llm_reason"`
	Summary   string  `json:"summary"`
}

// Aggregate combines rule and LLM results into a final verdict.
func (a *Aggregator) Aggregate(ruleResult *EvaluationResult, llmResult *EvaluationResult, llmErr error) *FinalVerdict {
	a.log.Info("Aggregating verdict",
		zap.Bool("rule_passed", ruleResult.Passed),
		zap.Float64("rule_score", ruleResult.Score),
	)

	// Red line: Rule Fail → final Fail
	if !ruleResult.Passed {
		return &FinalVerdict{
			Passed: false, Status: "slashed", Score: 0,
			RuleScore: ruleResult.Score,
			Summary:   fmt.Sprintf("Rule evaluator rejected: %s", ruleResult.Reason),
		}
	}

	// Rule Pass + LLM error → ignore LLM, pass with neutral score
	if llmErr != nil || llmResult == nil {
		a.log.Info("LLM unavailable, using neutral score")
		return &FinalVerdict{
			Passed: true, Status: "verified", Score: 0.5,
			RuleScore: ruleResult.Score,
			Summary:   "Rule passed, LLM unavailable — verified by rule engine",
		}
	}

	// Rule Pass + LLM score < 0.6 → PendingReview
	if llmResult.Score < 0.6 {
		a.log.Info("LLM score below threshold, pending review",
			zap.Float64("llm_score", llmResult.Score),
		)
		return &FinalVerdict{
			Passed:    false, Status: "pending_review",
			Score:     llmResult.Score,
			RuleScore: ruleResult.Score,
			LLMScore:  llmResult.Score,
			LLMReason: llmResult.Reason,
			Summary:   fmt.Sprintf("LLM flagged (score: %.2f): %s", llmResult.Score, llmResult.Reason),
		}
	}

	// Rule Pass + LLM Pass → verified, score = LLM score
	return &FinalVerdict{
		Passed:    true, Status: "verified",
		Score:     llmResult.Score,
		RuleScore: ruleResult.Score,
		LLMScore:  llmResult.Score,
		LLMReason: llmResult.Reason,
		Summary:   fmt.Sprintf("Both tracks passed. LLM: %s", llmResult.Reason),
	}
}
