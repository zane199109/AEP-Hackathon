package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/engine"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/model"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/provider"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/relayer"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/reputation"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/rule"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/store"
)

// h.cfg.CAW.ProviderAddr and h.cfg.CAW.SubProviderAddr are now read from config:
// h.cfg.CAW.ProviderAddr and h.cfg.CAW.SubProviderAddr

type Handler struct {
	cfg        *config.Config
	store      *store.Store
	caw        *provider.CAWProvider
	fluxa      *provider.FluxAProvider
	reputation *provider.OnChainReputationProvider
	ipfs       *provider.IPFSProvider
	ruleEngine *rule.RuleEngine
	ruleVeto   *engine.RuleEvaluator
	ruleCfg    config.OpenAIConfig
	llmEngine  *engine.LLMEvaluator
	agg        *engine.Aggregator
	relayer    *relayer.Relayer
	sse        *SSEHub
	agent      *engine.AgentEngine
	log        *zap.Logger
	pipeline   sync.Map
}

func NewHandler(cfg *config.Config, s *store.Store,
	caw *provider.CAWProvider, fluxa *provider.FluxAProvider,
	reputation *provider.OnChainReputationProvider,
	ipfs *provider.IPFSProvider,
	ruleEngine *rule.RuleEngine, ruleVeto *engine.RuleEvaluator,
	llmEngine *engine.LLMEvaluator,
	agg *engine.Aggregator, relayer *relayer.Relayer,
	sse *SSEHub, log *zap.Logger,
) *Handler {
	return &Handler{
		cfg: cfg, store: s, caw: caw, fluxa: fluxa, reputation: reputation, ipfs: ipfs,
		ruleEngine: ruleEngine, ruleVeto: ruleVeto, ruleCfg: cfg.OpenAI,
		llmEngine: llmEngine, agg: agg,
		relayer: relayer, sse: sse,
		agent: engine.NewAgentEngine(llmEngine, cfg.OpenAI, log),
		log:   log,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/bounty", h.PostBounty)
	mux.HandleFunc("POST /api/bounty/{id}/claim", h.ClaimBounty)
	mux.HandleFunc("POST /api/bounty/{id}/sub-bounty", h.CreateSubBounty)
	mux.HandleFunc("POST /api/bounty/{id}/submit", h.SubmitResult)
	mux.HandleFunc("POST /api/confirm/{jobId}", h.ConfirmJob)
	mux.HandleFunc("POST /admin/retry/{jobId}", h.AdminRetry)
	mux.HandleFunc("GET /api/events", h.sse.SSEHandler)
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("GET /api/debug/ping-pact", h.DebugPingPact)
	mux.HandleFunc("GET /api/pact/{pactId}", h.GetPactStatus)
	mux.HandleFunc("POST /api/parse-intent", h.ParseIntent)
	mux.HandleFunc("POST /api/arbitrate/{jobId}", h.ArbitrateSlash)
	mux.HandleFunc("POST /api/debug/test-auto-chain", h.DebugTestAutoChain)
	mux.HandleFunc("GET /api/reputation/{address}", h.GetReputation)
	mux.HandleFunc("GET /api/bounty/{id}/pipeline", h.GetBountyPipeline)
	mux.HandleFunc("GET /api/agents", h.GetAgents)
}

// PipelineData stores the auto-chain pipeline state for real-time frontend display.
type PipelineData struct {
	Step                 string   `json:"step"`
	StepsReached         []string `json:"steps_reached,omitempty"`
	Reasoning            string   `json:"reasoning,omitempty"`
	SubDelivery          string  `json:"sub_delivery,omitempty"`
	FinalDelivery        string  `json:"final_delivery,omitempty"`
	EvalStatus           string  `json:"eval_status,omitempty"`
	EvalScore            float64 `json:"eval_score,omitempty"`
	EvalSummary          string  `json:"eval_summary,omitempty"`
	EvalRuleBreakdown    string  `json:"eval_rule_breakdown,omitempty"`
	EvalLLMScore         float64 `json:"eval_llm_score,omitempty"`
	EvalLLMReason        string  `json:"eval_llm_reason,omitempty"`
	SubEvalStatus        string  `json:"sub_eval_status,omitempty"`
	SubEvalScore         float64 `json:"sub_eval_score,omitempty"`
	SubEvalSummary       string  `json:"sub_eval_summary,omitempty"`
	SubEvalRuleBreakdown string  `json:"sub_eval_rule_breakdown,omitempty"`
	SubEvalLLMScore      float64 `json:"sub_eval_llm_score,omitempty"`
	SubEvalLLMReason     string  `json:"sub_eval_llm_reason,omitempty"`
	ParentTxHash         string  `json:"parent_tx_hash,omitempty"`
	ChildTxHash          string  `json:"child_tx_hash,omitempty"`
	ParentAmount         string  `json:"parent_amount,omitempty"`
	ChildAmount          string  `json:"child_amount,omitempty"`
}

// getEvalQualityScore retrieves the evaluation quality score from pipeline data.
// Returns 0.5 (neutral default) if no pipeline data is available.
func (h *Handler) getEvalQualityScore(jobID uint64) float64 {
	val, ok := h.pipeline.Load(jobID)
	if !ok {
		return 0.5
	}
	pd, ok := val.(*PipelineData)
	if !ok {
		return 0.5
	}
	return pd.EvalScore
}

// updatePipeline stores the current auto-chain step with optional reasoning/delivery data.
// Also tracks all steps reached for frontend polling catch-up.
func (h *Handler) updatePipeline(jobID uint64, step string, opts ...string) {
	val, _ := h.pipeline.Load(jobID)
	pd, ok := val.(*PipelineData)
	if !ok {
		pd = &PipelineData{}
	}
	// Track all steps reached (for frontend polling to catch up on missed steps)
	if step != pd.Step {
		pd.StepsReached = append(pd.StepsReached, step)
	}
	pd.Step = step
	// opts[0] = reasoning, opts[1] = delivery content (stored up to 500 chars)
	if len(opts) > 0 && opts[0] != "" {
		pd.Reasoning = opts[0]
	}
	if len(opts) > 1 && opts[1] != "" {
		content := opts[1]
		// Store full content (no truncation)
		// Determine if this is sub or final delivery based on step name
		if step == "generating_sub_delivery" {
			pd.SubDelivery = content
		} else if step == "submitted" || step == "generating_delivery" {
			pd.FinalDelivery = content
		}
	}
	// opts[2] = eval status, opts[3] = eval score as string, opts[4] = eval summary
	if len(opts) > 2 && opts[2] != "" {
		pd.EvalStatus = opts[2]
	}
	if len(opts) > 3 && opts[3] != "" {
		fmt.Sscanf(opts[3], "%f", &pd.EvalScore)
	}
	if len(opts) > 4 && opts[4] != "" {
		pd.EvalSummary = opts[4]
	}
	h.pipeline.Store(jobID, pd)
}

// retryOperation retries fn up to maxAttempts times with linear backoff.
// On each retry, pushes a "retrying" SSE event. Returns nil on success,
// or the last error after exhausting all attempts.
func (h *Handler) retryOperation(ctx context.Context, jID uint64, taskName string, maxAttempts int, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			sleep := time.Duration(attempt) * time.Second
			h.log.Warn("Task failed, retrying", zap.String("task", taskName), zap.Int("attempt", attempt+1), zap.Error(lastErr))
			h.sse.pushEvent("agent_action", jID, "retrying", "yellow",
				fmt.Sprintf(`{"agent":"system","action":"retrying","task":"%s","attempt":%d,"error":"%s"}`,
					taskName, attempt+1, lastErr.Error()))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sleep):
			}
		}
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// pushTaskFailed sends a final failure SSE for a task that exhausted retries.
func (h *Handler) pushTaskFailed(jID uint64, taskName string, err error) {
	h.sse.pushEvent("agent_action", jID, "failed", "red",
		fmt.Sprintf(`{"agent":"system","action":"failed","task":"%s","error":"%s"}`, taskName, err.Error()))
}

// ==================== PostBounty ====================

func (h *Handler) PostBounty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
	}
	ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)

	var req model.PostBountyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	bizID := sha256.Sum256([]byte(req.Buyer + "0" + nonce))
	bizIDStr := hex.EncodeToString(bizID[:])

	h.log.Info("PostBounty",
		zap.String("trace_id", traceID),
		zap.String("biz_id", bizIDStr),
		zap.String("buyer", req.Buyer),
		zap.String("amount", req.Amount),
		zap.Int("min_reputation", req.MinReputation),
	)

	var deadlineSeconds uint64 = 2592000 // 30 day default
	if req.Deadline != "" {
		// Try multiple formats: RFC3339, datetime-local (no seconds), date-only
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05",
			"2006-01-02T15:04",
			"2006-01-02",
		}
		var parsedTime time.Time
		for _, f := range formats {
			if t, err := time.Parse(f, req.Deadline); err == nil {
				parsedTime = t
				break
			}
		}
		if !parsedTime.IsZero() {
			// Treat as UTC (frontend sends local time without timezone)
			if remaining := time.Until(parsedTime); remaining > 0 {
				deadlineSeconds = uint64(remaining.Seconds())
			}
		}
	}

	pact, err := h.caw.CreatePact(ctx, bizIDStr, h.cfg.CAW.WalletID, h.cfg.CAW.BuyerAddr, req.Amount, "BASE_ETH", req.Intent, deadlineSeconds, req.MinReputation)
	if err != nil {
		h.log.Error("CAW CreatePact failed", zap.String("trace_id", traceID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "CAW pact creation failed: "+err.Error())
		return
	}

	jobID := uint64(time.Now().UnixMilli())
	bounty := &model.Bounty{
		JobID: jobID, BuyerAddr: h.cfg.CAW.BuyerAddr, Status: model.StatusOpen,
		PactID: pact.PactID, Amount: req.Amount,
		BuyerWalletID: h.cfg.CAW.WalletID,
	}
	if req.Deadline != "" {
		if t, err := time.Parse(time.RFC3339, req.Deadline); err == nil {
			bounty.Deadline = t
		}
	}
	if err := h.store.CreateBounty(ctx, bounty); err != nil {
		h.log.Error("DB CreateBounty failed", zap.String("trace_id", traceID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	if req.Intent != "" {
		go func() {
			ruleParams := rule.ExtractRuleParams(context.Background(), req.Intent, h.ruleCfg, h.log)
			if ruleParams != "" {
				h.store.UpdateBountyRuleParams(context.Background(), jobID, ruleParams)
			}
		}()
	}

	if pact != nil {
		h.sse.pushEvent("awaiting_cobo_approval", jobID, pact.Status, "yellow",
			fmt.Sprintf(`{"pact_id":"%s","amount":"%s","type":"lock"}`, pact.PactID, req.Amount))

		go func(pactID string, jID uint64, intent string, amount string, buyerAddr string, demoSlash bool) {
			pollCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			h.log.Info("Goroutine started", zap.Uint64("job_id", jID), zap.String("pact_id", pactID))

			approved := false
			for !approved {
				select {
				case <-pollCtx.Done():
					h.log.Warn("Pact polling timed out", zap.String("pact_id", pactID), zap.Uint64("job_id", jID))
					return
				case <-ticker.C:
					status, err := h.caw.GetPactStatus(pollCtx, pactID)
					if err != nil {
						continue
					}
					if status.Status == "rejected" {
						h.log.Warn("Pact rejected in CAW App", zap.String("pact_id", pactID), zap.Uint64("job_id", jID))
						h.sse.pushEvent("pact_rejected", jID, "rejected", "red", fmt.Sprintf(`{"pact_id":"%s"}`, pactID))
						return
					}
					if status.Status == "active" {
						h.log.Info("Pact approved in CAW App", zap.String("pact_id", pactID), zap.Uint64("job_id", jID))
						h.sse.pushEvent("pact_approved", jID, "active", "green", fmt.Sprintf(`{"pact_id":"%s"}`, pactID))
						h.updatePipeline(jID, "pact_approved")
						approved = true
					}
				}
			}
			ticker.Stop()

			// Step 2: Auto-claim
			h.log.Info("Provider auto-claiming bounty", zap.Uint64("job_id", jID))
			h.sse.pushEvent("agent_action", jID, "claiming", "yellow", fmt.Sprintf(`{"agent":"provider","action":"claiming","job_id":%d}`, jID))
			h.updatePipeline(jID, "claiming")
			if err := h.autoClaim(pollCtx, jID); err != nil {
				h.log.Warn("Auto-claim failed", zap.Uint64("job_id", jID), zap.Error(err))
				h.sse.pushEvent("agent_action", jID, "claim_failed", "red", fmt.Sprintf(`{"agent":"provider","action":"claim_failed","error":"%s"}`, err.Error()))
				return
			}
			h.sse.pushEvent("agent_action", jID, "claimed", "yellow", fmt.Sprintf(`{"agent":"provider","action":"claimed","job_id":%d}`, jID))
			h.updatePipeline(jID, "claimed")

			// Step 3: LLM analyze
			h.sse.pushEvent("agent_thinking", jID, "analyzing", "purple", fmt.Sprintf(`{"agent":"provider","step":"analyzing task: %s"}`, intent))
			h.updatePipeline(jID, "analyzing")
			var decision *engine.AgentDecision
			if err := h.retryOperation(pollCtx, jID, "analyze_task", 3, func() error {
				var dErr error
				decision, dErr = h.agent.AnalyzeTask(pollCtx, intent, amount)
				return dErr
			}); err != nil {
				h.log.Warn("Agent analysis failed after retries", zap.Uint64("job_id", jID), zap.Error(err))
				h.pushTaskFailed(jID, "analyze_task", err)
				return
			}
			h.sse.pushEvent("agent_decided", jID, "decided", "purple", fmt.Sprintf(`{"agent":"provider","decision":"%v","reasoning":"%s"}`, decision.NeedsSubBounty, decision.Reasoning))
			h.updatePipeline(jID, "decided", decision.Reasoning)

			if decision.NeedsSubBounty && decision.SubDescription != "" {
				h.log.Info("Provider creating sub-bounty", zap.Uint64("job_id", jID), zap.String("desc", decision.SubDescription))
				h.sse.pushEvent("agent_action", jID, "creating_sub_bounty", "yellow", fmt.Sprintf(`{"agent":"provider","action":"creating_sub_bounty","description":"%s"}`, decision.SubDescription))
				h.updatePipeline(jID, "creating_sub_bounty")

				subAmountWei := "5000000000000000"
				if decision.SubAmount != "" {
					subAmountWei = decision.SubAmount
				}
				subJobID := uint64(time.Now().UnixMilli())
				subBounty := &model.Bounty{
					JobID: subJobID, BuyerAddr: h.cfg.CAW.ProviderAddr, SellerAddr: "",
					Amount: subAmountWei, Deadline: time.Now().Add(7 * 24 * time.Hour),
					Status: model.StatusOpen, PactID: "", ParentBountyID: &jID, Depth: 1,
				}
				if err := h.retryOperation(pollCtx, jID, "create_sub_bounty", 3, func() error {
					return h.store.CreateSubBounty(pollCtx, subBounty)
				}); err != nil {
					h.log.Warn("Failed to create sub-bounty after retries", zap.Error(err))
					h.pushTaskFailed(jID, "create_sub_bounty", err)
					return
				}
				h.sse.pushEvent("bounty_posted", subJobID, string(model.StatusOpen), "blue", fmt.Sprintf("Sub-bounty #%d created under #%d", subJobID, jID))

				// Sub-Provider auto-claim
				h.sse.pushEvent("agent_action", subJobID, "claiming", "yellow", fmt.Sprintf(`{"agent":"sub_provider","action":"claiming","job_id":%d}`, subJobID))
				h.updatePipeline(jID, "sub_claiming")
				subClaimCtx, subCancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, subClaimErr := h.store.ClaimBountyWithLock(subClaimCtx, subJobID, h.cfg.CAW.SubProviderAddr)
				subCancel()
				if subClaimErr != nil {
					h.log.Warn("Sub-Provider auto-claim failed", zap.Uint64("sub_job_id", subJobID), zap.Error(subClaimErr))
					h.sse.pushEvent("agent_action", subJobID, "claim_failed", "red", fmt.Sprintf(`{"agent":"sub_provider","action":"claim_failed","error":"%s"}`, subClaimErr.Error()))
					return
				}
				h.sse.pushEvent("agent_action", subJobID, "claimed", "green", fmt.Sprintf(`{"agent":"sub_provider","action":"claimed","job_id":%d}`, subJobID))
				h.updatePipeline(jID, "sub_claimed")

				// Generate sub-delivery
				h.sse.pushEvent("agent_thinking", subJobID, "generating_delivery", "purple", `{"agent":"sub_provider","step":"generating delivery for sub-task"}`)
				var subDelivery string
				if err := h.retryOperation(pollCtx, jID, "generate_sub_delivery", 3, func() error {
					var gErr error
					subDelivery, gErr = h.agent.GenerateSubDelivery(pollCtx, intent, decision.SubDescription)
					return gErr
				}); err != nil {
					h.log.Warn("Sub-Provider delivery generation failed after retries", zap.Error(err))
					h.pushTaskFailed(jID, "generate_sub_delivery", err)
					return
				}
				h.updatePipeline(jID, "generating_sub_delivery", "", subDelivery)

				var subCID string
				if err := h.retryOperation(pollCtx, jID, "ipfs_upload_sub", 3, func() error {
					var iErr error
					subCID, iErr = h.ipfs.UploadResult(pollCtx, []byte(subDelivery))
					return iErr
				}); err != nil {
					h.log.Warn("Sub-Provider IPFS upload failed after retries", zap.Error(err))
					h.pushTaskFailed(jID, "ipfs_upload_sub", err)
					return
				}

				// Evaluate sub-bounty
				h.sse.pushEvent("agent_action", subJobID, "submitting", "yellow", `{"agent":"sub_provider","action":"submitting_delivery"}`)
				h.sse.pushEvent("evaluation_started", subJobID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating sub-bounty #%d", subJobID))
				h.updatePipeline(jID, "evaluating_sub")
				// Init pipeline for subJobID so evaluateDelivery can store details
				h.pipeline.Store(subJobID, &PipelineData{Step: "evaluating_sub"})
				subDeliveryContent := &engine.DeliveryContent{
					JobID: subJobID, Seller: h.cfg.CAW.SubProviderAddr, ResultHash: subCID, RawData: subDelivery,
				}
				var subVerdict *engine.FinalVerdict
				if err := h.retryOperation(pollCtx, jID, "evaluate_sub_bounty", 3, func() error {
					var eErr error
					subVerdict, eErr = h.evaluateDelivery(pollCtx, subBounty, subDeliveryContent, false)
					return eErr
				}); err != nil {
					h.log.Warn("Sub-bounty evaluation failed after retries", zap.Error(err))
					h.pushTaskFailed(jID, "evaluate_sub_bounty", err)
					return
				}
				colorMap := map[string]string{"verified": "green", "slashed": "red"}
				subColor, _ := colorMap[subVerdict.Status]
				h.sse.pushEvent("evaluation_result", subJobID, subVerdict.Status, subColor, fmt.Sprintf(`{"status":"%s","score":%.2f,"summary":"%s"}`, subVerdict.Status, subVerdict.Score, subVerdict.Summary))

				// Copy sub-evaluation details into main pipeline's SubEval fields
				if subVal, ok := h.pipeline.Load(subJobID); ok {
					if subPd, ok := subVal.(*PipelineData); ok {
						if mainVal, ok := h.pipeline.Load(jID); ok {
							if mainPd, ok := mainVal.(*PipelineData); ok {
								mainPd.SubEvalStatus = subVerdict.Status
								mainPd.SubEvalScore = subVerdict.Score
								mainPd.SubEvalSummary = subVerdict.Summary
								mainPd.SubEvalRuleBreakdown = subPd.EvalRuleBreakdown
								mainPd.SubEvalLLMScore = subPd.EvalLLMScore
								mainPd.SubEvalLLMReason = subPd.EvalLLMReason
								h.pipeline.Store(jID, mainPd)
							}
						}
					}
				}

			if subVerdict.Status != "verified" {
				h.log.Warn("Sub-bounty evaluation not passed, aborting chain", zap.String("status", subVerdict.Status))
				h.pushTaskFailed(jID, "sub_evaluation_failed", fmt.Errorf("sub-bounty %s (score: %.2f)", subVerdict.Status, subVerdict.Score))
				return
			}
				h.updatePipeline(jID, "sub_verified")

			// Provider merges or generates failed delivery
			h.sse.pushEvent("agent_thinking", jID, "merging", "purple", `{"agent":"provider","step":"merging sub-provider delivery with own analysis"}`)
			var finalDelivery string
			if err := h.retryOperation(pollCtx, jID, "generate_final_delivery", 3, func() error {
				var mErr error
				if demoSlash {
					h.log.Info("DEMO SLASH: generating intentionally poor delivery", zap.Uint64("job_id", jID))
					h.sse.pushEvent("agent_action", jID, "generating_failed_delivery", "red", fmt.Sprintf(`{"agent":"provider","action":"仲裁演示: 生成低质量交付物"}`))
					finalDelivery, mErr = h.agent.GenerateFailedDelivery(pollCtx, intent)
				} else {
					finalDelivery, mErr = h.agent.MergeDeliveries(pollCtx, intent, subDelivery)
				}
				return mErr
			}); err != nil {
				h.log.Warn("Provider merge/generation failed after retries", zap.Error(err))
				h.pushTaskFailed(jID, "generate_final_delivery", err)
				return
			}

			var finalCID string
			if err := h.retryOperation(pollCtx, jID, "ipfs_upload_final", 3, func() error {
				var fiErr error
				finalCID, fiErr = h.ipfs.UploadResult(pollCtx, []byte(finalDelivery))
				return fiErr
			}); err != nil {
				h.log.Warn("Provider final IPFS upload failed after retries", zap.Error(err))
				h.pushTaskFailed(jID, "ipfs_upload_final", err)
				return
			}

				h.store.UpdateBountyStatus(pollCtx, jID, model.StatusSubmitted)
				h.sse.pushEvent("agent_action", jID, "submitted", "yellow", `{"agent":"provider","action":"final_delivery_submitted"}`)
				h.updatePipeline(jID, "submitted", "", finalDelivery)

				// Evaluate final delivery
			h.sse.pushEvent("evaluation_started", jID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating final delivery for bounty #%d", jID))
			h.updatePipeline(jID, "evaluating_final")
			mainBounty, mainErr := h.store.GetBountyByID(pollCtx, jID)
			if mainErr != nil {
				h.log.Warn("Failed to get main bounty for evaluation", zap.Error(mainErr))
				h.pushTaskFailed(jID, "get_main_bounty", mainErr)
				return
			}
			finalDeliveryContent := &engine.DeliveryContent{
				JobID: jID, Seller: h.cfg.CAW.ProviderAddr, ResultHash: finalCID, RawData: finalDelivery,
			}
			var finalVerdict *engine.FinalVerdict
			if err := h.retryOperation(pollCtx, jID, "evaluate_final_delivery", 3, func() error {
				var feErr error
				finalVerdict, feErr = h.evaluateDelivery(pollCtx, mainBounty, finalDeliveryContent, true)
				return feErr
			}); err != nil {
				h.log.Warn("Final delivery evaluation failed after retries", zap.Error(err))
				h.pushTaskFailed(jID, "evaluate_final_delivery", err)
				return
			}
				finalColor, _ := colorMap[finalVerdict.Status]
				h.sse.pushEvent("evaluation_result", jID, finalVerdict.Status, finalColor, fmt.Sprintf(`{"status":"%s","score":%.2f,"summary":"%s"}`, finalVerdict.Status, finalVerdict.Score, finalVerdict.Summary))
				h.updatePipeline(jID, "evaluated_"+finalVerdict.Status, "", "", finalVerdict.Status, fmt.Sprintf("%.2f", finalVerdict.Score), finalVerdict.Summary)

				if finalVerdict.Status == "verified" {
					amountWei := weiFromStr(amount)
					amountEth := float64(amountWei) / 1e18
					if amountEth >= 0.01 {
						h.sse.pushEvent("high_value_approval_required", jID, "PendingApproval", "yellow", fmt.Sprintf(`{"job_id":%d,"amount":"%.4f","type":"release"}`, jID, amountEth))
					}
					h.log.Info("Full auto-chain complete — awaiting buyer confirmation", zap.Uint64("job_id", jID))
					h.sse.pushEvent("awaiting_confirmation", jID, "Verified", "yellow", fmt.Sprintf("Awaiting buyer confirmation for bounty #%d", jID))
					h.updatePipeline(jID, "awaiting_confirmation")
				}
			}

			if !decision.NeedsSubBounty {
				h.log.Info("Provider handling task directly — auto-submitting", zap.Uint64("job_id", jID))
				h.sse.pushEvent("agent_thinking", jID, "generating_delivery", "purple", fmt.Sprintf(`{"agent":"provider","step":"generating delivery for task: %s"}`, intent))
				var directDelivery string
				if err := h.retryOperation(pollCtx, jID, "generate_direct_delivery", 3, func() error {
					var dErr error
					directDelivery, dErr = h.agent.GenerateSubDelivery(pollCtx, intent, intent)
					return dErr
				}); err != nil {
					h.log.Warn("Direct delivery generation failed after retries", zap.Error(err))
					h.pushTaskFailed(jID, "generate_direct_delivery", err)
					return
				}
				h.updatePipeline(jID, "generating_delivery", "", directDelivery)
				var directCID string
				if err := h.retryOperation(pollCtx, jID, "ipfs_upload_direct", 3, func() error {
					var diErr error
					directCID, diErr = h.ipfs.UploadResult(pollCtx, []byte(directDelivery))
					return diErr
				}); err != nil {
					h.log.Warn("Direct IPFS upload failed after retries", zap.Error(err))
					h.pushTaskFailed(jID, "ipfs_upload_direct", err)
					return
				}
				h.store.UpdateBountyStatus(pollCtx, jID, model.StatusSubmitted)
				h.sse.pushEvent("agent_action", jID, "submitted", "yellow", `{"agent":"provider","action":"delivery_submitted"}`)
				h.sse.pushEvent("evaluation_started", jID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating delivery for bounty #%d", jID))
				mainBounty, mainErr := h.store.GetBountyByID(pollCtx, jID)
				if mainErr != nil {
					h.log.Warn("Failed to get main bounty for direct evaluation", zap.Error(mainErr))
					h.pushTaskFailed(jID, "get_main_bounty_direct", mainErr)
					return
				}
				directDeliveryContent := &engine.DeliveryContent{
					JobID: jID, Seller: h.cfg.CAW.ProviderAddr, ResultHash: directCID, RawData: directDelivery,
				}
				var directVerdict *engine.FinalVerdict
				if err := h.retryOperation(pollCtx, jID, "evaluate_direct_delivery", 3, func() error {
					var deErr error
					directVerdict, deErr = h.evaluateDelivery(pollCtx, mainBounty, directDeliveryContent, true)
					return deErr
				}); err != nil {
					h.log.Warn("Direct delivery evaluation failed after retries", zap.Error(err))
					h.pushTaskFailed(jID, "evaluate_direct_delivery", err)
					return
				}
				colorMap := map[string]string{"verified": "green", "slashed": "red"}
				directColor, _ := colorMap[directVerdict.Status]
				h.sse.pushEvent("evaluation_result", jID, directVerdict.Status, directColor, fmt.Sprintf(`{"status":"%s","score":%.2f,"summary":"%s"}`, directVerdict.Status, directVerdict.Score, directVerdict.Summary))
				if directVerdict.Status == "verified" {
					amountWei := weiFromStr(amount)
					amountEth := float64(amountWei) / 1e18
					if amountEth >= 0.01 {
						h.sse.pushEvent("high_value_approval_required", jID, "PendingApproval", "yellow", fmt.Sprintf(`{"job_id":%d,"amount":"%.4f","type":"release"}`, jID, amountEth))
					}
					h.log.Info("Direct submission complete — awaiting buyer confirmation", zap.Uint64("job_id", jID))
					h.updatePipeline(jID, "awaiting_confirmation")
				}
			}
		}(pact.PactID, jobID, req.Intent, req.Amount, req.Buyer, req.DemoSlash)
	}

	writeJSON(w, http.StatusOK, model.PostBountyResponse{
		JobID: jobID, PactID: pact.PactID,
		PactStatus: pact.Status, Status: string(model.StatusOpen),
	})
	h.sse.pushEvent("bounty_posted", jobID, string(model.StatusOpen), "blue", fmt.Sprintf(`{"pact_id":"%s","main":true}`, pact.PactID))
}

// ==================== ClaimBounty ====================

func (h *Handler) ClaimBounty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := r.Header.Get("X-Trace-ID")
	ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)

	jobIDStr := r.PathValue("id")
	if jobIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing job id")
		return
	}
	var jobID uint64
	fmt.Sscanf(jobIDStr, "%d", &jobID)

	var req struct {
		Seller string `json:"seller"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Seller == "" {
		writeError(w, http.StatusBadRequest, "seller address required")
		return
	}

	locked, err := h.store.TryAcquireClaimLock(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !locked {
		writeError(w, http.StatusConflict, "bounty already claimed or being claimed")
		return
	}
	defer h.store.ReleaseClaimLock(ctx, jobID)

	if h.reputation != nil {
		if result, err := h.reputation.CheckReputation(ctx, req.Seller); err == nil && !result.Passed {
			writeError(w, http.StatusForbidden, fmt.Sprintf("provider reputation too low (score: %d)", result.Score))
			return
		}
	}

	fluxaResult, err := h.fluxa.CheckReputation(ctx, req.Seller)
	if err != nil {
		fluxaResult = &provider.FluxAResult{Passed: true, FallbackUsed: true, Score: 0}
	}
	if !fluxaResult.Passed {
		writeError(w, http.StatusForbidden, "provider reputation check failed")
		return
	}

	if _, err := h.store.ClaimBountyWithLock(ctx, jobID, req.Seller); err != nil {
		writeError(w, http.StatusConflict, "bounty already assigned or invalid")
		return
	}

	h.sse.pushEvent("claimed", jobID, "Assigned", "yellow", "Bounty claimed by seller")
	writeJSON(w, http.StatusOK, model.ClaimBountyResponse{JobID: jobID, Seller: req.Seller, Status: "Assigned"})
}

// ==================== CreateSubBounty ====================

func (h *Handler) CreateSubBounty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	parentIDStr := r.PathValue("id")
	var parentID uint64
	fmt.Sscanf(parentIDStr, "%d", &parentID)

	var req model.SubBountyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	parent, err := h.store.GetBountyByID(ctx, parentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "parent bounty not found")
		return
	}
	if parent.Status != model.StatusAssigned {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("parent not in Assigned (current: %s)", parent.Status))
		return
	}
	if req.Seller != string(parent.SellerAddr) {
		writeError(w, http.StatusForbidden, "only assigned provider can create sub-bounties")
		return
	}

	jobID := uint64(time.Now().UnixMilli())
	subBounty := &model.Bounty{
		JobID: jobID, BuyerAddr: parent.SellerAddr, SellerAddr: "",
		Amount: req.Amount, Deadline: parent.Deadline, Status: model.StatusOpen,
		PactID: "", ParentBountyID: &parentID, Depth: parent.Depth + 1, BuyerWalletID: req.WalletID,
	}
	if err := h.store.CreateSubBounty(ctx, subBounty); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	h.sse.pushEvent("bounty_posted", jobID, string(model.StatusOpen), "blue", fmt.Sprintf("Sub-bounty created under #%d", parentID))
	writeJSON(w, http.StatusOK, model.SubBountyResponse{JobID: jobID, ParentID: parentID, Depth: parent.Depth + 1, Amount: req.Amount, Status: string(model.StatusOpen)})
}

// ==================== SubmitResult ====================

func (h *Handler) SubmitResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
	}
	ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)

	jobIDStr := r.PathValue("id")
	var jobID uint64
	fmt.Sscanf(jobIDStr, "%d", &jobID)

	var req struct {
		Seller string `json:"seller"`
		Data   string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	bounty, err := h.store.GetBountyByID(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "bounty not found")
		return
	}
	if string(bounty.Status) != string(model.StatusAssigned) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("bounty not in Assigned (current: %s)", bounty.Status))
		return
	}

	cid, err := h.ipfs.UploadResult(ctx, []byte(req.Data))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ipfs upload failed")
		return
	}

	h.sse.pushEvent("evaluation_started", jobID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating delivery for job #%d", jobID))
	delivery := &engine.DeliveryContent{JobID: jobID, Seller: req.Seller, ResultHash: cid, RawData: req.Data}
	ruleResults := h.ruleEngine.Evaluate(req.Data, bounty.RuleParams)
	ruleResult := h.ruleVeto.Evaluate(ctx, delivery)
	llmResult, llmErr := h.llmEngine.Evaluate(ctx, delivery)
	verdict := h.agg.Aggregate(ruleResult, llmResult, llmErr)
	h.log.Info("Evaluation complete", zap.Uint64("job_id", jobID), zap.String("status", verdict.Status), zap.Float64("score", verdict.Score), zap.String("summary", verdict.Summary))

	if verdict.Status == "verified" {
		amountWei := weiFromStr(bounty.Amount)
		amountEth := float64(amountWei) / 1e18
		if amountEth >= 0.01 {
			h.sse.pushEvent("high_value_approval_required", jobID, "PendingApproval", "yellow", fmt.Sprintf(`{"job_id":%d,"amount":"%.4f","type":"release"}`, jobID, amountEth))
		}
	}
	if verdict.Status == "slashed" {
		h.store.UpdateBountyStatus(ctx, jobID, model.StatusDisputed)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "evaluated", "cid": cid, "verdict": verdict, "rule_results": ruleResults})
	h.sse.pushEvent("submitted", jobID, verdict.Status, map[string]string{"verified": "green", "slashed": "red", "pending_review": "yellow"}[verdict.Status], fmt.Sprintf("Delivery evaluated: %s", verdict.Summary))
}

// ==================== ConfirmJob ====================

func (h *Handler) ConfirmJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
	}
	ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)

	var jobID uint64
	fmt.Sscanf(r.PathValue("jobId"), "%d", &jobID)

	if err := h.store.ConfirmBuyerApproval(ctx, jobID); err != nil {
		h.log.Error("Failed to set BuyerApproval", zap.Uint64("job_id", jobID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to confirm")
		return
	}
	h.log.Info("UserConfirmed", zap.String("trace_id", traceID), zap.Uint64("job_id", jobID))

	// Update reputation immediately — doesn't depend on CAW transfer
	go func(jID uint64) {
		repCtx := context.Background()
		b, _ := h.store.GetBountyByID(repCtx, jID)
		if b == nil || b.SellerAddr == "" {
			return
		}
		repResult, _ := h.reputation.CheckReputation(repCtx, string(b.SellerAddr))
		currentScore := 50.0
		if repResult != nil {
			currentScore = float64(repResult.Score)
		}
		taskInfo := reputation.TaskInfo{QualityScore: h.getEvalQualityScore(jID), AmountWei: weiFromStr(b.Amount), SubmittedAt: time.Now().Unix(), Deadline: b.Deadline.Unix()}
		newScore := reputation.CalculateNewScore(currentScore, taskInfo, 0)
		paramHash := reputation.ComputeParamHash(true, taskInfo.AmountWei, uint64(taskInfo.SubmittedAt), uint64(taskInfo.Deadline))
		delta := newScore - currentScore
		if delta != 0 {
			reason := "delivery_passed"
			if delta < 0 {
				reason = "delivery_adjusted"
			}
			h.sse.pushEvent("reputation_changed", jID, "Verified", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%f,"taskId":"#%d","reason":"%s"}`, string(b.SellerAddr), currentScore, newScore, delta, jID, reason))
			updateCtx, updateCancel := context.WithTimeout(repCtx, 15*time.Second)
			if txHash, _ := h.reputation.UpdateReputation(updateCtx, string(b.SellerAddr), jID, uint64(newScore), int64(delta), reason, paramHash); txHash != "" {
				h.sse.pushEvent("reputation_updated", jID, "onchain", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%f,"txHash":"%s"}`, string(b.SellerAddr), currentScore, newScore, delta, txHash))
			}
			updateCancel()
		}
		// Sub-Provider reputation (with timeout)
		children, _ := h.store.GetChildrenBounties(repCtx, jID)
		for _, child := range children {
			if child.SellerAddr == "" {
				continue
			}
			childRep, _ := h.reputation.CheckReputation(repCtx, string(child.SellerAddr))
			ccs := 50.0
			if childRep != nil {
				ccs = float64(childRep.Score)
			}
			cti := reputation.TaskInfo{QualityScore: h.getEvalQualityScore(child.JobID), AmountWei: weiFromStr(child.Amount), SubmittedAt: time.Now().Unix(), Deadline: child.Deadline.Unix()}
			cns := reputation.CalculateNewScore(ccs, cti, 0)
			cph := reputation.ComputeParamHash(true, cti.AmountWei, uint64(cti.SubmittedAt), uint64(cti.Deadline))
			cd := cns - ccs
			if cd != 0 {
				cReason := "sub_bounty_settled"
				if cd < 0 {
					cReason = "sub_bounty_adjusted"
				}
				h.sse.pushEvent("reputation_changed", child.JobID, "Verified", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%f,"taskId":"#%d","reason":"%s"}`, string(child.SellerAddr), ccs, cns, cd, child.JobID, cReason))
				// Non-blocking on-chain update with 15s timeout
				updateCtx, updateCancel := context.WithTimeout(repCtx, 15*time.Second)
				if txHash, _ := h.reputation.UpdateReputation(updateCtx, string(child.SellerAddr), child.JobID, uint64(cns), int64(cd), cReason, cph); txHash != "" {
					h.sse.pushEvent("reputation_updated", child.JobID, "onchain", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%f,"txHash":"%s"}`, string(child.SellerAddr), ccs, cns, cd, txHash))
				}
				updateCancel()
			}
		}
		h.updatePipeline(jID, "settled")
		h.sse.pushEvent("settled", jID, "Verified", "green", "Reputation updated, settlement in progress")
	}(jobID)

	// Submit CAW transfer in background
	go func(jID uint64) {
		settleCtx := context.Background()
		result := h.relayer.SettleBountyTree(settleCtx, jID, traceID)
		// Get amounts for display
		parentAmount := ""
		childAmount := ""
		if b, _ := h.store.GetBountyByID(settleCtx, jID); b != nil && b.Amount != "" {
			if wei, err := strconv.ParseUint(b.Amount, 10, 64); err == nil {
				parentAmount = fmt.Sprintf("%.4f", float64(wei)/1e18)
			}
		}
		children, _ := h.store.GetChildrenBounties(settleCtx, jID)
		if len(children) > 0 && children[0].Amount != "" {
			if wei, err := strconv.ParseUint(children[0].Amount, 10, 64); err == nil {
				childAmount = fmt.Sprintf("%.4f", float64(wei)/1e18)
			}
		}
		// Child transfer: push immediately if tx hash available
		if result.Success && result.ChildTxHash != "" {
			h.sse.pushEvent("transfer_completed", jID, "settled", "green",
				fmt.Sprintf(`{"from":"provider","to":"sub_provider","txHash":"%s","amount":"%s"}`, result.ChildTxHash, childAmount))
		}
		// Parent transfer: push amount immediately even if tx hash not yet available
		// (MPC paired wallet requires human CAW App approval — tx hash may lag)
		if result.Success {
			parentTxHash := result.ParentTxHash
			if parentTxHash == "" {
				parentTxHash = result.TransferTxID // fallback: show transfer request ID
			}
			h.sse.pushEvent("transfer_completed", jID, "settled", "green",
				fmt.Sprintf(`{"from":"buyer","to":"provider","txHash":"%s","amount":"%s"}`, parentTxHash, parentAmount))
			// If tx hash not available yet, poll for it in background
			if result.ParentTxHash == "" && result.TransferTxID != "" {
				go func(txID, walletID string) {
					ticker := time.NewTicker(3 * time.Second)
					defer ticker.Stop()
					deadline := time.After(5 * time.Minute)
					for {
						select {
						case <-deadline:
							return
						case <-ticker.C:
							_, txHash, err := h.caw.GetTransactionByRequestID(context.Background(), walletID, txID)
							if err == nil && txHash != "" {
								h.sse.pushEvent("transfer_completed", jID, "settled", "green",
									fmt.Sprintf(`{"from":"buyer","to":"provider","txHash":"%s","amount":"%s"}`, txHash, parentAmount))
								return
							}
						}
					}
				}(result.TransferTxID, h.cfg.CAW.WalletID)
			}
		}
		if !result.Success {
			h.log.Warn("Relayer: settlement failed", zap.Uint64("job_id", jID), zap.String("status", result.Status), zap.String("message", result.Message))
		}
		// Store tx hashes + amounts in pipeline data for frontend polling to read
		if result.Success {
			if val, ok := h.pipeline.Load(jID); ok {
				if pd, ok := val.(*PipelineData); ok {
					if result.ChildTxHash != "" {
						pd.ChildTxHash = result.ChildTxHash
						pd.ChildAmount = childAmount
					}
					pd.ParentAmount = parentAmount
					if result.ParentTxHash != "" {
						pd.ParentTxHash = result.ParentTxHash
					} else if result.TransferTxID != "" {
						pd.ParentTxHash = result.TransferTxID
					}
					h.pipeline.Store(jID, pd)
				}
			}
		}
	}(jobID)

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "confirmed", "settlement": "processing"})
}

// ==================== AdminRetry ====================

func (h *Handler) AdminRetry(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Demo-Admin-Token")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing admin token")
		return
	}
	ctx := r.Context()
	var jobID uint64
	fmt.Sscanf(r.PathValue("jobId"), "%d", &jobID)
	result := h.relayer.SettleBounty(ctx, jobID, fmt.Sprintf("admin_%d", time.Now().UnixNano()))
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "retry_attempted", "settlement": result.Status, "message": result.Message})
}

// ==================== Health / Pact / Parse / Arbitrate ====================

// GetAgents returns the configured agent wallet info for the frontend.
func (h *Handler) GetAgents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"buyer": map[string]string{
			"address":   h.cfg.CAW.BuyerAddr,
			"wallet_id": h.cfg.CAW.WalletID,
		},
		"provider": map[string]string{
			"address": h.cfg.CAW.ProviderAddr,
		},
		"sub_provider": map[string]string{
			"address": h.cfg.CAW.SubProviderAddr,
		},
	})
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetPactStatus(w http.ResponseWriter, r *http.Request) {
	pactID := r.PathValue("pactId")
	if pactID == "" {
		writeError(w, http.StatusBadRequest, "missing pact id")
		return
	}
	status, err := h.caw.GetPactStatus(r.Context(), pactID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pact status check failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"pact_id": pactID, "status": status.Status})
}

func (h *Handler) ParseIntent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		writeError(w, http.StatusBadRequest, "missing text")
		return
	}
	result := rule.ExtractRuleParams(r.Context(), req.Text, h.ruleCfg, h.log)

	// Generate a refined title from the input — use full text, no truncation
	title := req.Text

	response := map[string]interface{}{"title": title, "intent": req.Text, "rule_params": result, "source": "keyword"}
	if result != "" {
		response["source"] = "llm"
		var parsed struct {
			AmountEth    float64 `json:"suggested_amount_eth"`
			DeadlineDays int     `json:"suggested_deadline_days"`
			MinRep       int     `json:"suggested_min_reputation"`
		}
		if err := json.Unmarshal([]byte(result), &parsed); err == nil {
			if parsed.AmountEth > 0 {
				response["suggested_amount_eth"] = parsed.AmountEth
			}
			if parsed.DeadlineDays > 0 {
				response["suggested_deadline_days"] = parsed.DeadlineDays
			}
			if parsed.MinRep > 0 {
				response["suggested_min_reputation"] = parsed.MinRep
			}
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) ArbitrateSlash(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var jobID uint64
	fmt.Sscanf(r.PathValue("jobId"), "%d", &jobID)

	bounty, err := h.store.GetBountyByID(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "bounty not found")
		return
	}
	if bounty.Status != model.StatusDisputed {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("bounty not in Disputed (current: %s)", bounty.Status))
		return
	}

	h.store.UpdateBountyStatus(ctx, jobID, model.StatusSlashed)

	go func() {
		repCtx := context.Background()
		seller := string(bounty.SellerAddr)
		repResult, _ := h.reputation.CheckReputation(repCtx, seller)
		currentScore := 50.0
		if repResult != nil {
			currentScore = float64(repResult.Score)
		}
		newScore, penalty := reputation.CalculateSlash(currentScore)
		taskInfo := reputation.TaskInfo{QualityScore: 0, AmountWei: weiFromStr(bounty.Amount), SubmittedAt: time.Now().Unix(), Deadline: bounty.Deadline.Unix()}
		paramHash := reputation.ComputeParamHash(false, taskInfo.AmountWei, uint64(taskInfo.SubmittedAt), uint64(taskInfo.Deadline))
		h.log.Info("Reputation slash after arbitration", zap.String("seller", seller), zap.Uint64("job_id", jobID), zap.Float64("current", currentScore), zap.Float64("new", newScore), zap.Float64("penalty", penalty))
		h.reputation.UpdateReputation(repCtx, seller, jobID, uint64(newScore), int64(-penalty), "arbitration_slashed", paramHash)
		h.sse.pushEvent("reputation_changed", jobID, "Slashed", "red", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"taskId":"#%d","reason":"arbitration_slashed"}`, seller, currentScore, newScore, -penalty, jobID))
	}()

	h.sse.pushEvent("slashed", jobID, "Slashed", "red", fmt.Sprintf("Arbitration confirmed fraud for job #%d", jobID))
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "slashed", "message": "arbitration confirmed, reputation penalized"})
}

// ==================== Reputation API ====================

func (h *Handler) GetReputation(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("address")
	if addr == "" {
		writeError(w, http.StatusBadRequest, "missing address")
		return
	}
	result, err := h.reputation.CheckReputation(r.Context(), addr)
	score := 50
	fallback := true
	if err == nil {
		score = result.Score
		fallback = result.FallbackUsed
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"address": addr, "score": score, "fallback_used": fallback})
}

// GetBountyPipeline returns the current auto-chain step + data for a bounty.
func (h *Handler) GetBountyPipeline(w http.ResponseWriter, r *http.Request) {
	jobIDStr := r.PathValue("id")
	var jobID uint64
	fmt.Sscanf(jobIDStr, "%d", &jobID)
	val, ok := h.pipeline.Load(jobID)
	pd := &PipelineData{Step: ""}
	if ok {
		if p, ok := val.(*PipelineData); ok {
			pd = p
		}
	}
	writeJSON(w, http.StatusOK, pd)
}

// ==================== extractKeywordsFromRuleParams ====================

func extractKeywordsFromRuleParams(ruleParamsJSON string) []string {
	if ruleParamsJSON == "" || ruleParamsJSON == "null" || ruleParamsJSON == "[]" || ruleParamsJSON == "{}" {
		return nil
	}
	var templates []struct {
		Name   string                 `json:"name"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.Unmarshal([]byte(ruleParamsJSON), &templates); err != nil {
		return nil
	}
	for _, t := range templates {
		if t.Name == "keyword_coverage" {
			raw, ok := t.Params["keywords"]
			if !ok {
				return nil
			}
			kwList, ok := raw.([]interface{})
			if !ok {
				return nil
			}
			keywords := make([]string, 0, len(kwList))
			for _, v := range kwList {
				if s, ok := v.(string); ok {
					keywords = append(keywords, s)
				}
			}
			return keywords
		}
	}
	return nil
}

// ==================== evaluateDelivery ====================

func (h *Handler) evaluateDelivery(ctx context.Context, bounty *model.Bounty, delivery *engine.DeliveryContent, finalize bool) (*engine.FinalVerdict, error) {
	_ = h.ruleEngine.Evaluate(delivery.RawData, bounty.RuleParams)
	if delivery.Keywords == nil && bounty.RuleParams != "" {
		delivery.Keywords = extractKeywordsFromRuleParams(bounty.RuleParams)
	}
	ruleResult := h.ruleVeto.Evaluate(ctx, delivery)
	llmResult, llmErr := h.llmEngine.Evaluate(ctx, delivery)
	verdict := h.agg.Aggregate(ruleResult, llmResult, llmErr)
	h.log.Info("Evaluation complete", zap.Uint64("job_id", delivery.JobID), zap.String("status", verdict.Status), zap.Float64("score", verdict.Score), zap.String("summary", verdict.Summary))

	// Store evaluation details in pipeline
	val, _ := h.pipeline.Load(delivery.JobID)
	if pd, ok := val.(*PipelineData); ok {
		pd.EvalStatus = verdict.Status
		pd.EvalScore = verdict.Score
		pd.EvalSummary = verdict.Summary
		pd.EvalRuleBreakdown = ruleResult.Reason
		pd.EvalLLMScore = verdict.LLMScore
		pd.EvalLLMReason = llmResult.Reason
		if llmErr == nil && llmResult != nil {
			pd.EvalLLMReason = llmResult.Reason
		}
		h.pipeline.Store(delivery.JobID, pd)
	}

	if finalize && verdict.Status == "slashed" {
		h.store.UpdateBountyStatus(ctx, delivery.JobID, model.StatusDisputed)
	}
	return verdict, nil
}

// ==================== Debug endpoints ====================

func (h *Handler) autoClaim(ctx context.Context, jobID uint64) error {
	claimCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := h.store.ClaimBountyWithLock(claimCtx, jobID, h.cfg.CAW.ProviderAddr)
	return err
}

func (h *Handler) DebugPingPact(w http.ResponseWriter, r *http.Request) {
	jobID := uint64(time.Now().UnixMilli())
	h.sse.pushEvent("pact_approved", jobID, "active", "green", `{"pact_id":"debug-test"}`)
	w.Write([]byte(`{"pushed":true}`))
}

func (h *Handler) DebugTestAutoChain(w http.ResponseWriter, r *http.Request) {
	jobID := uint64(time.Now().UnixMilli())
	intent := "Base链 Aave/Compound 流动性分析，800字，含TVL和利率比较"
	amount := "50000000000000000"
	bounty := &model.Bounty{
		JobID: jobID, BuyerAddr: "0xa115523ac8f1391075c0f0d74418a4f159df53fe",
		Status: model.StatusOpen, PactID: "debug-auto-test", Amount: amount,
	}
	if err := h.store.CreateBounty(r.Context(), bounty); err != nil {
		writeError(w, http.StatusInternalServerError, "create bounty: "+err.Error())
		return
	}
	go func(jID uint64, intent, amount string) {
		chainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		h.log.Info("Debug: Starting auto-chain", zap.Uint64("job_id", jID))
		h.log.Info("Debug: Provider auto-claiming", zap.Uint64("job_id", jID))
		h.sse.pushEvent("agent_action", jID, "claiming", "yellow", fmt.Sprintf(`{"agent":"provider","action":"claiming","job_id":%d}`, jID))
		if err := h.autoClaim(chainCtx, jID); err != nil {
			h.log.Warn("Debug: Auto-claim failed", zap.Error(err))
			return
		}
		h.sse.pushEvent("agent_action", jID, "claimed", "green", fmt.Sprintf(`{"agent":"provider","action":"claimed","job_id":%d}`, jID))
		h.log.Info("Debug: Auto-claim succeeded", zap.Uint64("job_id", jID))

		h.sse.pushEvent("agent_thinking", jID, "analyzing", "purple", fmt.Sprintf(`{"agent":"provider","step":"analyzing task: %s"}`, intent))
		decision, err := h.agent.AnalyzeTask(chainCtx, intent, amount)
		if err != nil {
			h.log.Warn("Debug: Agent analysis failed", zap.Error(err))
			return
		}
		h.sse.pushEvent("agent_decided", jID, "decided", "purple", fmt.Sprintf(`{"agent":"provider","decision":"%v","reasoning":"%s"}`, decision.NeedsSubBounty, decision.Reasoning))
		h.log.Info("Debug: Agent decided", zap.Bool("needs_sub_bounty", decision.NeedsSubBounty))

		if decision.NeedsSubBounty && decision.SubDescription != "" {
			subAmountWei := "5000000000000000"
			if decision.SubAmount != "" {
				subAmountWei = decision.SubAmount
			}
			subJobID := uint64(time.Now().UnixMilli())
			subBounty := &model.Bounty{
				JobID: subJobID, BuyerAddr: h.cfg.CAW.ProviderAddr, SellerAddr: "",
				Amount: subAmountWei, Deadline: time.Now().Add(7 * 24 * time.Hour),
				Status: model.StatusOpen, PactID: "", ParentBountyID: &jID, Depth: 1,
			}
			if err := h.store.CreateSubBounty(chainCtx, subBounty); err != nil {
				h.log.Warn("Debug: Failed to create sub-bounty", zap.Error(err))
				return
			}
			h.log.Info("Debug: Sub-bounty created", zap.Uint64("sub_job_id", subJobID))
			h.sse.pushEvent("bounty_posted", subJobID, string(model.StatusOpen), "blue", fmt.Sprintf("Sub-bounty #%d created under #%d", subJobID, jID))

			subClaimCtx, subCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, subClaimErr := h.store.ClaimBountyWithLock(subClaimCtx, subJobID, h.cfg.CAW.SubProviderAddr)
			subCancel()
			if subClaimErr != nil {
				h.log.Warn("Debug: Sub-Provider auto-claim failed", zap.Error(subClaimErr))
				return
			}
			h.sse.pushEvent("agent_action", subJobID, "claimed", "green", fmt.Sprintf(`{"agent":"sub_provider","action":"claimed","job_id":%d}`, subJobID))
			h.log.Info("Debug: Sub-Provider claimed sub-bounty", zap.Uint64("sub_job_id", subJobID))

			h.sse.pushEvent("agent_thinking", subJobID, "generating_delivery", "purple", `{"agent":"sub_provider","step":"generating delivery for sub-task"}`)
			subDelivery, genErr := h.agent.GenerateSubDelivery(chainCtx, intent, decision.SubDescription)
			if genErr != nil {
				h.log.Warn("Debug: Sub-Provider delivery generation failed", zap.Error(genErr))
				return
			}
			h.log.Info("Debug: Sub-Provider generated delivery", zap.Int("len", len(subDelivery)))
			subCID, ipfsErr := h.ipfs.UploadResult(chainCtx, []byte(subDelivery))
			if ipfsErr != nil {
				h.log.Warn("Debug: Sub-Provider IPFS upload failed", zap.Error(ipfsErr))
				return
			}

			h.sse.pushEvent("evaluation_started", subJobID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating sub-bounty #%d", subJobID))
			subDeliveryContent := &engine.DeliveryContent{JobID: subJobID, Seller: h.cfg.CAW.SubProviderAddr, ResultHash: subCID, RawData: subDelivery}
			subVerdict, evalErr := h.evaluateDelivery(chainCtx, subBounty, subDeliveryContent, false)
			if evalErr != nil {
				h.log.Warn("Debug: Sub-bounty evaluation failed", zap.Error(evalErr))
				return
			}
			h.log.Info("Debug: Sub-bounty evaluation", zap.String("status", subVerdict.Status), zap.Float64("score", subVerdict.Score))
			if subVerdict.Status != "verified" {
				h.log.Warn("Debug: Sub-bounty not verified, aborting", zap.String("status", subVerdict.Status))
				return
			}

			h.sse.pushEvent("agent_thinking", jID, "merging", "purple", `{"agent":"provider","step":"merging sub-provider delivery with own analysis"}`)
			finalDelivery, mergeErr := h.agent.MergeDeliveries(chainCtx, intent, subDelivery)
			if mergeErr != nil {
				h.log.Warn("Debug: Provider merge failed", zap.Error(mergeErr))
				return
			}
			finalCID, finalIPFSErr := h.ipfs.UploadResult(chainCtx, []byte(finalDelivery))
			if finalIPFSErr != nil {
				h.log.Warn("Debug: Final IPFS upload failed", zap.Error(finalIPFSErr))
				return
			}
			h.store.UpdateBountyStatus(chainCtx, jID, model.StatusSubmitted)
			h.sse.pushEvent("agent_action", jID, "submitted", "yellow", `{"agent":"provider","action":"final_delivery_submitted"}`)

			h.sse.pushEvent("evaluation_started", jID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating final delivery for bounty #%d", jID))
			mainBounty, _ := h.store.GetBountyByID(chainCtx, jID)
			if mainBounty == nil {
				return
			}
			finalDeliveryContent := &engine.DeliveryContent{JobID: jID, Seller: h.cfg.CAW.ProviderAddr, ResultHash: finalCID, RawData: finalDelivery}
			finalVerdict, finalEvalErr := h.evaluateDelivery(chainCtx, mainBounty, finalDeliveryContent, true)
			if finalEvalErr != nil {
				return
			}
			h.log.Info("Debug: Final evaluation", zap.String("status", finalVerdict.Status), zap.Float64("score", finalVerdict.Score))
			if finalVerdict.Status == "verified" {
				h.log.Info("Debug: Full auto-chain complete ✅", zap.Uint64("job_id", jID))
			}
		}
	}(jobID, intent, amount)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "auto_chain_started", "job_id": jobID})
}

// ==================== Utility ====================

type contextKey string

const ctxKeyTraceID contextKey = "trace_id"

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, model.ErrorResponse{Error: msg, Code: status})
}

func weiFromStr(s string) uint64 {
	var val uint64
	fmt.Sscanf(s, "%d", &val)
	return val
}
