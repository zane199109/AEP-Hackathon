package model

import (
	"time"
)

// BountyStatus matches the Solidity enum.
type BountyStatus string

const (
	StatusOpen      BountyStatus = "Open"
	StatusAssigned  BountyStatus = "Assigned"
	StatusSubmitted BountyStatus = "Submitted"
	StatusDisputed  BountyStatus = "Disputed"
	StatusVerified  BountyStatus = "Verified"
	StatusSlashed   BountyStatus = "Slashed"
	StatusRefunded  BountyStatus = "Refunded"
)

// Bounty represents a row in the bounties table.
type Bounty struct {
	ID              int64        `gorm:"primaryKey;autoIncrement" db:"id" json:"id"`
	JobID           uint64       `gorm:"uniqueIndex;not null" db:"job_id" json:"job_id"`
	BuyerAddr       string       `gorm:"type:bytea;not null" db:"buyer_addr" json:"buyer_addr"`
	SellerAddr      string       `gorm:"type:bytea;default:null" db:"seller_addr" json:"seller_addr"`
	Amount          string       `gorm:"type:numeric(78,0);not null" db:"amount" json:"amount"`
	Deadline        time.Time    `gorm:"not null" db:"deadline" json:"deadline"`
	ResultHash      string       `gorm:"default:''" db:"result_hash" json:"result_hash"`
	Status          BountyStatus `gorm:"type:varchar(20);default:'Open';index" db:"status" json:"status"`
	PactID          string       `gorm:"default:'';index" db:"pact_id" json:"pact_id"`
	BuyerApproval   bool         `gorm:"default:false" db:"buyer_approval" json:"buyer_approval"`
	ParentBountyID  *uint64      `gorm:"default:null" db:"parent_bounty_id" json:"parent_bounty_id"`
	Depth           int          `gorm:"default:0" db:"depth" json:"depth"`
	BuyerWalletID   string       `gorm:"default:''" db:"buyer_wallet_id" json:"buyer_wallet_id"`
	SellerWalletID  string       `gorm:"default:''" db:"seller_wallet_id" json:"seller_wallet_id"`
	RuleParams      string       `gorm:"type:jsonb;default:'[]'" db:"rule_params" json:"rule_params"`
	CreatedAt       time.Time    `gorm:"autoCreateTime" db:"created_at" json:"created_at"`
	UpdatedAt       time.Time    `gorm:"autoUpdateTime" db:"updated_at" json:"updated_at"`
}

// TableName overrides the default table name for Bounty.
func (Bounty) TableName() string {
	return "bounties"
}

// ProcessedEvent represents the L2 dedup table.
type ProcessedEvent struct {
	TxHash      string    `gorm:"primaryKey;type:bytea" db:"tx_hash" json:"tx_hash"`
	LogIndex    uint      `gorm:"primaryKey;autoIncrement:false" db:"log_index" json:"log_index"`
	EventType   string    `gorm:"type:varchar(50)" db:"event_type" json:"event_type"`
	JobID       uint64    `gorm:"index" db:"job_id" json:"job_id"`
	ProcessedAt time.Time `gorm:"autoCreateTime" db:"processed_at" json:"processed_at"`
}

// TableName overrides the default table name for ProcessedEvent.
func (ProcessedEvent) TableName() string {
	return "processed_events"
}

// PostBountyRequest is the JSON body for POST /api/bounty.
type PostBountyRequest struct {
	Buyer          string `json:"buyer" validate:"required"`
	Amount         string `json:"amount" validate:"required"`
	Deadline       string `json:"deadline" validate:"required"`
	MinReputation  int    `json:"min_reputation"`
	Intent         string `json:"intent"`
	DemoSlash      bool   `json:"demo_slash"`
}

// PostBountyResponse is returned by POST /api/bounty.
type PostBountyResponse struct {
	JobID      uint64 `json:"job_id"`
	PactID     string `json:"pact_id"`
	PactStatus string `json:"pact_status"`
	Status     string `json:"status"`
}

// ClaimBountyResponse is returned on successful claim.
type ClaimBountyResponse struct {
	JobID  uint64 `json:"job_id"`
	Seller string `json:"seller"`
	Status string `json:"status"`
}

// ErrorResponse is a standard error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// SubBountyRequest is the JSON body for POST /api/bounty/{id}/sub-bounty.
type SubBountyRequest struct {
	Seller   string `json:"seller" validate:"required"`
	Amount   string `json:"amount" validate:"required"`
	Intent   string `json:"intent"`
	WalletID string `json:"wallet_id"`
}

// SubBountyResponse is returned by POST /api/bounty/{id}/sub-bounty.
type SubBountyResponse struct {
	JobID    uint64 `json:"job_id"`
	ParentID uint64 `json:"parent_id"`
	Depth    int    `json:"depth"`
	Amount   string `json:"amount"`
	Status   string `json:"status"`
}
