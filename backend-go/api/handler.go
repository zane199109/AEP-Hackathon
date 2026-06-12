package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
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

var PROVIDER_ADDR = "0x276e8c07f3c140d6f894ee5567df146d58db3c56"
var SUB_PROVIDER_ADDR = "0xe813c4298dc1263de7ec22293f1175ed2afa0623"

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
		log: log,
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
}

// PipelineData stores the auto-chain pipeline state for real-time frontend display.
type PipelineData struct {
	Step          string  `json:"step"`
	Reasoning     string  `json:"reasoning,omitempty"`
	SubDelivery   string  `json:"sub_delivery,omitempty"`
	FinalDelivery string  `json:"final_delivery,omitempty"`
	EvalStatus    string  `json:"eval_status,omitempty"`
	EvalScore     float64 `json:"eval_score,omitempty"`
	EvalSummary   string  `json:"eval_summary,omitempty"`
	EvalRuleBreakdown string `json:"eval_rule_breakdown,omitempty"`
	EvalLLMScore      float64 `json:"eval_llm_score,omitempty"`
	EvalLLMReason     string  `json:"eval_llm_reason,omitempty"`
	SubEvalStatus    string  `json:"sub_eval_status,omitempty"`
	SubEvalScore     float64 `json:"sub_eval_score,omitempty"`
	SubEvalSummary   string  `json:"sub_eval_summary,omitempty"`
	SubEvalRuleBreakdown string `json:"sub_eval_rule_breakdown,omitempty"`
	SubEvalLLMScore      float64 `json:"sub_eval_llm_score,omitempty"`
	SubEvalLLMReason     string  `json:"sub_eval_llm_reason,omitempty"`
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
func (h *Handler) updatePipeline(jobID uint64, step string, opts ...string) {
	val, _ := h.pipeline.Load(jobID)
	pd, ok := val.(*PipelineData)
	if !ok {
		pd = &PipelineData{}
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
	if len(opts) > 2 && opts[2] != "" { pd.EvalStatus = opts[2] }
	if len(opts) > 3 && opts[3] != "" {
		fmt.Sscanf(opts[3], "%f", &pd.EvalScore)
	}
	if len(opts) > 4 && opts[4] != "" { pd.EvalSummary = opts[4] }
	h.pipeline.Store(jobID, pd)
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

	var deadlineSeconds uint64 = 2592000
	if req.Deadline != "" {
		if t, err := time.Parse(time.RFC3339, req.Deadline); err == nil {
			if remaining := time.Until(t); remaining > 0 {
				deadlineSeconds = uint64(remaining.Seconds())
			}
		}
	}

	pact, err := h.caw.CreatePact(ctx, bizIDStr, h.cfg.CAW.WalletID, req.Buyer, req.Amount, "BASE_ETH", req.Intent, deadlineSeconds, req.MinReputation)
	if err != nil {
		h.log.Error("CAW CreatePact failed", zap.String("trace_id", traceID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "CAW pact creation failed: "+err.Error())
		return
	}

	jobID := uint64(time.Now().UnixMilli())
	bounty := &model.Bounty{
		JobID: jobID, BuyerAddr: req.Buyer, Status: model.StatusOpen,
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
			decision, err := h.agent.AnalyzeTask(pollCtx, intent, amount)
			if err != nil {
				h.log.Warn("Agent analysis failed", zap.Uint64("job_id", jID), zap.Error(err))
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
					JobID: subJobID, BuyerAddr: PROVIDER_ADDR, SellerAddr: "",
					Amount: subAmountWei, Deadline: time.Now().Add(7 * 24 * time.Hour),
					Status: model.StatusOpen, PactID: "", ParentBountyID: &jID, Depth: 1,
				}
				if err := h.store.CreateSubBounty(pollCtx, subBounty); err != nil {
					h.log.Warn("Failed to create sub-bounty", zap.Error(err))
					return
				}
				h.sse.pushEvent("bounty_posted", subJobID, string(model.StatusOpen), "blue", fmt.Sprintf("Sub-bounty #%d created under #%d", subJobID, jID))

				// Sub-Provider auto-claim
				h.sse.pushEvent("agent_action", subJobID, "claiming", "yellow", fmt.Sprintf(`{"agent":"sub_provider","action":"claiming","job_id":%d}`, subJobID))
				h.updatePipeline(jID, "sub_claiming")
				subClaimCtx, subCancel := context.WithTimeout(context.Background(), 10*time.Second)
				_, subClaimErr := h.store.ClaimBountyWithLock(subClaimCtx, subJobID, SUB_PROVIDER_ADDR)
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
				subDelivery, genErr := h.agent.GenerateSubDelivery(pollCtx, intent, decision.SubDescription)
				if genErr != nil {
					h.log.Warn("Sub-Provider delivery generation failed", zap.Error(genErr))
					return
				}
				h.updatePipeline(jID, "generating_sub_delivery", "", subDelivery)

				subCID, ipfsErr := h.ipfs.UploadResult(pollCtx, []byte(subDelivery))
				if ipfsErr != nil {
					h.log.Warn("Sub-Provider IPFS upload failed", zap.Error(ipfsErr))
					return
				}

				// Evaluate sub-bounty
				h.sse.pushEvent("agent_action", subJobID, "submitting", "yellow", `{"agent":"sub_provider","action":"submitting_delivery"}`)
				h.sse.pushEvent("evaluation_started", subJobID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating sub-bounty #%d", subJobID))
			h.updatePipeline(jID, "evaluating_sub")
			// Init pipeline for subJobID so evaluateDelivery can store details
			h.pipeline.Store(subJobID, &PipelineData{Step: "evaluating_sub"})
			subDeliveryContent := &engine.DeliveryContent{
				JobID: subJobID, Seller: SUB_PROVIDER_ADDR, ResultHash: subCID, RawData: subDelivery,
			}
			subVerdict, evalErr := h.evaluateDelivery(pollCtx, subBounty, subDeliveryContent, false)
			if evalErr != nil {
				h.log.Warn("Sub-bounty evaluation failed", zap.Error(evalErr))
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
					return
				}
				h.updatePipeline(jID, "sub_verified")

				// Provider merges or generates failed delivery
				h.sse.pushEvent("agent_thinking", jID, "merging", "purple", `{"agent":"provider","step":"merging sub-provider delivery with own analysis"}`)
				var finalDelivery string
				var mergeErr error
				if demoSlash {
					h.log.Info("DEMO SLASH: generating intentionally poor delivery", zap.Uint64("job_id", jID))
					h.sse.pushEvent("agent_action", jID, "generating_failed_delivery", "red", fmt.Sprintf(`{"agent":"provider","action":"仲裁演示: 生成低质量交付物"}`))
					finalDelivery, mergeErr = h.agent.GenerateFailedDelivery(pollCtx, intent)
				} else {
					finalDelivery, mergeErr = h.agent.MergeDeliveries(pollCtx, intent, subDelivery)
				}
				if mergeErr != nil {
					h.log.Warn("Provider merge failed", zap.Error(mergeErr))
					return
				}

				finalCID, finalIPFSErr := h.ipfs.UploadResult(pollCtx, []byte(finalDelivery))
				if finalIPFSErr != nil {
					h.log.Warn("Provider final IPFS upload failed", zap.Error(finalIPFSErr))
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
					return
				}
				finalDeliveryContent := &engine.DeliveryContent{
					JobID: jID, Seller: PROVIDER_ADDR, ResultHash: finalCID, RawData: finalDelivery,
				}
				finalVerdict, finalEvalErr := h.evaluateDelivery(pollCtx, mainBounty, finalDeliveryContent, true)
				if finalEvalErr != nil {
					h.log.Warn("Final delivery evaluation failed", zap.Error(finalEvalErr))
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
					h.updatePipeline(jID, "awaiting_confirmation")
				}
			}

			if !decision.NeedsSubBounty {
				h.log.Info("Provider handling task directly — auto-submitting", zap.Uint64("job_id", jID))
				h.sse.pushEvent("agent_thinking", jID, "generating_delivery", "purple", fmt.Sprintf(`{"agent":"provider","step":"generating delivery for task: %s"}`, intent))
				directDelivery, directErr := h.agent.GenerateSubDelivery(pollCtx, intent, intent)
				if directErr != nil { return }
				h.updatePipeline(jID, "generating_delivery", "", directDelivery)
				directCID, ipfsErr := h.ipfs.UploadResult(pollCtx, []byte(directDelivery))
				if ipfsErr != nil { return }
				h.store.UpdateBountyStatus(pollCtx, jID, model.StatusSubmitted)
				h.sse.pushEvent("agent_action", jID, "submitted", "yellow", `{"agent":"provider","action":"delivery_submitted"}`)
				h.sse.pushEvent("evaluation_started", jID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating delivery for bounty #%d", jID))
				mainBounty, mainErr := h.store.GetBountyByID(pollCtx, jID)
				if mainErr != nil { return }
				directDeliveryContent := &engine.DeliveryContent{
					JobID: jID, Seller: PROVIDER_ADDR, ResultHash: directCID, RawData: directDelivery,
				}
				directVerdict, directEvalErr := h.evaluateDelivery(pollCtx, mainBounty, directDeliveryContent, true)
				if directEvalErr != nil { return }
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
	h.sse.pushEvent("bounty_posted", jobID, string(model.StatusOpen), "blue", "Bounty posted, funds locked in CAW pact")
}

// ==================== ClaimBounty ====================

func (h *Handler) ClaimBounty(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := r.Header.Get("X-Trace-ID")
	ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)

	jobIDStr := r.PathValue("id")
	if jobIDStr == "" { writeError(w, http.StatusBadRequest, "missing job id"); return }
	var jobID uint64
	fmt.Sscanf(jobIDStr, "%d", &jobID)

	var req struct{ Seller string `json:"seller"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Seller == "" {
		writeError(w, http.StatusBadRequest, "seller address required"); return
	}

	locked, err := h.store.TryAcquireClaimLock(ctx, jobID)
	if err != nil { writeError(w, http.StatusInternalServerError, "internal error"); return }
	if !locked { writeError(w, http.StatusConflict, "bounty already claimed or being claimed"); return }
	defer h.store.ReleaseClaimLock(ctx, jobID)

	if h.reputation != nil {
		if result, err := h.reputation.CheckReputation(ctx, req.Seller); err == nil && !result.Passed {
			writeError(w, http.StatusForbidden, fmt.Sprintf("provider reputation too low (score: %d)", result.Score)); return
		}
	}

	fluxaResult, err := h.fluxa.CheckReputation(ctx, req.Seller)
	if err != nil { fluxaResult = &provider.FluxAResult{Passed: true, FallbackUsed: true, Score: 0} }
	if !fluxaResult.Passed { writeError(w, http.StatusForbidden, "provider reputation check failed"); return }

	if _, err := h.store.ClaimBountyWithLock(ctx, jobID, req.Seller); err != nil {
		writeError(w, http.StatusConflict, "bounty already assigned or invalid"); return
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, http.StatusBadRequest, "invalid request body"); return }

	parent, err := h.store.GetBountyByID(ctx, parentID)
	if err != nil { writeError(w, http.StatusNotFound, "parent bounty not found"); return }
	if parent.Status != model.StatusAssigned { writeError(w, http.StatusBadRequest, fmt.Sprintf("parent not in Assigned (current: %s)", parent.Status)); return }
	if req.Seller != string(parent.SellerAddr) { writeError(w, http.StatusForbidden, "only assigned provider can create sub-bounties"); return }

	jobID := uint64(time.Now().UnixMilli())
	subBounty := &model.Bounty{
		JobID: jobID, BuyerAddr: parent.SellerAddr, SellerAddr: "",
		Amount: req.Amount, Deadline: parent.Deadline, Status: model.StatusOpen,
		PactID: "", ParentBountyID: &parentID, Depth: parent.Depth + 1, BuyerWalletID: req.WalletID,
	}
	if err := h.store.CreateSubBounty(ctx, subBounty); err != nil { writeError(w, http.StatusInternalServerError, "database error"); return }

	h.sse.pushEvent("bounty_posted", jobID, string(model.StatusOpen), "blue", fmt.Sprintf("Sub-bounty created under #%d", parentID))
	writeJSON(w, http.StatusOK, model.SubBountyResponse{JobID: jobID, ParentID: parentID, Depth: parent.Depth + 1, Amount: req.Amount, Status: string(model.StatusOpen)})
}

// ==================== SubmitResult ====================

func (h *Handler) SubmitResult(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" { traceID = fmt.Sprintf("trace_%d", time.Now().UnixNano()) }
	ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)

	jobIDStr := r.PathValue("id")
	var jobID uint64
	fmt.Sscanf(jobIDStr, "%d", &jobID)

	var req struct{ Seller string `json:"seller"`; Data string `json:"data"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { writeError(w, http.StatusBadRequest, "invalid body"); return }

	bounty, err := h.store.GetBountyByID(ctx, jobID)
	if err != nil { writeError(w, http.StatusNotFound, "bounty not found"); return }
	if string(bounty.Status) != string(model.StatusAssigned) { writeError(w, http.StatusBadRequest, fmt.Sprintf("bounty not in Assigned (current: %s)", bounty.Status)); return }

	cid, err := h.ipfs.UploadResult(ctx, []byte(req.Data))
	if err != nil { writeError(w, http.StatusInternalServerError, "ipfs upload failed"); return }

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
	if traceID == "" { traceID = fmt.Sprintf("trace_%d", time.Now().UnixNano()) }
	ctx = context.WithValue(ctx, ctxKeyTraceID, traceID)

	var jobID uint64
	fmt.Sscanf(r.PathValue("jobId"), "%d", &jobID)

	if err := h.store.ConfirmBuyerApproval(ctx, jobID); err != nil {
		h.log.Error("Failed to set BuyerApproval", zap.Uint64("job_id", jobID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to confirm"); return
	}
	h.log.Info("UserConfirmed", zap.String("trace_id", traceID), zap.Uint64("job_id", jobID))

	result := h.relayer.SettleBountyTree(ctx, jobID, traceID)
	if !result.Success {
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "confirmed", "settlement": result.Status, "settlement_msg": result.Message})
		return
	}

	// Check if release is pending CAW approval
	releasePending := result.TransferStatus == "pending_approval"
	if releasePending {
		bounty, _ := h.store.GetBountyByID(ctx, jobID)
		if bounty == nil { bounty = &model.Bounty{} }
		h.log.Info("Release pending CAW approval — will poll", zap.Uint64("job_id", jobID), zap.String("pact_id", bounty.PactID))
		h.sse.pushEvent("release_pending", jobID, "PendingApproval", "yellow",
			fmt.Sprintf(`{"pact_id":"%s","amount":"%s"}`, bounty.PactID, bounty.Amount))
		h.updatePipeline(jobID, "release_pending")

		// Start polling goroutine for release approval
		go func(jID uint64, pactID string) {
			pollCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-pollCtx.Done():
					h.log.Warn("Release polling timed out", zap.Uint64("job_id", jID))
					return
				case <-ticker.C:
					ps, err := h.caw.GetPactStatus(pollCtx, pactID)
					if err != nil { continue }
					if ps.Status != "active" {
						h.log.Info("Release approved — settlement complete", zap.Uint64("job_id", jID), zap.String("pact_status", ps.Status))
						h.updatePipeline(jID, "settled")

						// Reputation reward
						repCtx := context.Background()
						b, _ := h.store.GetBountyByID(repCtx, jID)
						if b != nil {
							// Reputation reward inline
						repResult, _ := h.reputation.CheckReputation(repCtx, string(b.SellerAddr))
						currentScore := 50.0; taskCount := 0
						if repResult != nil { currentScore = float64(repResult.Score) }
						taskInfo := reputation.TaskInfo{QualityScore: h.getEvalQualityScore(jID), AmountWei: weiFromStr(b.Amount), SubmittedAt: time.Now().Unix(), Deadline: b.Deadline.Unix()}
						newScore := reputation.CalculateNewScore(currentScore, taskInfo, taskCount)
						paramHash := reputation.ComputeParamHash(true, taskInfo.AmountWei, uint64(taskInfo.SubmittedAt), uint64(taskInfo.Deadline))
						delta := newScore - currentScore
						if delta > 0 {
							h.sse.pushEvent("reputation_changed", jID, "Verified", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"taskId":"#%d","reason":"delivery_passed"}`, string(b.SellerAddr), currentScore, newScore, delta, jID))
							if txHash, _ := h.reputation.UpdateReputation(repCtx, string(b.SellerAddr), jID, uint64(newScore), int64(delta), "delivery_passed", paramHash); txHash != "" {
								h.sse.pushEvent("reputation_updated", jID, "onchain", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"txHash":"%s"}`, string(b.SellerAddr), currentScore, newScore, delta, txHash))
							}
						}
						// Sub-Provider reputation
						children, _ := h.store.GetChildrenBounties(repCtx, jID)
						for _, child := range children {
							if child.SellerAddr == "" { continue }
							childRep, _ := h.reputation.CheckReputation(repCtx, string(child.SellerAddr))
							ccs := 50.0
							if childRep != nil { ccs = float64(childRep.Score) }
							cti := reputation.TaskInfo{QualityScore: h.getEvalQualityScore(child.JobID), AmountWei: weiFromStr(child.Amount), SubmittedAt: time.Now().Unix(), Deadline: child.Deadline.Unix()}
							cns := reputation.CalculateNewScore(ccs, cti, 0)
							cph := reputation.ComputeParamHash(true, cti.AmountWei, uint64(cti.SubmittedAt), uint64(cti.Deadline))
							cd := cns - ccs
							if cd > 0 {
								h.sse.pushEvent("reputation_changed", child.JobID, "Verified", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"taskId":"#%d","reason":"sub_bounty_settled"}`, string(child.SellerAddr), ccs, cns, cd, child.JobID))
								if txHash, _ := h.reputation.UpdateReputation(repCtx, string(child.SellerAddr), child.JobID, uint64(cns), int64(cd), "sub_bounty_settled", cph); txHash != "" {
									h.sse.pushEvent("reputation_updated", child.JobID, "onchain", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"txHash":"%s"}`, string(child.SellerAddr), ccs, cns, cd, txHash))
								}
							}
						}
						}
						h.sse.pushEvent("settled", jID, "Verified", "green", "Release approved, funds transferred")
						return
					}
				}
			}
		}(jobID, bounty.PactID)
	}

	// Immediate settlement (no pending approval needed)
	if !releasePending {
		h.updatePipeline(jobID, "settled")
		if result.Success {
			go func() {
				repCtx := context.Background()
				b, _ := h.store.GetBountyByID(repCtx, jobID)
				if b == nil { return }
				repResult, _ := h.reputation.CheckReputation(repCtx, string(b.SellerAddr))
				currentScore := 50.0; taskCount := 0
				if repResult != nil { currentScore = float64(repResult.Score) }
				taskInfo := reputation.TaskInfo{QualityScore: h.getEvalQualityScore(jobID), AmountWei: weiFromStr(b.Amount), SubmittedAt: time.Now().Unix(), Deadline: b.Deadline.Unix()}
				newScore := reputation.CalculateNewScore(currentScore, taskInfo, taskCount)
				paramHash := reputation.ComputeParamHash(true, taskInfo.AmountWei, uint64(taskInfo.SubmittedAt), uint64(taskInfo.Deadline))
				delta := newScore - currentScore
				if delta > 0 {
					h.sse.pushEvent("reputation_changed", jobID, "Verified", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"taskId":"#%d","reason":"delivery_passed"}`, string(b.SellerAddr), currentScore, newScore, delta, jobID))
					if txHash, _ := h.reputation.UpdateReputation(repCtx, string(b.SellerAddr), jobID, uint64(newScore), int64(delta), "delivery_passed", paramHash); txHash != "" {
						h.sse.pushEvent("reputation_updated", jobID, "onchain", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"txHash":"%s"}`, string(b.SellerAddr), currentScore, newScore, delta, txHash))
					}
				}
				children, _ := h.store.GetChildrenBounties(repCtx, jobID)
				for _, child := range children {
					if child.SellerAddr == "" { continue }
					childRep, _ := h.reputation.CheckReputation(repCtx, string(child.SellerAddr))
					ccs := 50.0
					if childRep != nil { ccs = float64(childRep.Score) }
					cti := reputation.TaskInfo{QualityScore: h.getEvalQualityScore(child.JobID), AmountWei: weiFromStr(child.Amount), SubmittedAt: time.Now().Unix(), Deadline: child.Deadline.Unix()}
					cns := reputation.CalculateNewScore(ccs, cti, 0)
					cph := reputation.ComputeParamHash(true, cti.AmountWei, uint64(cti.SubmittedAt), uint64(cti.Deadline))
					cd := cns - ccs
					if cd > 0 {
						h.sse.pushEvent("reputation_changed", child.JobID, "Verified", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"taskId":"#%d","reason":"sub_bounty_settled"}`, string(child.SellerAddr), ccs, cns, cd, child.JobID))
						if txHash, _ := h.reputation.UpdateReputation(repCtx, string(child.SellerAddr), child.JobID, uint64(cns), int64(cd), "sub_bounty_settled", cph); txHash != "" {
							h.sse.pushEvent("reputation_updated", child.JobID, "onchain", "green", fmt.Sprintf(`{"agent":"%s","oldScore":%.0f,"newScore":%.0f,"delta":%.0f,"txHash":"%s"}`, string(child.SellerAddr), ccs, cns, cd, txHash))
						}
					}
				}
			}()
		}
		h.sse.pushEvent("settled", jobID, "Verified", "green", fmt.Sprintf("Buyer confirmed, settlement: %s", result.Status))
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "confirmed", "settlement": result.Status, "settlement_msg": result.Message})
	} else {
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": "release_pending", "message": "Release pending CAW approval"})
	}
}

// ==================== AdminRetry ====================

func (h *Handler) AdminRetry(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Demo-Admin-Token")
	if token == "" { writeError(w, http.StatusUnauthorized, "missing admin token"); return }
	ctx := r.Context()
	var jobID uint64
	fmt.Sscanf(r.PathValue("jobId"), "%d", &jobID)
	result := h.relayer.SettleBounty(ctx, jobID, fmt.Sprintf("admin_%d", time.Now().UnixNano()))
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "retry_attempted", "settlement": result.Status, "message": result.Message})
}

// ==================== Health / Pact / Parse / Arbitrate ====================

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, map[string]string{"status": "ok"}) }

func (h *Handler) GetPactStatus(w http.ResponseWriter, r *http.Request) {
	pactID := r.PathValue("pactId")
	if pactID == "" { writeError(w, http.StatusBadRequest, "missing pact id"); return }
	status, err := h.caw.GetPactStatus(r.Context(), pactID)
	if err != nil { writeError(w, http.StatusInternalServerError, "pact status check failed"); return }
	writeJSON(w, http.StatusOK, map[string]interface{}{"pact_id": pactID, "status": status.Status})
}

func (h *Handler) ParseIntent(w http.ResponseWriter, r *http.Request) {
	var req struct{ Text string `json:"text"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" { writeError(w, http.StatusBadRequest, "missing text"); return }
	result := rule.ExtractRuleParams(r.Context(), req.Text, h.ruleCfg, h.log)
	response := map[string]interface{}{"title": req.Text, "intent": req.Text, "rule_params": result, "source": "keyword"}
	if result != "" {
		response["source"] = "llm"
		var parsed struct {
			AmountEth   float64 `json:"suggested_amount_eth"`
			DeadlineDays int    `json:"suggested_deadline_days"`
			MinRep      int    `json:"suggested_min_reputation"`
		}
		if err := json.Unmarshal([]byte(result), &parsed); err == nil {
			if parsed.AmountEth > 0 { response["suggested_amount_eth"] = parsed.AmountEth }
			if parsed.DeadlineDays > 0 { response["suggested_deadline_days"] = parsed.DeadlineDays }
			if parsed.MinRep > 0 { response["suggested_min_reputation"] = parsed.MinRep }
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) ArbitrateSlash(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var jobID uint64
	fmt.Sscanf(r.PathValue("jobId"), "%d", &jobID)

	bounty, err := h.store.GetBountyByID(ctx, jobID)
	if err != nil { writeError(w, http.StatusNotFound, "bounty not found"); return }
	if bounty.Status != model.StatusDisputed { writeError(w, http.StatusBadRequest, fmt.Sprintf("bounty not in Disputed (current: %s)", bounty.Status)); return }

	h.store.UpdateBountyStatus(ctx, jobID, model.StatusSlashed)

	go func() {
		repCtx := context.Background()
		seller := string(bounty.SellerAddr)
		repResult, _ := h.reputation.CheckReputation(repCtx, seller)
		currentScore := 50.0
		if repResult != nil { currentScore = float64(repResult.Score) }
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
	if addr == "" { writeError(w, http.StatusBadRequest, "missing address"); return }
	result, err := h.reputation.CheckReputation(r.Context(), addr)
	score := 50; fallback := true
	if err == nil { score = result.Score; fallback = result.FallbackUsed }
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
	if ruleParamsJSON == "" || ruleParamsJSON == "null" || ruleParamsJSON == "[]" || ruleParamsJSON == "{}" { return nil }
	var templates []struct {
		Name   string                 `json:"name"`
		Params map[string]interface{} `json:"params"`
	}
	if err := json.Unmarshal([]byte(ruleParamsJSON), &templates); err != nil { return nil }
	for _, t := range templates {
		if t.Name == "keyword_coverage" {
			raw, ok := t.Params["keywords"]; if !ok { return nil }
			kwList, ok := raw.([]interface{}); if !ok { return nil }
			keywords := make([]string, 0, len(kwList))
			for _, v := range kwList { if s, ok := v.(string); ok { keywords = append(keywords, s) } }
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
	_, err := h.store.ClaimBountyWithLock(claimCtx, jobID, PROVIDER_ADDR)
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
		writeError(w, http.StatusInternalServerError, "create bounty: "+err.Error()); return
	}
	go func(jID uint64, intent, amount string) {
		chainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		h.log.Info("Debug: Starting auto-chain", zap.Uint64("job_id", jID))
		h.log.Info("Debug: Provider auto-claiming", zap.Uint64("job_id", jID))
		h.sse.pushEvent("agent_action", jID, "claiming", "yellow", fmt.Sprintf(`{"agent":"provider","action":"claiming","job_id":%d}`, jID))
		if err := h.autoClaim(chainCtx, jID); err != nil { h.log.Warn("Debug: Auto-claim failed", zap.Error(err)); return }
		h.sse.pushEvent("agent_action", jID, "claimed", "green", fmt.Sprintf(`{"agent":"provider","action":"claimed","job_id":%d}`, jID))
		h.log.Info("Debug: Auto-claim succeeded", zap.Uint64("job_id", jID))

		h.sse.pushEvent("agent_thinking", jID, "analyzing", "purple", fmt.Sprintf(`{"agent":"provider","step":"analyzing task: %s"}`, intent))
		decision, err := h.agent.AnalyzeTask(chainCtx, intent, amount)
		if err != nil { h.log.Warn("Debug: Agent analysis failed", zap.Error(err)); return }
		h.sse.pushEvent("agent_decided", jID, "decided", "purple", fmt.Sprintf(`{"agent":"provider","decision":"%v","reasoning":"%s"}`, decision.NeedsSubBounty, decision.Reasoning))
		h.log.Info("Debug: Agent decided", zap.Bool("needs_sub_bounty", decision.NeedsSubBounty))

		if decision.NeedsSubBounty && decision.SubDescription != "" {
			subAmountWei := "5000000000000000"
			if decision.SubAmount != "" { subAmountWei = decision.SubAmount }
			subJobID := uint64(time.Now().UnixMilli())
			subBounty := &model.Bounty{
				JobID: subJobID, BuyerAddr: PROVIDER_ADDR, SellerAddr: "",
				Amount: subAmountWei, Deadline: time.Now().Add(7 * 24 * time.Hour),
				Status: model.StatusOpen, PactID: "", ParentBountyID: &jID, Depth: 1,
			}
			if err := h.store.CreateSubBounty(chainCtx, subBounty); err != nil { h.log.Warn("Debug: Failed to create sub-bounty", zap.Error(err)); return }
			h.log.Info("Debug: Sub-bounty created", zap.Uint64("sub_job_id", subJobID))
			h.sse.pushEvent("bounty_posted", subJobID, string(model.StatusOpen), "blue", fmt.Sprintf("Sub-bounty #%d created under #%d", subJobID, jID))

			subClaimCtx, subCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, subClaimErr := h.store.ClaimBountyWithLock(subClaimCtx, subJobID, SUB_PROVIDER_ADDR)
			subCancel()
			if subClaimErr != nil { h.log.Warn("Debug: Sub-Provider auto-claim failed", zap.Error(subClaimErr)); return }
			h.sse.pushEvent("agent_action", subJobID, "claimed", "green", fmt.Sprintf(`{"agent":"sub_provider","action":"claimed","job_id":%d}`, subJobID))
			h.log.Info("Debug: Sub-Provider claimed sub-bounty", zap.Uint64("sub_job_id", subJobID))

			h.sse.pushEvent("agent_thinking", subJobID, "generating_delivery", "purple", `{"agent":"sub_provider","step":"generating delivery for sub-task"}`)
			subDelivery, genErr := h.agent.GenerateSubDelivery(chainCtx, intent, decision.SubDescription)
			if genErr != nil { h.log.Warn("Debug: Sub-Provider delivery generation failed", zap.Error(genErr)); return }
			h.log.Info("Debug: Sub-Provider generated delivery", zap.Int("len", len(subDelivery)))
			subCID, ipfsErr := h.ipfs.UploadResult(chainCtx, []byte(subDelivery))
			if ipfsErr != nil { h.log.Warn("Debug: Sub-Provider IPFS upload failed", zap.Error(ipfsErr)); return }

			h.sse.pushEvent("evaluation_started", subJobID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating sub-bounty #%d", subJobID))
			subDeliveryContent := &engine.DeliveryContent{JobID: subJobID, Seller: SUB_PROVIDER_ADDR, ResultHash: subCID, RawData: subDelivery}
			subVerdict, evalErr := h.evaluateDelivery(chainCtx, subBounty, subDeliveryContent, false)
			if evalErr != nil { h.log.Warn("Debug: Sub-bounty evaluation failed", zap.Error(evalErr)); return }
			h.log.Info("Debug: Sub-bounty evaluation", zap.String("status", subVerdict.Status), zap.Float64("score", subVerdict.Score))
			if subVerdict.Status != "verified" { h.log.Warn("Debug: Sub-bounty not verified, aborting", zap.String("status", subVerdict.Status)); return }

			h.sse.pushEvent("agent_thinking", jID, "merging", "purple", `{"agent":"provider","step":"merging sub-provider delivery with own analysis"}`)
			finalDelivery, mergeErr := h.agent.MergeDeliveries(chainCtx, intent, subDelivery)
			if mergeErr != nil { h.log.Warn("Debug: Provider merge failed", zap.Error(mergeErr)); return }
			finalCID, finalIPFSErr := h.ipfs.UploadResult(chainCtx, []byte(finalDelivery))
			if finalIPFSErr != nil { h.log.Warn("Debug: Final IPFS upload failed", zap.Error(finalIPFSErr)); return }
			h.store.UpdateBountyStatus(chainCtx, jID, model.StatusSubmitted)
			h.sse.pushEvent("agent_action", jID, "submitted", "yellow", `{"agent":"provider","action":"final_delivery_submitted"}`)

			h.sse.pushEvent("evaluation_started", jID, "Evaluating", "purple", fmt.Sprintf("AEP evaluating final delivery for bounty #%d", jID))
			mainBounty, _ := h.store.GetBountyByID(chainCtx, jID)
			if mainBounty == nil { return }
			finalDeliveryContent := &engine.DeliveryContent{JobID: jID, Seller: PROVIDER_ADDR, ResultHash: finalCID, RawData: finalDelivery}
			finalVerdict, finalEvalErr := h.evaluateDelivery(chainCtx, mainBounty, finalDeliveryContent, true)
			if finalEvalErr != nil { return }
			h.log.Info("Debug: Final evaluation", zap.String("status", finalVerdict.Status), zap.Float64("score", finalVerdict.Score))
			if finalVerdict.Status == "verified" { h.log.Info("Debug: Full auto-chain complete ✅", zap.Uint64("job_id", jID)) }
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
