package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/zane199109/AEP-Hackathon/backend-go/api"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/engine"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/listener"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/provider"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/relayer"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/rule"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/store"
)

func main() {
	cfgPath := flag.String("config", "conf/config.yaml", "path to config file")
	offline := flag.Bool("offline", false, "run in offline mode (read mock_events.jsonl)")
	flag.Parse()

	// initialize Logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	// load config
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.String("path", *cfgPath), zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//initialize Store (PG + Redis)
	s, err := store.New(ctx, cfg, logger)
	if err != nil {
		logger.Fatal("failed to initialize store", zap.Error(err))
	}
	defer s.Close()

	// Initialize Providers
	cawProvider := provider.NewCAWProvider(cfg.CAW, logger)
	fluxaProvider := provider.NewFluxAProvider(cfg.FluxA, logger)
	ipfsProvider := provider.NewIPFSProvider(cfg.IPFS, logger)

	// Initialize on-chain reputation provider
	onChainRep := provider.NewOnChainReputationProvider(cfg.Chain, logger)

	// Initialize dual-track evaluation engine
	ruleEngine := engine.NewRuleEvaluator(logger)

	// New template-based rule engine
	templateEngine := rule.NewRuleEngine()
	llmEngine := engine.NewLLMEvaluator(cfg.OpenAI, logger)
	agg := engine.NewAggregator(logger)

	// Initialize settlement relayer
	relayerSvc := relayer.New(s, cawProvider, logger, cfg.CAW.ProviderAPIKey, cfg.CAW.ProviderWalletID, cfg.CAW.ProviderAddr, cfg.CAW.SubProviderAddr, cfg.CAW.Sandbox)

	// Initialize SSE hub for frontend topology
	sseHub := api.NewSSEHub()

	// Initialize event listener (skip in offline mode)
	if !*offline {
		evtCh := make(chan *listener.Event, 100)
		l := listener.New(s, s.RD, cfg.Chain.RPCURL, logger)
		l.Start(ctx, evtCh)

		// 事件处理 goroutine
		go func() {
			for evt := range evtCh {
				if err := l.ProcessEvent(ctx, evt); err != nil {
					logger.Error("event processing failed",
						zap.String("tx_hash", evt.TxHash),
						zap.Error(err),
					)
				}
			}
		}()

		defer l.Stop()
	} else {
		logger.Info("Running in offline mode — reading from mock_events.jsonl")
		go func() {
			data, err := os.ReadFile("../mock_events.jsonl")
			if err != nil {
				logger.Warn("Failed to read mock_events.jsonl", zap.Error(err))
				return
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var evt struct {
					Type   string `json:"type"`
					JobID  uint64 `json:"job_id"`
					Status string `json:"status"`
				}
				if err := json.Unmarshal([]byte(line), &evt); err != nil {
					logger.Warn("Failed to parse mock event", zap.String("line", line), zap.Error(err))
					continue
				}
				color := "blue"
				if evt.Type == "BountyClaimed" {
					color = "yellow"
				} else if evt.Type == "ResultSubmitted" {
					color = "green"
				}
				sseHub.Emit(api.SSEEvent{
					Type:    evt.Type,
					JobID:   evt.JobID,
					Status:  evt.Status,
					Color:   color,
					Message: "Offline replay: " + evt.Type,
				})
				time.Sleep(2 * time.Second) // Simulate real-time pacing
			}
			logger.Info("Offline replay complete")
		}()
	}

	// Register HTTP routes
	h := api.NewHandler(cfg, s, cawProvider, fluxaProvider, onChainRep, ipfsProvider, templateEngine, ruleEngine, llmEngine, agg, relayerSvc, sseHub, logger)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// CORS middleware for frontend dev server
	corsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Trace-ID, X-Demo-Admin-Token")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})

	// Start HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      corsHandler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx) //nolint:errcheck
		cancel()
	}()

	logger.Info("AEP backend starting",
		zap.Int("port", cfg.Server.Port),
		zap.Bool("offline", *offline),
	)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("server error", zap.Error(err))
	}
}
