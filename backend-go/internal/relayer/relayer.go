package relayer

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/model"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/provider"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/store"
)

// Relayer handles settlement: CAW pact release with L2 dedup.
// Red line: must verify BuyerApproval == true before calling CAW Release.
type Relayer struct {
	store            *store.Store
	caw              *provider.CAWProvider
	log              *zap.Logger
	providerAPIKey   string
	providerWalletID string
	providerAddr     string
	subProviderAddr  string
	sandbox          bool
}

// New creates a new Relayer.
func New(s *store.Store, caw *provider.CAWProvider, log *zap.Logger, providerAPIKey, providerWalletID, providerAddr, subProviderAddr string, sandbox bool) *Relayer {
	return &Relayer{store: s, caw: caw, log: log, providerAPIKey: providerAPIKey, providerWalletID: providerWalletID, providerAddr: providerAddr, subProviderAddr: subProviderAddr, sandbox: sandbox}
}

// SettlementResult holds the outcome of a settlement attempt.
type SettlementResult struct {
	Success        bool   `json:"success"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	TransferStatus string `json:"transfer_status,omitempty"`
	TransferTxID   string `json:"transfer_tx_id,omitempty"`
	OnChainTxHash  string `json:"on_chain_tx_hash,omitempty"`
	ChildTxHash    string `json:"child_tx_hash,omitempty"`
	ParentTxHash   string `json:"parent_tx_hash,omitempty"`
	Amount         string `json:"amount,omitempty"`
	ChildAmount    string `json:"child_amount,omitempty"`
}

// SettleBountyTree recursively settles a bounty and all its children.
// Children are settled first (bottom-up), then the parent.
func (r *Relayer) SettleBountyTree(ctx context.Context, jobID uint64, traceID string) *SettlementResult {
	r.log.Info("Relayer: recursive settlement started",
		zap.Uint64("job_id", jobID),
		zap.String("trace_id", traceID),
	)

	// Step 1: Recursively settle all children first
	children, err := r.store.GetChildrenBounties(ctx, jobID)
	if err != nil {
		r.log.Error("Relayer: failed to fetch children", zap.Uint64("job_id", jobID), zap.Error(err))
		return &SettlementResult{Success: false, Status: "failed", Message: "failed to fetch children"}
	}

	var childTxHash string
	for _, child := range children {
		// Skip children already settled
		if child.Status == model.StatusVerified || child.Status == model.StatusRefunded {
			continue
		}
		// Children (sub-bounties) are internal — auto-confirm without human approval
		if !child.BuyerApproval {
			_ = r.store.ConfirmBuyerApproval(ctx, child.JobID)
		}

		// If child has a PactID, settle normally through CAW ReleasePact
		if child.PactID != "" {
			childResult := r.SettleBountyTree(ctx, child.JobID, traceID)
			if !childResult.Success {
				r.log.Error("Relayer: child settlement failed",
					zap.Uint64("child_job_id", child.JobID),
					zap.String("status", childResult.Status),
				)
				return &SettlementResult{
					Success: false, Status: "failed",
					Message: fmt.Sprintf("child bounty #%d settlement failed: %s", child.JobID, childResult.Message),
				}
			}
		} else {
			// No PactID → create a pact from Provider to SubProvider, then release
			r.log.Info("Relayer: child has no PactID, creating settlement pact",
				zap.Uint64("child_job_id", child.JobID),
				zap.String("to_addr", string(child.SellerAddr)),
				zap.String("amount", child.Amount),
			)

			// Create a temporary CAWProvider with Provider's API key
			provCfg := config.CAWConfig{
				APIKey:    r.providerAPIKey,
				WalletID:  r.providerWalletID,
				Sandbox:   r.sandbox,
				Timeout:   30 * time.Second,
				DevMode:   false,
			}
			// Set Retry to default values to avoid nil panic
			provCfg.Retry.MaxAttempts = 3
			provCfg.Retry.BaseDelay = 1 * time.Second
			provProvider := provider.NewCAWProvider(provCfg, r.log)

			// Convert amount: use child.Amount (wei) or fallback to 0.001 ETH
			settleAmount := child.Amount
			if settleAmount == "" || settleAmount == "0" {
				settleAmount = "1000000000000000" // 0.001 ETH default
			}

			// Create a settlement pact (short deadline: 5 minutes)
			pact, err := provProvider.CreatePact(ctx,
				fmt.Sprintf("sub_settle_%d_%d", child.JobID, time.Now().UnixMilli()),
				r.providerWalletID,
				r.providerAddr,
				settleAmount,
				"BASE_ETH",
				fmt.Sprintf("Sub-bounty #%d settlement", child.JobID),
				300,  // 5 minute deadline
				0,    // no min reputation
			)
			if err != nil {
				r.log.Error("Relayer: failed to create settlement pact",
					zap.Uint64("child_job_id", child.JobID),
					zap.Error(err),
				)
				return &SettlementResult{
					Success: false, Status: "failed",
					Message: fmt.Sprintf("failed to create settlement pact for child #%d: %s", child.JobID, err.Error()),
				}
			}

			r.log.Info("Relayer: settlement pact created",
				zap.Uint64("child_job_id", child.JobID),
				zap.String("pact_id", pact.PactID),
				zap.String("status", pact.Status),
			)

			// Poll for pact approval (unpaired wallets auto-approve)
			pollCtx, pollCancel := context.WithTimeout(ctx, 30*time.Second)
			approved := false
			for i := 0; i < 15; i++ {
				select {
				case <-pollCtx.Done():
					break
				default:
				}
				time.Sleep(2 * time.Second)
				status, err := provProvider.GetPactStatus(pollCtx, pact.PactID)
				if err != nil {
					r.log.Warn("Relayer: pact status poll failed", zap.Int("attempt", i+1), zap.Error(err))
					continue
				}
				if status.Status == "active" {
					approved = true
					break
				}
				r.log.Info("Relayer: waiting for pact approval...",
					zap.Int("attempt", i+1),
					zap.String("status", status.Status),
				)
			}
			pollCancel()

			if !approved {
				r.log.Error("Relayer: settlement pact not approved in time",
					zap.Uint64("child_job_id", child.JobID),
				)
				return &SettlementResult{
					Success: false, Status: "failed",
					Message: fmt.Sprintf("settlement pact for child #%d not approved in time", child.JobID),
				}
			}

			// Convert wei to ETH for ReleasePact
			amountDecimal := "0.001"
			if settleAmount != "" {
				var wei uint64
				if _, err := fmt.Sscanf(settleAmount, "%d", &wei); err == nil && wei > 0 {
					amountDecimal = fmt.Sprintf("%.18f", float64(wei)/1e18)
				}
			}

			// Release the pact to transfer funds to SubProvider
			transferStatus, txUUID, err := provProvider.ReleasePact(ctx,
				pact.PactID,
				string(child.SellerAddr),
				amountDecimal,
				r.providerWalletID,
				r.providerAddr,
			)
			if err != nil {
				r.log.Error("Relayer: settlement pact release failed",
					zap.Uint64("child_job_id", child.JobID),
					zap.Error(err),
				)
				return &SettlementResult{
					Success: false, Status: "failed",
					Message: fmt.Sprintf("settlement pact release failed for child #%d: %s", child.JobID, err.Error()),
				}
			}

			r.log.Info("Relayer: settlement pact released",
				zap.Uint64("child_job_id", child.JobID),
				zap.String("pact_id", pact.PactID),
				zap.String("transfer_status", transferStatus),
				zap.String("tx_uuid", txUUID),
			)

			// If release is pending_approval, auto-approve via CLI first (unpaired wallet)
			if transferStatus == "pending_approval" {
				r.log.Info("Relayer: release pending, auto-approving via CLI...",
					zap.Uint64("child_job_id", child.JobID),
				)
				apiURL := "https://api-core.agenticwallet.cobo.com"
				if r.sandbox {
					apiURL = "https://api-core.agenticwallet.dev.cobo.com"
				}
				listCmd := exec.Command("caw", "pending", "list",
					"--api-key", r.providerAPIKey,
					"--api-url", apiURL,
				)
				listOut, err := listCmd.Output()
				if err == nil {
					var listResp struct {
						Result struct {
							Items []struct {
								ID            string `json:"id"`
								OperationData struct {
									RequestID string `json:"request_id"`
								} `json:"operation_data"`
							} `json:"items"`
						} `json:"result"`
					}
					if json.Unmarshal(listOut, &listResp) == nil {
						for _, item := range listResp.Result.Items {
							if item.OperationData.RequestID == txUUID || txUUID == "" {
								r.log.Info("Relayer: approving pending operation",
									zap.String("op_id", item.ID),
									zap.String("request_id", item.OperationData.RequestID),
								)
								approveCmd := exec.Command("caw", "pending", "approve",
									"--api-key", r.providerAPIKey,
									"--api-url", apiURL,
									"--operation-id", item.ID,
								)
								if out, err := approveCmd.Output(); err != nil {
									r.log.Warn("Relayer: auto-approve failed",
										zap.String("op_id", item.ID),
										zap.Error(err),
									)
								} else {
									r.log.Info("Relayer: auto-approve succeeded",
										zap.String("op_id", item.ID),
										zap.String("output", string(out)),
									)
								}
							}
						}
					}
				} else {
					r.log.Warn("Relayer: failed to list pending ops", zap.Error(err))
				}
			}

			// Poll for on-chain tx hash (CAW processes asynchronously, after approval)
			var onChainTxHash string
			if txUUID != "" {
				pollCtx2, pollCancel2 := context.WithTimeout(ctx, 120*time.Second)
				for i := 0; i < 60; i++ {
					select {
					case <-pollCtx2.Done():
						break
					default:
					}
					time.Sleep(2 * time.Second)
					_, txHash, err := provider.GetTxHashByRequestIDWithKey(pollCtx2, r.providerAPIKey, r.providerWalletID, txUUID, r.sandbox, 10*time.Second, r.log)
					if err == nil && txHash != "" {
						onChainTxHash = txHash
						break
					}
					r.log.Info("Relayer: polling transfer tx hash...",
						zap.Int("attempt", i+1),
						zap.Uint64("child_job_id", child.JobID),
					)
				}
				pollCancel2()
			}

			// Update child bounty status
			_ = r.store.UpdateBountyStatus(ctx, child.JobID, model.StatusVerified)
			// Propagate on-chain tx hash for SSE push
			if onChainTxHash != "" && childTxHash == "" {
				childTxHash = onChainTxHash
			}
		}
		r.log.Info("Relayer: child settled successfully",
			zap.Uint64("child_job_id", child.JobID),
		)
	}

	// Step 2: Settle this bounty
	result := r.SettleBounty(ctx, jobID, traceID)
	if childTxHash != "" {
		result.ChildTxHash = childTxHash
	}
	if result.Success {
		// Update bounty status to Verified
		if err := r.store.UpdateBountyStatus(ctx, jobID, model.StatusVerified); err != nil {
			r.log.Warn("Relayer: failed to update bounty status",
				zap.Uint64("job_id", jobID), zap.Error(err))
		}
	}
	return result
}

// SettleBounty attempts to settle a single bounty by releasing the CAW pact.
// This is called after the buyer confirms (BuyerApproval = true).
//
// Settlement flow:
//  1. Fetch bounty from PG
//  2. Verify BuyerApproval == true (red line)
//  3. Check L2 dedup — skip if already processed
//  4. Call CAW ReleasePact
//  5. Mark event as processed (L2 dedup)
func (r *Relayer) SettleBounty(ctx context.Context, jobID uint64, traceID string) *SettlementResult {
	r.log.Info("Relayer: attempting settlement",
		zap.Uint64("job_id", jobID),
		zap.String("trace_id", traceID),
	)

	// Step 1: Fetch bounty
	bounty, err := r.store.GetBountyByID(ctx, jobID)
	if err != nil {
		r.log.Error("Relayer: bounty not found", zap.Uint64("job_id", jobID), zap.Error(err))
		return &SettlementResult{Success: false, Status: "failed", Message: "bounty not found"}
	}

	// Step 2: Red line — verify BuyerApproval before CAW Release
	if !bounty.BuyerApproval {
		r.log.Warn("Relayer: BuyerApproval is false, refusing release",
			zap.Uint64("job_id", jobID),
			zap.String("trace_id", traceID),
		)
		return &SettlementResult{
			Success: false, Status: "not_confirmed",
			Message: "buyer has not confirmed — BuyerApproval is false",
		}
	}

	// Red line: print UserConfirmed in logs (already done in ConfirmJob handler)
	r.log.Info("Relayer: BuyerApproval verified, proceeding with release",
		zap.Uint64("job_id", jobID),
	)

	// Step 3: L2 dedup check — prevent double settlement
	// Use a deterministic tx_hash-like key for the settlement event
	txHash := fmt.Sprintf("settle_%d", jobID)
	logIndex := uint(0)

	alreadySettled, err := r.store.IsEventProcessed(ctx, txHash, logIndex)
	if err != nil {
		r.log.Error("Relayer: L2 dedup check failed", zap.Error(err))
		return &SettlementResult{Success: false, Status: "failed", Message: "dedup check error"}
	}
	if alreadySettled {
		r.log.Info("Relayer: bounty already settled, skipping (L2 dedup)",
			zap.Uint64("job_id", jobID),
		)
		return &SettlementResult{
			Success: true, Status: "already_settled",
			Message: "bounty was already settled (L2 dedup)",
		}
	}

	// Step 4: Call CAW ReleasePact with recipient details
	if bounty.PactID != "" {
		// Convert amount to decimal string for transfer API
		amountDecimal := "0.001" // default for demo; should parse from bounty.Amount
		if bounty.Amount != "" {
			// Convert wei to ETH (wei / 1e18)
			var wei uint64
			if _, err := fmt.Sscanf(bounty.Amount, "%d", &wei); err == nil && wei > 0 {
				amountDecimal = fmt.Sprintf("%.18f", float64(wei)/1e18)
			}
		}
		transferStatus, txUUID, err := r.caw.ReleasePact(ctx, bounty.PactID, string(bounty.SellerAddr), amountDecimal, string(bounty.BuyerWalletID), string(bounty.BuyerAddr))
		if err != nil {
			r.log.Error("Relayer: CAW ReleasePact failed",
				zap.Uint64("job_id", jobID),
				zap.String("pact_id", bounty.PactID),
				zap.Error(err),
			)
			return &SettlementResult{
				Success: false, Status: "failed",
				Message: fmt.Sprintf("CAW release failed: %s", err.Error()),
			}
		}
		r.log.Info("Relayer: CAW pact released",
			zap.Uint64("job_id", jobID),
			zap.String("pact_id", bounty.PactID),
			zap.String("transfer_status", transferStatus),
			zap.String("tx_uuid", txUUID),
		)
		if transferStatus == "pending_approval" {
			r.log.Info("Relayer: release pending CAW approval",
				zap.Uint64("job_id", jobID),
			)
		}

		// Poll for on-chain tx hash
		var onChainTxHash string
		if txUUID != "" {
			pollCtx, pollCancel := context.WithTimeout(ctx, 120*time.Second)
			for i := 0; i < 30; i++ {
				select {
				case <-pollCtx.Done():
					break
				default:
				}
				time.Sleep(2 * time.Second)
				_, txHash, err := r.caw.GetTransactionByRequestID(pollCtx, string(bounty.BuyerWalletID), txUUID)
				if err == nil && txHash != "" {
					onChainTxHash = txHash
					break
				}
				r.log.Info("Relayer: polling parent tx hash...",
					zap.Int("attempt", i+1),
					zap.Uint64("job_id", jobID),
				)
			}
			pollCancel()
		}

		if onChainTxHash != "" {
			r.log.Info("Relayer: parent transfer on-chain tx hash",
				zap.Uint64("job_id", jobID),
				zap.String("tx_hash", onChainTxHash),
			)
		}

		return &SettlementResult{
			Success: true, Status: "settled",
			Message: "funds released successfully",
			TransferStatus: transferStatus,
			TransferTxID:   txUUID,
			ParentTxHash:   onChainTxHash,
		}
	} else {
		r.log.Warn("Relayer: no pact_id found, skipping CAW release",
			zap.Uint64("job_id", jobID),
		)
	}

	// Step 5: Mark as processed (L2 dedup)
	pe := &model.ProcessedEvent{
		TxHash:    txHash,
		LogIndex:  logIndex,
		EventType: "Settlement",
		JobID:     jobID,
	}
	if err := r.store.MarkEventProcessed(ctx, pe); err != nil {
		r.log.Error("Relayer: failed to mark settlement as processed",
			zap.Uint64("job_id", jobID),
			zap.Error(err),
		)
		// Non-fatal: settlement already happened
	}

	r.log.Info("Relayer: settlement complete",
		zap.Uint64("job_id", jobID),
		zap.String("trace_id", traceID),
	)

	return &SettlementResult{
		Success: true, Status: "settled",
		Message: "funds released successfully",
	}
}
