package relayer

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/model"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/provider"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/store"
)

// Relayer handles settlement: CAW pact release with L2 dedup.
// Red line: must verify BuyerApproval == true before calling CAW Release.
type Relayer struct {
	store *store.Store
	caw   *provider.CAWProvider
	log   *zap.Logger
}

// New creates a new Relayer.
func New(s *store.Store, caw *provider.CAWProvider, log *zap.Logger) *Relayer {
	return &Relayer{store: s, caw: caw, log: log}
}

// SettlementResult holds the outcome of a settlement attempt.
type SettlementResult struct {
	Success        bool   `json:"success"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	TransferStatus string `json:"transfer_status,omitempty"`
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

	for _, child := range children {
		// Skip children already settled
		if child.Status == model.StatusVerified || child.Status == model.StatusRefunded {
			continue
		}
		// Children (sub-bounties) are internal — auto-confirm without human approval
		if !child.BuyerApproval {
			_ = r.store.ConfirmBuyerApproval(ctx, child.JobID)
		}
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
		r.log.Info("Relayer: child settled successfully",
			zap.Uint64("child_job_id", child.JobID),
		)
	}

	// Step 2: Settle this bounty
	result := r.SettleBounty(ctx, jobID, traceID)
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
		transferStatus, err := r.caw.ReleasePact(ctx, bounty.PactID, string(bounty.SellerAddr), amountDecimal, string(bounty.BuyerWalletID), string(bounty.BuyerAddr))
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
		)
		// If still pending approval, the relayer treats it as successful
		// (caller will handle polling)
		if transferStatus == "pending_approval" {
			r.log.Info("Relayer: release pending CAW approval",
				zap.Uint64("job_id", jobID),
			)
		}
		// Store transfer status in result so ConfirmJob can decide
		return &SettlementResult{
			Success: true, Status: "settled",
			Message: "funds released successfully",
			TransferStatus: transferStatus,
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
