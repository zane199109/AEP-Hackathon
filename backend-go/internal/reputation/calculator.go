package reputation

import (
	"crypto/sha256"
	"fmt"
	"math"
	"time"
)

// TaskInfo holds the data needed to calculate reputation for a single task.
type TaskInfo struct {
	QualityScore float64 // 0.0 ~ 1.0 from evaluation
	AmountWei    uint64  // reward in wei
	SubmittedAt  int64   // unix timestamp
	Deadline     int64   // unix timestamp
}

// Amount tiers (in ETH)
var amountTiers = []struct {
	maxWei  uint64
	score   float64
}{
	{wei(0.001), 0},    // < 0.001 ETH → 0
	{wei(0.01), 25},    // 0.001 ~ 0.01 ETH → 25
	{wei(0.1), 50},     // 0.01 ~ 0.1 ETH → 50
	{wei(1.0), 75},     // 0.1 ~ 1 ETH → 75
	{math.MaxUint64, 100}, // ≥ 1 ETH → 100
}

func wei(eth float64) uint64 {
	return uint64(eth * 1e18)
}

const (
	qualityWeight = 0.50
	amountWeight  = 0.30
	speedWeight   = 0.20

	inactiveDays    = 30
	decayPerDay     = 1.0
	slashMinPenalty = 20.0
	slashPct        = 0.30
)

// amountScore returns the tiered score for a given amount in wei.
func amountScore(wei uint64) float64 {
	for _, t := range amountTiers {
		if wei < t.maxWei {
			return t.score
		}
	}
	return 100
}

// speedScore calculates the timeliness score.
// On time = 100. Each day late loses 20 points, min 0.
func speedScore(submittedAt, deadline int64) float64 {
	if deadline <= 0 || submittedAt <= deadline {
		return 100
	}
	daysLate := (submittedAt - deadline) / int64(24*3600)
	score := 100 - float64(daysLate)*20
	if score < 0 {
		return 0
	}
	return score
}

// CalculateNewScore computes the new reputation score after completing a task.
//
// The weighted score combines quality (50%), amount (30%), and speed (20%).
// The new score is an exponential moving average:
//
//	new = old + α × (weighted - old)
//
// where α = 1 / (1 + taskCount). This means early tasks change reputation
// quickly, while later tasks have diminishing impact.
func CalculateNewScore(oldScore float64, task TaskInfo, taskCount int) float64 {
	if taskCount < 0 {
		taskCount = 0
	}

	// Quality: use evaluation score directly (0~100)
	qScore := task.QualityScore * 100

	// Amount: tiered
	aScore := amountScore(task.AmountWei)

	// Speed: timeliness
	sScore := speedScore(task.SubmittedAt, task.Deadline)

	// Weighted composite
	weighted := qScore*qualityWeight + aScore*amountWeight + sScore*speedWeight

	// Iteration factor: early tasks change more
	alpha := 1.0 / (1.0 + float64(taskCount))

	newScore := oldScore + alpha*(weighted-oldScore)

	if newScore > 100 {
		newScore = 100
	}
	if newScore < 0 {
		newScore = 0
	}
	return newScore
}

// CalculateSlash computes the penalty for a fraudulent/bad delivery.
// Penalty = max(20, currentScore × 30%), ensuring high score = high risk.
// Returns the new score (never below 0).
func CalculateSlash(currentScore float64) (newScore float64, penalty float64) {
	penalty = slashMinPenalty
	pctPenalty := currentScore * slashPct
	if pctPenalty > penalty {
		penalty = pctPenalty
	}
	newScore = currentScore - penalty
	if newScore < 0 {
		newScore = 0
	}
	return newScore, penalty
}

// ApplyDecay reduces score due to inactivity.
// If more than inactiveDays have passed since last activity,
// score drops by decayPerDay per idle day.
func ApplyDecay(currentScore float64, lastActivityAt, now int64) (float64, float64) {
	if lastActivityAt <= 0 || now <= lastActivityAt {
		return currentScore, 0
	}
	idleDays := (now - lastActivityAt) / int64(24*3600)
	if idleDays <= inactiveDays {
		return currentScore, 0
	}
	decayDays := idleDays - inactiveDays
	totalDecay := float64(decayDays) * decayPerDay
	newScore := currentScore - totalDecay
	if newScore < 0 {
		newScore = 0
	}
	return newScore, totalDecay
}

// ComputeParamHash creates a verifiable hash of the evaluation parameters.
// Anyone can re-compute this hash off-chain to verify the score was not manipulated.
func ComputeParamHash(qualityPass bool, amountWei, submittedAt, deadline uint64) [32]byte {
	data := fmt.Sprintf("%t|%d|%d|%d", qualityPass, amountWei, submittedAt, deadline)
	return sha256.Sum256([]byte(data))
}

// ReputationResult holds the full outcome of a reputation calculation.
type ReputationResult struct {
	OldScore   float64
	NewScore   float64
	Delta      float64
	Slash      bool
	Penalty    float64
	Decay      float64
	QScore     float64
	AScore     float64
	SScore     float64
	Weighted   float64
	Alpha      float64
	TaskCount  int
}

// CalculateFull runs the full reputation pipeline and returns detailed results.
func CalculateFull(oldScore float64, task TaskInfo, taskCount int, now int64) ReputationResult {
	r := ReputationResult{
		OldScore:  oldScore,
		TaskCount: taskCount,
	}

	// Quality score (0~100)
	r.QScore = task.QualityScore * 100

	// Amount score
	r.AScore = amountScore(task.AmountWei)

	// Speed score
	r.SScore = speedScore(task.SubmittedAt, task.Deadline)

	// Weighted composite
	r.Weighted = r.QScore*qualityWeight + r.AScore*amountWeight + r.SScore*speedWeight

	// Iteration factor
	r.Alpha = 1.0 / (1.0 + float64(taskCount))

	// New score
	r.NewScore = oldScore + r.Alpha*(r.Weighted-oldScore)

	// Cap
	if r.NewScore > 100 {
		r.NewScore = 100
	}
	if r.NewScore < 0 {
		r.NewScore = 0
	}

	r.Delta = r.NewScore - r.OldScore

	// Decay
	nowTime := time.Now().Unix()
	r.NewScore, r.Decay = ApplyDecay(r.NewScore, 0, nowTime)

	return r
}
