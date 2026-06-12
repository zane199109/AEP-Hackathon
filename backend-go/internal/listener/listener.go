package listener

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/internal/model"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/store"
)

// Listener monitors on-chain events via WebSocket with Polling fallback.
type Listener struct {
	store  *store.Store
	rdb    *redis.Client
	log    *zap.Logger
	rpcURL string
	quit   chan struct{}
}

// Event represents a parsed chain event.
type Event struct {
	TxHash    string
	LogIndex  uint
	EventType string
	JobID     *big.Int
	Raw       interface{}
}

// New creates a new Listener.
func New(s *store.Store, rdb *redis.Client, rpcURL string, log *zap.Logger) *Listener {
	return &Listener{
		store:  s,
		rdb:    rdb,
		log:    log,
		rpcURL: rpcURL,
		quit:   make(chan struct{}),
	}
}

// Start begins listening for events. On WS disconnect, falls back to polling.
func (l *Listener) Start(ctx context.Context, events chan<- *Event) {
	go l.runWebSocket(ctx, events)
}

// Stop signals the listener to shut down.
func (l *Listener) Stop() {
	close(l.quit)
}

func (l *Listener) runWebSocket(ctx context.Context, events chan<- *Event) {
	defer func() {
		l.log.Warn("WS listener ended, falling back to polling")
		l.runPolling(ctx, events)
	}()

	// TODO: 使用 ethclient.Dial(l.rpcURL) 建立 WS 连接
	l.log.Info("WS listener starting", zap.String("rpc", l.rpcURL))

	select {
	case <-ctx.Done():
		return
	case <-l.quit:
		return
	}
}

// runPolling is the fallback when WebSocket disconnects.
// 动态退避 1-5s
func (l *Listener) runPolling(ctx context.Context, events chan<- *Event) {
	l.log.Info("Polling fallback started")

	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.quit:
			return
		default:
		}

		// TODO: poll new blocks via ethclient
		// Simulate: single backoff pass
		jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
		sleepDuration := backoff + jitter
		if sleepDuration > 5*time.Second {
			sleepDuration = 5*time.Second + jitter
		}
		time.Sleep(sleepDuration)
	}
}

// ProcessEvent handles an incoming event with L1+L2 dedup.
// L1: Redis SETNX ; L2: PG processed_events
func (l *Listener) ProcessEvent(ctx context.Context, evt *Event) error {
	// L1 dedup (Redis)
	dedupKey := store.EventDedupKey(evt.TxHash, evt.LogIndex)
	firstSeen, err := l.rdb.SetNX(ctx, dedupKey, "1", 24*time.Hour).Result()
	if err != nil {
		return fmt.Errorf("l1 dedup error: %w", err)
	}
	if !firstSeen {
		l.log.Info("L1 dedup: event already seen",
			zap.String("tx_hash", evt.TxHash),
			zap.Uint("log_index", evt.LogIndex),
		)
		return nil
	}

	// L2 dedup (PG)
	txHashStr := evt.TxHash
	alreadyProcessed, err := l.store.IsEventProcessed(ctx, txHashStr, evt.LogIndex)
	if err != nil {
		return fmt.Errorf("l2 dedup check error: %w", err)
	}
	if alreadyProcessed {
		l.log.Info("L2 dedup: event already processed",
			zap.String("tx_hash", evt.TxHash),
			zap.Uint("log_index", evt.LogIndex),
		)
		return nil
	}

	// 记录 L2
	pe := &model.ProcessedEvent{
		TxHash:    evt.TxHash,
		LogIndex:  evt.LogIndex,
		EventType: evt.EventType,
		JobID:     evt.JobID.Uint64(),
	}
	if err := l.store.MarkEventProcessed(ctx, pe); err != nil {
		return fmt.Errorf("mark event processed error: %w", err)
	}

	return nil
}
