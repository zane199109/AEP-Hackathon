package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
)

// CAW (Cobo Agentic Wallet) API endpoints.
// Dev vs Prod is determined by config.Sandbox.
const (
	cawDevBaseURL     = "https://api-core.agenticwallet.dev.cobo.com/api/v1"
	cawProdBaseURL    = "https://api-core.agenticwallet.cobo.com/api/v1"
)

// RetryConfig holds retry settings.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	Log         *zap.Logger
	Operation   string
}

// DoWithRetry executes fn with exponential backoff.
// Red line: max 3 attempts, exponential backoff base 1s.
func DoWithRetry(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := fn(ctx); err != nil {
			lastErr = err
			cfg.Log.Warn("retry attempt failed",
				zap.String("operation", cfg.Operation),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
			if attempt == cfg.MaxAttempts {
				break
			}
			delay := cfg.BaseDelay * (1 << (attempt - 1)) // 1s, 2s, 4s
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("%s failed after %d attempts: %w", cfg.Operation, cfg.MaxAttempts, lastErr)
}

// ==================== CAW REST Client ====================

// CAWClient handles HTTP requests to the Cobo Agentic Wallet REST API.
// Auth: X-API-Key header (see POST /api/v1/principals/provision to create a key).
type CAWClient struct {
	apiKey  string
	baseURL string
	httpCli *http.Client
	log     *zap.Logger
}

// NewCAWClient creates a new CAW API client.
func NewCAWClient(apiKey string, sandbox bool, timeout time.Duration, log *zap.Logger) *CAWClient {
	baseURL := cawProdBaseURL
	if sandbox {
		baseURL = cawDevBaseURL
	}
	return &CAWClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpCli: &http.Client{Timeout: timeout},
		log:     log,
	}
}

// doRequest sends a request to the CAW API with X-API-Key auth and optional extra headers.
func (c *CAWClient) doRequest(ctx context.Context, method, path string, body interface{}, out interface{}, extraHeaders ...map[string]string) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	// Apply optional extra headers (e.g. Idempotency-Key)
	for _, hdr := range extraHeaders {
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// The CAW API wraps responses in { "success": bool, "result": ..., "message": "..." }
	var wrapper struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		// Try raw parse if wrapper format isn't used
		wrapper.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
		wrapper.Result = respBody
	}

	if !wrapper.Success || resp.StatusCode >= 400 {
		msg := wrapper.Message
		if msg == "" {
			msg = string(respBody)
		}
		return fmt.Errorf("caw api error (status %d): %s", resp.StatusCode, msg)
	}

	c.log.Debug("CAW API response",
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("status", resp.StatusCode),
	)

	if out != nil && len(wrapper.Result) > 0 {
		if err := json.Unmarshal(wrapper.Result, out); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}
	return nil
}

// ==================== CAW Provider ====================

// CAWProvider wraps the Cobo Agentic Wallet API for AEP operations.
type CAWProvider struct {
	client *CAWClient
	cfg    config.CAWConfig
	log    *zap.Logger
}

// NewCAWProvider creates a CAW provider.
func NewCAWProvider(cfg config.CAWConfig, log *zap.Logger) *CAWProvider {
	client := NewCAWClient(cfg.APIKey, cfg.Sandbox, cfg.Timeout, log)
	return &CAWProvider{client: client, cfg: cfg, log: log}
}

// PactResult holds the result of a pact submission.
type PactResult struct {
	PactID     string `json:"pact_id"`
	Status     string `json:"status"`
	ApprovalID string `json:"approval_id,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
}

// ==================== Wallet Management ====================

// ProvisionResponse is returned by POST /principals/provision.
type ProvisionResponse struct {
	AgentID string `json:"agent_id"`
	APIKey  string `json:"api_key"`
	Status  string `json:"status"`
}

// ProvisionAgent creates a new agent API key via POST /principals/provision.
// This is the first step — the returned api_key becomes the X-API-Key for all subsequent calls.
func (p *CAWProvider) ProvisionAgent(ctx context.Context, name string) (*ProvisionResponse, error) {
	body := map[string]string{"name": name}
	var result ProvisionResponse
	err := DoWithRetry(ctx, RetryConfig{
		MaxAttempts: p.cfg.Retry.MaxAttempts,
		BaseDelay:   p.cfg.Retry.BaseDelay,
		Log:         p.log,
		Operation:   "CAW.ProvisionAgent",
	}, func(ctx context.Context) error {
		return p.client.doRequest(ctx, "POST", "/principals/provision", body, &result)
	})
	if err != nil {
		return nil, fmt.Errorf("provision agent: %w", err)
	}
	return &result, nil
}

// SubmitPactRequest is the payload for POST /pacts/submit.
type SubmitPactRequest struct {
	WalletID        string     `json:"wallet_id"`
	Name            string     `json:"name"`
	Intent          string     `json:"intent"`
	OriginalIntent  string     `json:"original_intent"`
	Amount          string     `json:"amount,omitempty"`
	MinReputation   int        `json:"min_reputation,omitempty"`
	Spec            PactSpec   `json:"spec"`
}

// PactSpec defines the policy and conditions for a pact.
type PactSpec struct {
	Policies           []Policy           `json:"policies"`
	CompletionConditions []CompletionCondition `json:"completion_conditions"`
	ExecutionPlan      string             `json:"execution_plan"`
}

// Policy defines a rule for what the pact allows.
type Policy struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Rules PolicyRules `json:"rules"`
}

// PolicyRules controls how the policy behaves.
type PolicyRules struct {
	AlwaysReview bool `json:"always_review,omitempty"`
}

// CompletionCondition defines when a pact is considered complete.
type CompletionCondition struct {
	Type      string `json:"type"`
	Threshold string `json:"threshold"`
}

// SubmitPactResponse is returned by POST /pacts/submit.
type SubmitPactResponse struct {
	PactID     string `json:"pact_id"`
	Status     string `json:"status"`
	ApprovalID string `json:"approval_id"`
	Message    string `json:"message"`
}

// GetPactResponse is returned by GET /pacts/{pact_id}.
type GetPactResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	APIKey string `json:"api_key"` // Only populated when status == "active"
}

// CreatePact submits a pact to lock funds for a bounty.
// In DevMode, creates a simulated pact without calling CAW API.
func (p *CAWProvider) CreatePact(ctx context.Context, bizID, walletID, fromAddr, amount, chainID, intent string, deadlineSeconds uint64, minReputation int) (*PactResult, error) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	// DevMode: simulate pact creation for testing without CAW
	if p.cfg.DevMode {
		p.log.Warn("CAW DevMode: simulating pact creation",
			zap.String("biz_id", bizID),
			zap.String("amount", amount),
		)
		return &PactResult{
			PactID: fmt.Sprintf("dev_pact_%s", bizID[:16]),
			Status: "active",
		}, nil
	}

	if p.cfg.WalletID == "" {
		return nil, fmt.Errorf("CAW wallet_id not configured — see docs/cobo_setup.md")
	}

	// Use the user's intent if provided, otherwise fall back to default
	pactIntent := intent
	if pactIntent == "" {
		pactIntent = fmt.Sprintf("Lock %s %s for bounty %s", amount, chainID, bizID[:16])
	}

	// Use a simple transfer policy: the pact allows transferring the locked amount
	// After human approval in CAW App, the agent can release the funds
	// Convert amount from wei to ETH for display in CAW App
	amountEth := "0"
	if a, err := strconv.ParseFloat(amount, 64); err == nil {
		amountEth = fmt.Sprintf("%.4f", a/1e18)
	}
	req := SubmitPactRequest{
		WalletID:       walletID,
		Name:           fmt.Sprintf("AEP Bounty Lock - %s (%s ETH)", bizID[:16], amountEth),
		Intent:         pactIntent,
		OriginalIntent: pactIntent,
		Amount:         amount,
		// MinReputation passed from handler below
		Spec: PactSpec{
			Policies: []Policy{
				{
					Name: "release-funds",
					Type: "transfer",
					Rules: PolicyRules{
						AlwaysReview: true, // Requires human approval in CAW App
					},
				},
			},
			CompletionConditions: []CompletionCondition{
				{
					Type:      "time_elapsed",
					Threshold: fmt.Sprintf("%d", deadlineSeconds),
				},
			},
			ExecutionPlan: fmt.Sprintf("Submit verified job result, then release %s to seller", amount),
		},
	}

	// Pass min reputation to CAW
	req.MinReputation = minReputation

	var resp SubmitPactResponse
	err := DoWithRetry(ctx, RetryConfig{
		MaxAttempts: p.cfg.Retry.MaxAttempts,
		BaseDelay:   p.cfg.Retry.BaseDelay,
		Log:         p.log,
		Operation:   "CAW.CreatePact",
	}, func(ctx context.Context) error {
		return p.client.doRequest(ctx, "POST", "/pacts/submit", req, &resp, map[string]string{"Idempotency-Key": bizID})
	})
	if err != nil {
		return nil, fmt.Errorf("caw create pact: %w", err)
	}

	p.log.Info("CAW pact submitted",
		zap.String("biz_id", bizID),
		zap.String("wallet_id", walletID),
		zap.String("amount", amount),
		zap.String("pact_id", resp.PactID),
		zap.String("status", resp.Status),
	)

	return &PactResult{
		PactID:     resp.PactID,
		Status:     resp.Status,
		ApprovalID: resp.ApprovalID,
	}, nil
}

// GetPactStatus queries the current status of a pact.
// When status == "active", the pact's API key is available for executing transactions.
func (p *CAWProvider) GetPactStatus(ctx context.Context, pactID string) (*GetPactResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()

	var resp GetPactResponse
	err := DoWithRetry(ctx, RetryConfig{
		MaxAttempts: p.cfg.Retry.MaxAttempts,
		BaseDelay:   p.cfg.Retry.BaseDelay,
		Log:         p.log,
		Operation:   "CAW.GetPactStatus",
	}, func(ctx context.Context) error {
		return p.client.doRequest(ctx, "GET", "/pacts/"+pactID, nil, &resp)
	})
	if err != nil {
		return nil, fmt.Errorf("get pact status: %w", err)
	}
	return &resp, nil
}

// ReleasePact releases/settles a pact by executing a transfer via the CAW API.
// Uses the pact's temporary API key to execute the transfer, which may trigger
// human approval on the CAW App if the policy requires always_review.
// Returns the transfer status ("pending_approval", "completed", etc.) and any error.
func (p *CAWProvider) ReleasePact(ctx context.Context, pactID, toAddr, amount, walletID, srcAddr string) (string, error) {
	if p.cfg.DevMode {
		p.log.Warn("CAW DevMode: simulating pact release", zap.String("pact_id", pactID))
		return "completed", nil
	}

	p.log.Info("CAW ReleasePact: fetching pact status", zap.String("pact_id", pactID))

	status, err := p.GetPactStatus(ctx, pactID)
	if err != nil {
		return "", fmt.Errorf("release pact: cannot verify status: %w", err)
	}

	if status.Status != "active" {
		return "", fmt.Errorf("release pact: pact %s is not active (status: %s)", pactID, status.Status)
	}

	if status.APIKey == "" {
		return "", fmt.Errorf("release pact: pact %s has no api_key — not activated", pactID)
	}

	p.log.Info("CAW ReleasePact: executing transfer via pact",
		zap.String("pact_id", pactID),
		zap.String("to_addr", toAddr),
		zap.String("amount", amount),
	)

	// Use the pact's temp API key to execute the transfer
	pactClient := NewCAWClient(status.APIKey, p.cfg.Sandbox, p.cfg.Timeout, p.log)

	var transferResp struct {
		RequestID  string      `json:"request_id"`
		RecordUUID string      `json:"record_uuid"`
		Status     json.Number `json:"status"`
	}
	err = DoWithRetry(ctx, RetryConfig{
		MaxAttempts: p.cfg.Retry.MaxAttempts,
		BaseDelay:   p.cfg.Retry.BaseDelay,
		Log:         p.log,
		Operation:   "CAW.TransferPact",
	}, func(ctx context.Context) error {
		return pactClient.doRequest(ctx, "POST", "/wallets/"+walletID+"/transfer", map[string]interface{}{
			"dst_addr":   toAddr,
			"src_addr":   srcAddr,
			"amount":     amount,
			"token_id":   "SETH",
			"request_id": fmt.Sprintf("release_%s_%d", pactID, time.Now().UnixMilli()),
		}, &transferResp)
	})
	if err != nil {
		return "", fmt.Errorf("release pact: transfer failed: %w", err)
	}

	p.log.Info("CAW pact released (settled)",
		zap.String("pact_id", pactID),
		zap.String("status", status.Status),
		zap.String("transfer_status", transferResp.Status.String()),
		zap.String("request_id", transferResp.RequestID),
	)
	return transferResp.Status.String(), nil
}

// ==================== FluxA Provider ====================

// FluxAProvider wraps the FluxA reputation API.
type FluxAProvider struct {
	cfg config.FluxAConfig
	log *zap.Logger
}

// NewFluxAProvider creates a FluxA provider.
func NewFluxAProvider(cfg config.FluxAConfig, log *zap.Logger) *FluxAProvider {
	return &FluxAProvider{cfg: cfg, log: log}
}

// FluxAResult holds the reputation check result.
type FluxAResult struct {
	Passed       bool `json:"passed"`
	Score        int  `json:"score"`
	FallbackUsed bool `json:"fallback_used"`
}

// CheckReputation checks seller reputation via FluxA.
// Red line: 3s timeout degrades gracefully, FallbackUsed=true on failure.
func (f *FluxAProvider) CheckReputation(ctx context.Context, sellerAddr string) (*FluxAResult, error) {
	if !f.cfg.Enabled {
		f.log.Info("FluxA disabled, bypassing reputation check")
		return &FluxAResult{Passed: true, FallbackUsed: true}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, f.cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", f.cfg.BaseURL+"/reputation/"+sellerAddr, nil)
	if err != nil {
		f.log.Warn("FluxA request creation failed, using fallback", zap.Error(err))
		return &FluxAResult{Passed: true, FallbackUsed: true}, nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.log.Warn("FluxA request failed, using fallback", zap.Error(err))
		return &FluxAResult{Passed: true, FallbackUsed: true}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.log.Warn("FluxA returned non-200, using fallback", zap.Int("status", resp.StatusCode))
		return &FluxAResult{Passed: true, FallbackUsed: true}, nil
	}

	var result FluxAResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		f.log.Warn("FluxA decode failed, using fallback", zap.Error(err))
		return &FluxAResult{Passed: true, FallbackUsed: true}, nil
	}

	f.log.Info("FluxA reputation check",
		zap.String("seller", sellerAddr),
		zap.Bool("passed", result.Passed),
		zap.Int("score", result.Score),
	)
	return &result, nil
}

// ==================== IPFS Provider ====================

// IPFSProvider wraps the IPFS/Pinata upload API.
type IPFSProvider struct {
	cfg     config.IPFSConfig
	httpCli *http.Client
	log     *zap.Logger
}

// NewIPFSProvider creates an IPFS provider.
func NewIPFSProvider(cfg config.IPFSConfig, log *zap.Logger) *IPFSProvider {
	return &IPFSProvider{
		cfg:     cfg,
		httpCli: &http.Client{Timeout: cfg.Timeout},
		log:     log,
	}
}

// pinataUpload uploads data to Pinata IPFS and returns the CID.
func (p *IPFSProvider) pinataUpload(ctx context.Context, data []byte) (string, error) {
	// Build multipart form: Pinata's pinFileToIPFS accepts a file
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create form file field
	part, err := writer.CreateFormFile("file", "delivery.txt")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("write file data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.pinata.cloud/pinning/pinFileToIPFS", body)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.cfg.PinataJWT)

	resp, err := p.httpCli.Do(req)
	if err != nil {
		return "", fmt.Errorf("pinata request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("pinata error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		IpfsHash  string `json:"IpfsHash"`
		PinSize   int    `json:"PinSize"`
		Timestamp string `json:"Timestamp"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse pinata response: %w", err)
	}
	if result.IpfsHash == "" {
		return "", fmt.Errorf("pinata returned empty IpfsHash")
	}

	return "ipfs://" + result.IpfsHash, nil
}

// UploadResult uploads data to IPFS via Pinata and returns the CID.
// Falls back to local file storage when Pinata is unavailable.
func (p *IPFSProvider) UploadResult(ctx context.Context, data []byte) (string, error) {
	if p.cfg.PinataJWT == "" {
		return p.localUpload(data)
	}

	cid, err := p.pinataUpload(ctx, data)
	if err != nil {
		p.log.Warn("Pinata upload failed, falling back to local storage", zap.Error(err))
		return p.localUpload(data)
	}
	return cid, nil
}

// localUpload saves data to the local filesystem as a fallback when Pinata is unavailable.
func (p *IPFSProvider) localUpload(data []byte) (string, error) {
	hash := sha256.Sum256(data)
	cid := fmt.Sprintf("local://%x", hash[:16])
	dir := "/tmp/aep-ipfs-fallback"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create local ipfs dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%x", hash[:8]))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("local ipfs write: %w", err)
	}
	p.log.Info("Saved to local IPFS fallback", zap.String("cid", cid), zap.String("path", path))
	return cid, nil
}
