package rule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
)

// ExtractRuleParams calls the LLM to extract rule parameters from a task intent.
// Returns a JSON string suitable for storing in rule_params, or empty string on failure.
// Timeout: 5s. On failure, returns empty (rules are optional, not blocking).
func ExtractRuleParams(ctx context.Context, intent string, llmCfg config.OpenAIConfig, log *zap.Logger) string {
	if intent == "" {
		return ""
	}

	prompt := fmt.Sprintf(`From the following task description, extract parameters for evaluating the delivery and suggest task budget. Output ONLY valid JSON, no other text.

Task description: %s

Output format:
{
  "suggested_amount_eth": <number>,
  "suggested_deadline_days": <number>,
  "suggested_min_reputation": <number>,
  "templates": [
    { "name": "keyword_coverage", "params": { "keywords": ["keyword1", "keyword2", ...] } },
    { "name": "min_length", "params": { "min_words": <number> } },
    { "name": "structure_check", "params": { "expected_sections": ["Section 1", "Section 2", ...] } }
  ]
}

Guidelines:
- suggested_amount_eth: reasonable reward (0.001 to 1.0 ETH) based on task complexity
- suggested_deadline_days: estimated days to complete (1 to 30), based on scope
- suggested_min_reputation: minimum agent reputation (0-100), higher for complex tasks
- keyword_coverage: extract 3-8 key technical terms or topic words from the task
- min_length: estimate a reasonable minimum word count (50-2000 words)
- structure_check: extract 2-5 expected section headings

Only include rules where you can confidently extract parameters.`, intent)

	timeout := 5 * time.Second
	if llmCfg.Timeout > 0 && llmCfg.Timeout < timeout {
		timeout = llmCfg.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	baseURL := llmCfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := llmCfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You extract structured rule parameters from task descriptions. Output only JSON."},
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		log.Warn("RuleExtractor: failed to marshal request", zap.Error(err))
		return ""
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		log.Warn("RuleExtractor: failed to create request", zap.Error(err))
		return ""
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+llmCfg.APIKey)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		log.Warn("RuleExtractor: LLM call failed", zap.Error(err))
		return ""
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warn("RuleExtractor: failed to read response", zap.Error(err))
		return ""
	}

	if resp.StatusCode != 200 {
		log.Warn("RuleExtractor: LLM returned non-200",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBytes[:min(len(respBytes), 500)])),
		)
		return ""
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		log.Warn("RuleExtractor: failed to unmarshal response", zap.Error(err))
		return ""
	}

	if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content == "" {
		log.Warn("RuleExtractor: empty response")
		return ""
	}

	content := chatResp.Choices[0].Message.Content

	// Validate that the response is valid JSON with templates array
	var validation struct {
		Templates []RuleParam `json:"templates"`
	}
	if err := json.Unmarshal([]byte(content), &validation); err != nil {
		log.Warn("RuleExtractor: invalid JSON from LLM", zap.Error(err), zap.String("raw", content))
		return ""
	}

	if len(validation.Templates) == 0 {
		log.Info("RuleExtractor: LLM returned no templates")
		return ""
	}

	log.Info("RuleExtractor: extracted rule params",
		zap.Int("template_count", len(validation.Templates)),
	)

	return content
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
