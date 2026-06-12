package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/model"
)

// Store wraps PG (via GORM) and Redis connections.
type Store struct {
	DB  *gorm.DB
	RD  *redis.Client
	Log *zap.Logger
}

// New creates a new Store from config.
func New(ctx context.Context, cfg *config.Config, log *zap.Logger) (*Store, error) {
	db, err := gorm.Open(postgres.Open(cfg.DB.DSN), &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm db: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get underlying sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpen)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxIdle)

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &Store{DB: db, RD: rdb, Log: log}, nil
}

// Close tears down connections.
func (s *Store) Close() {
	sqlDB, err := s.DB.DB()
	if err == nil {
		sqlDB.Close()
	}
	s.RD.Close()
}

// AutoMigrate runs GORM auto-migration for all models.
func (s *Store) AutoMigrate() error {
	return s.DB.AutoMigrate(&model.Bounty{}, &model.ProcessedEvent{})
}

// ==================== PG: Bounties ====================

// CreateBounty inserts a new bounty row.
func (s *Store) CreateBounty(ctx context.Context, b *model.Bounty) error {
	return s.DB.WithContext(ctx).Create(b).Error
}

// GetBountyByID fetches a bounty by job_id.
func (s *Store) GetBountyByID(ctx context.Context, jobID uint64) (*model.Bounty, error) {
	var b model.Bounty
	err := s.DB.WithContext(ctx).Where("job_id = ?", jobID).First(&b).Error
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// ClaimBountyWithLock atomically claims a bounty using PG row-level lock.
func (s *Store) ClaimBountyWithLock(ctx context.Context, jobID uint64, sellerAddr string) (*model.Bounty, error) {
	tx := s.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("begin tx: %w", tx.Error)
	}
	defer tx.Rollback()

	var b model.Bounty
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("job_id = ?", jobID).First(&b).Error; err != nil {
		return nil, err
	}

	if b.Status != model.StatusOpen {
		return nil, fmt.Errorf("bounty %d not open, current status: %s", jobID, b.Status)
	}

	result := tx.Model(&b).Where("job_id = ? AND status = ?", jobID, model.StatusOpen).
		Updates(map[string]interface{}{
			"seller_addr": sellerAddr,
			"status":      string(model.StatusAssigned),
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("bounty %d already assigned", jobID)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	b.SellerAddr = sellerAddr
	b.Status = model.StatusAssigned
	return &b, nil
}

// ConfirmBuyerApproval sets buyer_approval = true.
func (s *Store) ConfirmBuyerApproval(ctx context.Context, jobID uint64) error {
	return s.DB.WithContext(ctx).Model(&model.Bounty{}).
		Where("job_id = ?", jobID).
		Update("buyer_approval", true).Error
}

// CreateSubBounty creates a child bounty linked to a parent.
func (s *Store) CreateSubBounty(ctx context.Context, b *model.Bounty) error {
	return s.DB.WithContext(ctx).Create(b).Error
}

// GetChildrenBounties fetches all direct child bounties of a parent.
func (s *Store) GetChildrenBounties(ctx context.Context, parentJobID uint64) ([]*model.Bounty, error) {
	var bounties []*model.Bounty
	err := s.DB.WithContext(ctx).Where("parent_bounty_id = ?", parentJobID).
		Order("id ASC").Find(&bounties).Error
	return bounties, err
}

// UpdateBountyStatus updates the status of a bounty.
func (s *Store) UpdateBountyStatus(ctx context.Context, jobID uint64, status model.BountyStatus) error {
	return s.DB.WithContext(ctx).Model(&model.Bounty{}).
		Where("job_id = ?", jobID).
		Update("status", string(status)).Error
}

// UpdateBountyRuleParams updates the rule_params JSONB field for a bounty.
func (s *Store) UpdateBountyRuleParams(ctx context.Context, jobID uint64, ruleParams string) error {
	return s.DB.WithContext(ctx).Model(&model.Bounty{}).
		Where("job_id = ?", jobID).
		Update("rule_params", ruleParams).Error
}

// GetAllBounties fetches all bounties (for admin/debug).
func (s *Store) GetAllBounties(ctx context.Context) ([]*model.Bounty, error) {
	var bounties []*model.Bounty
	err := s.DB.WithContext(ctx).Order("id DESC").Find(&bounties).Error
	return bounties, err
}

// ==================== PG: Processed Events (L2 dedup) ====================

// IsEventProcessed checks if an event was already processed (L2 dedup).
func (s *Store) IsEventProcessed(ctx context.Context, txHash string, logIndex uint) (bool, error) {
	var count int64
	err := s.DB.WithContext(ctx).Model(&model.ProcessedEvent{}).
		Where("tx_hash = ? AND log_index = ?", txHash, logIndex).
		Count(&count).Error
	return count > 0, err
}

// MarkEventProcessed inserts a processed event record.
func (s *Store) MarkEventProcessed(ctx context.Context, evt *model.ProcessedEvent) error {
	return s.DB.WithContext(ctx).Create(evt).Error
}

// ==================== Redis: L1 Dedup ====================

const l1DedupTTL = 24 * time.Hour

// EventDedupKey returns the Redis key for L1 event dedup.
func EventDedupKey(txHash string, logIndex uint) string {
	return fmt.Sprintf("evt:%s:%d", txHash, logIndex)
}

// TryAcquireEventDedup tries to acquire the L1 dedup lock. Returns true if first-seen.
func (s *Store) TryAcquireEventDedup(ctx context.Context, txHash string, logIndex uint) (bool, error) {
	return s.RD.SetNX(ctx, EventDedupKey(txHash, logIndex), "1", l1DedupTTL).Result()
}

// ==================== Redis: Claim Lock ====================

const claimLockTTL = 120 * time.Second

// ClaimLockKey returns the Redis key for bounty claim lock.
func ClaimLockKey(jobID uint64) string {
	return fmt.Sprintf("claim:lock:%d", jobID)
}

// TryAcquireClaimLock tries to acquire Redis SETNX lock for claiming.
func (s *Store) TryAcquireClaimLock(ctx context.Context, jobID uint64) (bool, error) {
	return s.RD.SetNX(ctx, ClaimLockKey(jobID), "1", claimLockTTL).Result()
}

// ReleaseClaimLock releases the Redis claim lock.
func (s *Store) ReleaseClaimLock(ctx context.Context, jobID uint64) error {
	return s.RD.Del(ctx, ClaimLockKey(jobID)).Err()
}
