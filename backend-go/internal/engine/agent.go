package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
)

// AgentDecision represents the LLM's analysis result for a task.
type AgentDecision struct {
	NeedsSubBounty bool   `json:"needs_sub_bounty"`
	Reasoning      string `json:"reasoning"`
	SubDescription string `json:"sub_task_description,omitempty"`
	SubAmount      string `json:"sub_amount,omitempty"`
}

// AgentEngine orchestrates agent decisions via LLM.
type AgentEngine struct {
	llm  *LLMEvaluator
	log  *zap.Logger
	cfg  config.OpenAIConfig
}

// NewAgentEngine creates a new agent decision engine.
func NewAgentEngine(llm *LLMEvaluator, cfg config.OpenAIConfig, log *zap.Logger) *AgentEngine {
	return &AgentEngine{
		llm:  llm,
		log:  log,
		cfg:  cfg,
	}
}

// AnalyzeTask uses LLM to determine if a task needs a sub-bounty.
func (e *AgentEngine) AnalyzeTask(ctx context.Context, intent, amount string) (*AgentDecision, error) {
	prompt := fmt.Sprintf(`You are a task decomposition AI for a bounty platform. Analyze this task and decide if it needs to be broken down into sub-tasks.

Task: %s
Budget: %s ETH

IMPORTANT: For demonstration purposes, this task REQUIRES delegation to a sub-provider.
The task is complex enough that it needs at least one sub-task to be delegated.

Think step by step:
1. Identify which parts need specialized skills
2. Describe the sub-task that can be delegated
3. Estimate a reasonable budget for the sub-task (30-40%% of main budget)

Output ONLY valid JSON:
{
  "needs_sub_bounty": true,
  "reasoning": "explain in Chinese why delegation is needed, max 80 chars",
  "sub_task_description": "describe ONE specific sub-task in Chinese (数据收集/分析/报告撰写等)",
  "sub_amount": "ETH amount for sub-task (e.g. 0.003)"
}`, intent, amount)

	resp, err := e.queryLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("agent LLM query: %w", err)
	}

	jsonStr := extractJSON(resp)
	if jsonStr == "" {
		return &AgentDecision{
			NeedsSubBounty: false,
			Reasoning:      "LLM response parsing failed, defaulting to no sub-bounty",
		}, nil
	}

	var decision AgentDecision
	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		return &AgentDecision{
			NeedsSubBounty: false,
			Reasoning:      "Failed to parse LLM decision, defaulting to no sub-bounty",
		}, nil
	}

	return &decision, nil
}

// GenerateSubDelivery uses LLM to generate simulated delivery content for a sub-bounty.
// The prompt is designed to produce high-quality content that will pass AEP evaluation.
func (e *AgentEngine) GenerateSubDelivery(ctx context.Context, intent string, subDescription string) (string, error) {
	prompt := fmt.Sprintf(`You are a Sub-Provider completing a delegated sub-task. Generate a high-quality delivery report.

Main task: %s
Sub-task: %s

Requirements:
1. Write 200-300 characters in Chinese.
2. Include specific data, analysis, and concrete conclusions.
3. CRITICAL: Include the following keywords naturally in your content: 数据分析, 报告, 结论
4. Structure the report with markdown headings:
   ## 摘要
   [summary content]
   ## 数据分析
   [data analysis with specific metrics]
   ## 结论
   [conclusion]
5. Make sure each section has clear heading markers (##).
6. Include at least one numbered list or data table.
7. Make it look like a professional research report.

Output ONLY the delivery content, no JSON wrapper.`, intent, subDescription)

	return e.queryLLM(ctx, prompt)
}

// MergeDeliveries uses LLM to merge sub-provider delivery with provider's own content.
// The provider acts as the aggregator, adding context and synthesizing the final output.
func (e *AgentEngine) MergeDeliveries(ctx context.Context, originalIntent string, subDelivery string) (string, error) {
	prompt := fmt.Sprintf(`You are a Provider who delegated part of a task to a Sub-Provider.
Now merge the Sub-Provider's delivery with your own analysis into a complete final report.

Original task: %s
Sub-Provider's delivery: %s

Requirements:
1. Write 400-500 characters in Chinese.
2. Begin with "#【Final Report】" as the title.
3. Use markdown structure:
   ## 执行摘要
   [executive summary]
   ## 子任务成果
   [credit Sub-Provider's work]
   ## 综合分析
   [your own analysis]
   ## 最终结论
   [final conclusion]
4. Credit the Sub-Provider appropriately.
5. Each section MUST have ## headings.
6. Make it comprehensive and professional.

Output ONLY the final report content, no JSON wrapper.`, originalIntent, subDelivery)

	return e.queryLLM(ctx, prompt)
}

// GenerateFailedDelivery produces intentionally poor-quality content that will
// fail AEP evaluation — used for demo arbitration scenarios.
// Content is: too short, lacks structure, no keywords, informal tone.
func (e *AgentEngine) GenerateFailedDelivery(ctx context.Context, originalIntent string) (string, error) {
	prompt := fmt.Sprintf(`You are a Provider who deliberately submits a LOW-QUALITY deliverable.
Generate a very brief, unstructured, and informal response.

Original task: %s

Requirements:
1. Write only 2-3 short sentences in Chinese, maximum 40 characters total.
2. NO markdown headings, NO structure, NO formatting.
3. Do NOT include any of these keywords: 分析, 数据, 结果, 报告, 总结, 评估, 摘要
4. Use casual, unprofessional language like a quick chat message.
5. Include a vague promise like "明天再补" or "稍后更新".
6. Do NOT provide any actual analysis or data.

Output ONLY the delivery content, no JSON wrapper.`, originalIntent)

	return e.queryLLM(ctx, prompt)
}

// queryLLM sends a prompt to the LLM API and returns the response text.
func (e *AgentEngine) queryLLM(ctx context.Context, prompt string) (string, error) {
	baseURL := e.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := e.cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	apiKey := e.cfg.APIKey
	if apiKey == "" {
		return "", fmt.Errorf("LLM API key not configured")
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
	}

	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	timeout := e.cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LLM API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse LLM response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}

// extractJSON finds the first JSON object in a string.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(s, "}")
	if end < start {
		return ""
	}
	return s[start : end+1]
}
