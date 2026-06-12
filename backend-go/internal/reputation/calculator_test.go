package reputation

import (
	"math"
	"testing"
	"time"
)

func TestAmountScore(t *testing.T) {
	tests := []struct {
		wei   uint64
		score float64
	}{
		{wei(0.0005), 0},
		{wei(0.001) - 1, 0},
		{wei(0.001), 25},
		{wei(0.005), 25},
		{wei(0.01) - 1, 25},
		{wei(0.01), 50},
		{wei(0.05), 50},
		{wei(0.1) - 1, 50},
		{wei(0.1), 75},
		{wei(0.5), 75},
		{wei(1.0), 100},
		{wei(10.0), 100},
		{math.MaxUint64, 100},
	}
	for _, tt := range tests {
		got := amountScore(tt.wei)
		if got != tt.score {
			t.Errorf("amountScore(%d) = %.0f, want %.0f", tt.wei, got, tt.score)
		}
	}
}

func TestSpeedScore(t *testing.T) {
	deadline := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC).Unix()

	tests := []struct {
		name      string
		submitted int64
		want      float64
	}{
		{"on time", deadline - 3600, 100},
		{"exactly on deadline", deadline, 100},
		{"1 day late", deadline + 86400, 80},
		{"3 days late", deadline + 3*86400, 40},
		{"5 days late", deadline + 5*86400, 0},
		{"10 days late", deadline + 10*86400, 0},
		{"no deadline", 0, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := speedScore(tt.submitted, deadline)
			if got != tt.want {
				t.Errorf("speedScore() = %.0f, want %.0f", got, tt.want)
			}
		})
	}
}

func TestCalculateNewScore(t *testing.T) {
	deadline := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC).Unix()

	t.Run("first task high quality", func(t *testing.T) {
		task := TaskInfo{
			QualityScore: 0.85,
			AmountWei:    wei(0.05),
			SubmittedAt:  deadline - 3600,
			Deadline:     deadline,
		}
		got := CalculateNewScore(50, task, 0)
		// weighted = 85*0.5 + 50*0.3 + 100*0.2 = 42.5 + 15 + 20 = 77.5
		// α = 1/(1+0) = 1.0
		// new = 50 + 1.0 * (77.5 - 50) = 77.5
		want := 77.5
		if math.Abs(got-want) > 0.01 {
			t.Errorf("CalculateNewScore() = %.2f, want %.2f", got, want)
		}
	})

	t.Run("second task converges", func(t *testing.T) {
		task := TaskInfo{
			QualityScore: 0.90,
			AmountWei:    wei(0.05),
			SubmittedAt:  deadline - 3600,
			Deadline:     deadline,
		}
		got := CalculateNewScore(63.75, task, 1)
		// weighted = 90*0.5 + 50*0.3 + 100*0.2 = 45 + 15 + 20 = 80
		// α = 1/2 = 0.5
		// new = 63.75 + 0.5 * (80 - 63.75) = 63.75 + 8.125 = 71.875
		want := 71.875
		if math.Abs(got-want) > 0.01 {
			t.Errorf("CalculateNewScore() = %.2f, want %.2f", got, want)
		}
	})

	t.Run("10th task barely moves", func(t *testing.T) {
		task := TaskInfo{
			QualityScore: 0.50,
			AmountWei:    wei(0.001),
			SubmittedAt:  deadline - 3600,
			Deadline:     deadline,
		}
		got := CalculateNewScore(80, task, 10)
		// weighted = 50*0.5 + 25*0.3 + 100*0.2 = 25 + 7.5 + 20 = 52.5
		// α = 1/11 ≈ 0.0909
		// new = 80 + 0.0909 * (52.5 - 80) = 80 - 2.5 = 77.5
		if got > 79 || got < 76 {
			t.Errorf("10th task should barely move, got %.2f (expected ~77.5)", got)
		}
	})

	t.Run("very low quality", func(t *testing.T) {
		task := TaskInfo{
			QualityScore: 0.10,
			AmountWei:    wei(0.001),
			SubmittedAt:  deadline + 3*86400, // 3 days late
			Deadline:     deadline,
		}
		got := CalculateNewScore(60, task, 0)
		// weighted = 10*0.5 + 25*0.3 + 40*0.2 = 5 + 7.5 + 8 = 20.5
		// new = 60 + 0.5 * (20.5 - 60) = 60 - 19.75 = 40.25
		if got > 45 {
			t.Errorf("low quality should drop significantly, got %.2f", got)
		}
	})
}

func TestCalculateSlash(t *testing.T) {
	tests := []struct {
		name        string
		current     float64
		wantNewMin  float64
		wantPenalty float64
	}{
		{"low score", 10, 0, 20},          // min 20 penalty
		{"high score", 90, 63, 27},        // 90 * 0.3 = 27 > 20
		{"medium score", 50, 30, 20},      // min 20
		{"zero score", 0, 0, 20},          // min 20
		{"max score", 100, 70, 30},        // 100 * 0.3 = 30
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNew, gotPenalty := CalculateSlash(tt.current)
			if math.Abs(gotNew-tt.wantNewMin) > 0.01 || math.Abs(gotPenalty-tt.wantPenalty) > 0.01 {
				t.Errorf("CalculateSlash(%.0f) = (%.2f, %.2f), want (%.2f, %.2f)",
					tt.current, gotNew, gotPenalty, tt.wantNewMin, tt.wantPenalty)
			}
		})
	}
}

func TestApplyDecay(t *testing.T) {
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC).Unix() // 40 days after June 10
	lastActive := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC).Unix()

	score, decay := ApplyDecay(80, lastActive, now)
	// 40 days idle, first 30 free, 10 days decay
	// decay = 10 * 1 = 10
	if math.Abs(decay-10) > 0.01 || math.Abs(score-70) > 0.01 {
		t.Errorf("ApplyDecay() = (%.2f, %.2f), want (70, 10)", score, decay)
	}

	// Under 30 days = no decay
	score2, decay2 := ApplyDecay(80, lastActive, lastActive+20*86400)
	if decay2 != 0 || score2 != 80 {
		t.Errorf("No decay expected, got (%.2f, %.2f)", score2, decay2)
	}

	// Low score can't go negative
	score3, _ := ApplyDecay(5, lastActive, now)
	if score3 < 0 {
		t.Errorf("Score should not go negative, got %.2f", score3)
	}
}

func TestComputeParamHash(t *testing.T) {
	h1 := ComputeParamHash(true, wei(0.05), 1000, 2000)
	h2 := ComputeParamHash(true, wei(0.05), 1000, 2000)
	h3 := ComputeParamHash(false, wei(0.05), 1000, 2000)

	if h1 != h2 {
		t.Error("same params should produce same hash")
	}
	if h1 == h3 {
		t.Error("different params should produce different hash")
	}
	if len(h1) != 32 {
		t.Errorf("hash should be 32 bytes, got %d", len(h1))
	}
}

func TestCalculateFull(t *testing.T) {
	deadline := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC).Unix()
	task := TaskInfo{
		QualityScore: 0.85,
		AmountWei:    wei(0.05),
		SubmittedAt:  deadline - 3600,
		Deadline:     deadline,
	}
	r := CalculateFull(50, task, 0, deadline+86400)
	if r.QScore != 85 {
		t.Errorf("QScore = %.2f, want 85", r.QScore)
	}
	if r.AScore != 50 {
		t.Errorf("AScore = %.2f, want 50", r.AScore)
	}
	if r.SScore != 100 {
		t.Errorf("SScore = %.2f, want 100", r.SScore)
	}
	if r.Alpha != 1.0 {
		t.Errorf("Alpha = %.2f, want 1.0", r.Alpha)
	}
	if r.NewScore < 75 || r.NewScore > 80 {
		t.Errorf("NewScore = %.2f, expected ~77.5", r.NewScore)
	}
}

func TestConvergence(t *testing.T) {
	deadline := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC).Unix()
	score := 50.0
	// Simulate 20 high-quality tasks
	for i := 0; i < 20; i++ {
		task := TaskInfo{
			QualityScore: 0.90,
			AmountWei:    wei(0.05),
			SubmittedAt:  deadline - 3600,
			Deadline:     deadline,
		}
		score = CalculateNewScore(score, task, i)
	}
	// Should converge toward ~80 (weighted max ~80)
	if score < 75 || score > 82 {
		t.Errorf("Should converge to ~80, got %.2f", score)
	}

	// Simulate a slash
	newScore, penalty := CalculateSlash(score)
	if penalty < 20 {
		t.Errorf("Slash penalty should be >= 20, got %.2f", penalty)
	}
	if newScore >= score {
		t.Errorf("Slash should reduce score, got %.2f -> %.2f", score, newScore)
	}
}
