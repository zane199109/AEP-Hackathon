package provider
import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"go.uber.org/zap"
	"github.com/zane199109/AEP-Hackathon/backend-go/internal/config"
)
// ==================== On-Chain Reputation Provider ====================
// OnChainReputationProvider reads and writes agent reputation from/to AEPReputation.sol.
type OnChainReputationProvider struct {
	cfg     config.ChainConfig
	httpCli *http.Client
	log     *zap.Logger
}
// NewOnChainReputationProvider creates a provider for on-chain reputation.
func NewOnChainReputationProvider(cfg config.ChainConfig, log *zap.Logger) *OnChainReputationProvider {
	return &OnChainReputationProvider{
		cfg:     cfg,
		httpCli: &http.Client{Timeout: 10 * time.Second},
		log:     log,
	}
}
// getScoreSelector is bytes4(keccak256("getScore(address)"))
const getScoreSelector = "0xd47875d0"
// updateScoreSelector is bytes4(keccak256("updateScore(address,uint256,uint256,int256,string,bytes32)"))
const updateScoreSelector = "0xf9dc70a0"
// CheckReputation queries AEPReputation.getScore(agent) via eth_call.
// Returns score as uint256, or error if contract not configured or RPC fails.
func (p *OnChainReputationProvider) CheckReputation(ctx context.Context, agentAddr string) (*FluxAResult, error) {
	if p.cfg.ReputationAddr == "" {
		p.log.Warn("Reputation contract address not set, returning fallback")
		return &FluxAResult{Passed: true, Score: 50, FallbackUsed: true}, nil
	}
	if p.cfg.RPCURL == "" {
		p.log.Warn("RPC URL not set, returning fallback")
		return &FluxAResult{Passed: true, Score: 50, FallbackUsed: true}, nil
	}
	cleanAddr := agentAddr
	if len(cleanAddr) == 42 && cleanAddr[:2] == "0x" {
		cleanAddr = cleanAddr[2:]
	}
	paddedAddr := fmt.Sprintf("%064s", cleanAddr)
	data := getScoreSelector + paddedAddr
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_call",
		"params": []interface{}{
			map[string]string{
				"to":   p.cfg.ReputationAddr,
				"data": data,
			},
			"latest",
		},
		"id": 1,
	}
	bodyBytes, _ := json.Marshal(rpcReq)
	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.RPCURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create rpc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp struct {
		Result string `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse rpc response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", rpcResp.Error.Message)
	}
	hexStr := rpcResp.Result
	if len(hexStr) < 2 {
		return nil, fmt.Errorf("unexpected rpc result: %s", hexStr)
	}
	resultHex := hexStr[2:]
	if len(resultHex) > 64 {
		resultHex = resultHex[len(resultHex)-64:]
	}
	scoreBig := new(big.Int)
	scoreBig.SetString(resultHex, 16)
	score := int(scoreBig.Uint64())
	passed := score >= 60
	p.log.Info("On-chain reputation check",
		zap.String("agent", agentAddr),
		zap.Int("score", score),
		zap.Bool("passed", passed),
	)
	return &FluxAResult{Passed: passed, Score: score, FallbackUsed: false}, nil
}
// UpdateReputation calls AEPReputation.updateScore on-chain via eth_sendTransaction.
// The transaction is signed using the configured private key.
//
// Parameters:
//   - agentAddr: the agent whose score to update
//   - taskId: the task ID (anti-replay)
//   - newScore: the calculated new score (0-100)
//   - delta: score change (+ positive, - negative)
//   - reason: human-readable reason ("delivery_passed", "delivery_slashed", etc.)
//   - paramHash: SHA256 hash of evaluation params (for verifiability)
//
// If contract address is not configured, logs and returns nil (graceful degrade).
func (p *OnChainReputationProvider) UpdateReputation(
	ctx context.Context,
	agentAddr string,
	taskId uint64,
	newScore uint64,
	delta int64,
	reason string,
	paramHash [32]byte,
) (string, error) {
	if p.cfg.ReputationAddr == "" || p.cfg.RPCURL == "" || p.cfg.PrivateKey == "" {
		p.log.Warn("Reputation update skipped: missing contract addr, RPC, or private key",
			zap.String("agent", agentAddr),
			zap.Uint64("task_id", taskId),
			zap.Uint64("new_score", newScore),
		)
		return "", nil
	}
	// 1. Parse private key → derive sender address
	privKey, err := crypto.HexToECDSA(p.cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	senderAddr := crypto.PubkeyToAddress(privKey.PublicKey)
	// 2. Get nonce
	nonce, err := p.ethGetTransactionCount(ctx, senderAddr)
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}
	// 3. Get gas price
	gasPrice, err := p.ethGasPrice(ctx)
	if err != nil {
		return "", fmt.Errorf("get gas price: %w", err)
	}
	// 4. Build ABI-encoded calldata
	calldata := p.buildUpdateScoreCalldata(agentAddr, taskId, newScore, delta, reason, paramHash)
	// 5. Build legacy transaction
	toAddr := common.HexToAddress(p.cfg.ReputationAddr)
	tx := &types.LegacyTx{
		Nonce:    nonce,
		GasPrice: gasPrice,
		Gas:      200000,
		To:       &toAddr,
		Value:    big.NewInt(0),
		Data:     calldata,
	}
	// 6. Sign with EIP-155 (chain ID = 11155111 for Sepolia)
	chainID := big.NewInt(11155111)
	signedTx, err := types.SignTx(types.NewTx(tx), types.NewEIP155Signer(chainID), privKey)
	if err != nil {
		return "", fmt.Errorf("sign tx: %w", err)
	}
	// 7. RLP encode
	rawTx, err := signedTx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("rlp encode: %w", err)
	}
	// 8. Send via eth_sendRawTransaction
	txHash, err := p.ethSendRawTransaction(ctx, rawTx)
	if err != nil {
		return "", fmt.Errorf("send raw tx: %w", err)
	}
	p.log.Info("Reputation updated on-chain",
		zap.String("agent", agentAddr),
		zap.Uint64("task_id", taskId),
		zap.Uint64("new_score", newScore),
		zap.Int64("delta", delta),
	zap.String("reason", reason),
		zap.String("tx_hash", txHash),
	)
	return txHash, nil
}

// ethGetTransactionCount returns the nonce for the given address.
func (p *OnChainReputationProvider) ethGetTransactionCount(ctx context.Context, addr common.Address) (uint64, error) {
	var result string
	if err := p.rpcCall(ctx, "eth_getTransactionCount", []interface{}{addr.Hex(), "pending"}, &result); err != nil {
		return 0, err
	}
	return parseHexUint64(result[2:]), nil
}
// ethGasPrice returns the current gas price.
func (p *OnChainReputationProvider) ethGasPrice(ctx context.Context) (*big.Int, error) {
	var result string
	if err := p.rpcCall(ctx, "eth_gasPrice", []interface{}{}, &result); err != nil {
		return nil, err
	}
	return parseHexBig(result[2:]), nil
}
// parseHexUint64 parses a hex string (without 0x prefix) into uint64.
func parseHexUint64(s string) uint64 {
	n := new(big.Int)
	n.SetString(s, 16)
	return n.Uint64()
}
// parseHexBig parses a hex string (without 0x prefix) into *big.Int.
func parseHexBig(s string) *big.Int {
	n := new(big.Int)
	n.SetString(s, 16)
	return n
}
// ethSendRawTransaction sends a signed raw transaction and returns the tx hash.
func (p *OnChainReputationProvider) ethSendRawTransaction(ctx context.Context, rawTx []byte) (string, error) {
	var result string
	if err := p.rpcCall(ctx, "eth_sendRawTransaction", []interface{}{"0x" + hex.EncodeToString(rawTx)}, &result); err != nil {
		return "", err
	}
	return result, nil
}
// rpcCall performs a JSON-RPC call and unmarshals the result into out.
func (p *OnChainReputationProvider) rpcCall(ctx context.Context, method string, params []interface{}, out *string) error {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	bodyBytes, _ := json.Marshal(rpcReq)
	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.RPCURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpCli.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp struct {
		Result string `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("rpc error: %s", rpcResp.Error.Message)
	}
	*out = rpcResp.Result
	return nil
}
// buildUpdateScoreCalldata ABI-encodes the updateScore function call.
func (p *OnChainReputationProvider) buildUpdateScoreCalldata(agentAddr string, taskId uint64, newScore uint64, delta int64, reason string, paramHash [32]byte) []byte {
	selector, _ := hex.DecodeString(updateScoreSelector[2:])
	cleanAgent := agentAddr
	if len(cleanAgent) == 42 && cleanAgent[:2] == "0x" {
		cleanAgent = cleanAgent[2:]
	}
	agentPadded := fmt.Sprintf("%064s", cleanAgent)
	agentBytes, _ := hex.DecodeString(agentPadded)
	taskPadded := fmt.Sprintf("%064s", fmt.Sprintf("%x", taskId))
	taskBytes, _ := hex.DecodeString(taskPadded)
	scorePadded := fmt.Sprintf("%064s", fmt.Sprintf("%x", newScore))
	scoreBytes, _ := hex.DecodeString(scorePadded)
	var deltaPadded string
	if delta >= 0 {
		deltaPadded = fmt.Sprintf("%064s", fmt.Sprintf("%x", delta))
	} else {
		absVal := uint64(-delta)
		comp := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), new(big.Int).SetUint64(absVal))
		deltaPadded = fmt.Sprintf("%064x", comp)
	}
	deltaBytes, _ := hex.DecodeString(deltaPadded)
	strOffset := uint64(192)
	strOffsetPadded := fmt.Sprintf("%064s", fmt.Sprintf("%x", strOffset))
	strOffsetBytes, _ := hex.DecodeString(strOffsetPadded)
	paramBytes := paramHash[:]
	reasonBytes := []byte(reason)
	reasonLen := fmt.Sprintf("%064s", fmt.Sprintf("%x", len(reasonBytes)))
	reasonLenBytes, _ := hex.DecodeString(reasonLen)
	paddedLen := ((len(reasonBytes) + 31) / 32) * 32
	reasonData := make([]byte, paddedLen)
	copy(reasonData, reasonBytes)
	return bytes.Join([][]byte{
		selector,
		agentBytes,
		taskBytes,
		scoreBytes,
		deltaBytes,
		strOffsetBytes,
		paramBytes,
		reasonLenBytes,
		reasonData,
	}, nil)
}
