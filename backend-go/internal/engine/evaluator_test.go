package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
)

// ==================== RuleEvaluator Tests ====================

func TestRuleEvaluator_EmptyContent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	r := NewRuleEvaluator(logger)

	result := r.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "",
	})

	if result.Passed {
		t.Error("expected empty content to FAIL, but got PASS")
	}
	if result.Score != 0 {
		t.Errorf("expected score 0 for empty, got %.1f", result.Score)
	}
	if result.Track != "rule" {
		t.Errorf("expected track 'rule', got %s", result.Track)
	}
}

func TestRuleEvaluator_ShortContent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	r := NewRuleEvaluator(logger)

	result := r.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "short",
	})

	if result.Passed {
		t.Error("expected content <10 chars to FAIL, but got PASS")
	}
	if result.Score != 0.2 {
		t.Errorf("expected score 0.2, got %.1f", result.Score)
	}
}

func TestRuleEvaluator_ValidContent(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	r := NewRuleEvaluator(logger)

	// Content that passes all 5 rules: >=3 keywords, has structure, length 100-5000
	result := r.Evaluate(context.Background(), &DeliveryContent{
		JobID: 1,
		RawData: `## Analysis Report

1. Data collection and code review completed.
2. Test results confirm score improvement.
3. Summary of findings documented.

The implementation covers transaction data and risk assessment.
Additional analysis and report data show good code quality.`,
	})

	if !result.Passed {
		t.Errorf("expected valid content to PASS, but got FAIL (reason: %s)", result.Reason)
	}
	if result.Score < 0.5 || result.Score > 1.0 {
		t.Errorf("expected score between 0.5 and 1.0, got %.2f", result.Score)
	}
}

// ==================== LLMEvaluator Tests ====================

// mockLLMServer returns a server that mimics DeepSeek/OpenAI chat completions.
func mockLLMServer(t *testing.T, responseCode int, responseBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			// Our test base URL doesn't include /v1, so the full path should match
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected Authorization: Bearer test-key, got %s", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(responseCode)
		w.Write([]byte(responseBody))
	}))
}

func TestLLMEvaluator_Success(t *testing.T) {
	// Mock LLM returns a valid JSON response with PASS
	mockResp := `{
		"choices": [{
			"message": {
				"content": "{\"score\": 0.85, \"reason\": \"Delivery meets all requirements\", \"suggest_pass\": true}"
			}
		}]
	}`

	server := mockLLMServer(t, 200, mockResp)
	defer server.Close()

	logger, _ := zap.NewDevelopment()
	llm := NewLLMEvaluator(config.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "deepseek-chat",
		BaseURL: server.URL + "/v1",
		Timeout: 5 * time.Second,
	}, logger)

	result, err := llm.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "Good delivery content",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Error("expected PASS, got FAIL")
	}
	if result.Score != 0.85 {
		t.Errorf("expected score 0.85, got %.2f", result.Score)
	}
	if result.Track != "llm" {
		t.Errorf("expected track 'llm', got %s", result.Track)
	}
}

func TestLLMEvaluator_InvalidJSON(t *testing.T) {
	// LLM returns invalid JSON → should return error (caller falls back to rule)
	mockResp := `{
		"choices": [{
			"message": {
				"content": "This is not JSON at all"
			}
		}]
	}`

	server := mockLLMServer(t, 200, mockResp)
	defer server.Close()

	logger, _ := zap.NewDevelopment()
	llm := NewLLMEvaluator(config.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "deepseek-chat",
		BaseURL: server.URL + "/v1",
		Timeout: 5 * time.Second,
	}, logger)

	_, err := llm.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "some content",
	})

	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestLLMEvaluator_HTTPError(t *testing.T) {
	// Server returns 500 → should return error
	server := mockLLMServer(t, 500, `{"error": "internal error"}`)
	defer server.Close()

	logger, _ := zap.NewDevelopment()
	llm := NewLLMEvaluator(config.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "deepseek-chat",
		BaseURL: server.URL + "/v1",
		Timeout: 5 * time.Second,
	}, logger)

	_, err := llm.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "some content",
	})

	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestLLMEvaluator_Timeout(t *testing.T) {
	// Server that delays response → should timeout
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-done
	}))
	defer server.Close()
	defer close(done)

	logger, _ := zap.NewDevelopment()
	llm := NewLLMEvaluator(config.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "deepseek-chat",
		BaseURL: server.URL + "/v1",
		Timeout: 50 * time.Millisecond, // quick timeout for test
	}, logger)

	_, err := llm.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "some content",
	})

	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestLLMEvaluator_EmptyChoices(t *testing.T) {
	mockResp := `{"choices": []}`

	server := mockLLMServer(t, 200, mockResp)
	defer server.Close()

	logger, _ := zap.NewDevelopment()
	llm := NewLLMEvaluator(config.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "deepseek-chat",
		BaseURL: server.URL + "/v1",
		Timeout: 5 * time.Second,
	}, logger)

	_, err := llm.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "some content",
	})

	if err == nil {
		t.Error("expected error for empty choices, got nil")
	}
}

func TestLLMEvaluator_ScoreClamping(t *testing.T) {
	// Score outside [0, 1] should be clamped to 0.5
	mockResp := `{
		"choices": [{
			"message": {
				"content": "{\"score\": 99.0, \"reason\": \"weird score\", \"suggest_pass\": true}"
			}
		}]
	}`

	server := mockLLMServer(t, 200, mockResp)
	defer server.Close()

	logger, _ := zap.NewDevelopment()
	llm := NewLLMEvaluator(config.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "deepseek-chat",
		BaseURL: server.URL + "/v1",
		Timeout: 5 * time.Second,
	}, logger)

	result, err := llm.Evaluate(context.Background(), &DeliveryContent{
		JobID:   1,
		RawData: "some content",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Score != 0.5 {
		t.Errorf("expected score clamped to 0.5, got %.2f", result.Score)
	}
}

// ==================== Aggregator Tests ====================

func TestAggregator_RuleFail(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	a := NewAggregator(logger)

	ruleResult := &EvaluationResult{Passed: false, Score: 0, Reason: "empty content", Track: "rule"}
	llmResult := &EvaluationResult{Passed: true, Score: 0.9, Track: "llm"}

	verdict := a.Aggregate(ruleResult, llmResult, nil)

	if verdict.Passed {
		t.Error("expected FAIL when rule fails, regardless of LLM")
	}
	if verdict.Status != "slashed" {
		t.Errorf("expected 'slashed', got '%s'", verdict.Status)
	}
}

func TestAggregator_RulePass_LLMError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	a := NewAggregator(logger)

	ruleResult := &EvaluationResult{Passed: true, Score: 0.7, Track: "rule"}

	// LLM returned an error → should fall back to neutral score 0.5
	verdict := a.Aggregate(ruleResult, nil, json.Unmarshal([]byte("bad"), &struct{}{}))

	if !verdict.Passed {
		t.Error("expected PASS when LLM errors, falling back to rule")
	}
	if verdict.Status != "verified" {
		t.Errorf("expected 'verified', got '%s'", verdict.Status)
	}
}

func TestAggregator_LLMLowScore(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	a := NewAggregator(logger)

	ruleResult := &EvaluationResult{Passed: true, Score: 0.7, Track: "rule"}
	llmResult := &EvaluationResult{Passed: false, Score: 0.3, Reason: "quality concerns", Track: "llm"}

	verdict := a.Aggregate(ruleResult, llmResult, nil)

	if verdict.Passed {
		t.Error("expected FAIL when LLM score < 0.6")
	}
	if verdict.Status != "pending_review" {
		t.Errorf("expected 'pending_review', got '%s'", verdict.Status)
	}
}

func TestAggregator_BothPass(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	a := NewAggregator(logger)

	ruleResult := &EvaluationResult{Passed: true, Score: 0.7, Track: "rule"}
	llmResult := &EvaluationResult{Passed: true, Score: 0.85, Reason: "good work", Track: "llm"}

	verdict := a.Aggregate(ruleResult, llmResult, nil)

	if !verdict.Passed {
		t.Error("expected PASS when both tracks pass")
	}
	if verdict.Status != "verified" {
		t.Errorf("expected 'verified', got '%s'", verdict.Status)
	}
	// Score should be LLM score
	expectedScore := 0.85
	if verdict.Score > expectedScore+0.01 || verdict.Score < expectedScore-0.01 {
		t.Errorf("expected score %.2f, got %.2f", expectedScore, verdict.Score)
	}
}
