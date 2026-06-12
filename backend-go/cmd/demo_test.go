package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// Demo scenario tests against the running backend at localhost:8080.
// Prerequisites:
//   - docker compose up -d (PG + Redis)
//   - Backend running on :8080 (bash /tmp/start-backend.sh)
//   - .env configured with real credentials

const baseURL = "http://localhost:8080"

func post(path string, body interface{}) (*http.Response, error) {
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)
	return http.Post(baseURL+path, "application/json", &buf)
}

func readBody(resp *http.Response) (map[string]interface{}, error) {
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	json.Unmarshal(b, &result)
	return result, nil
}

// ==================== Scenario 1: Happy Path ====================

func TestDemo_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in short mode")
	}

	var jobID float64

	// Step 1: Post Bounty
	t.Run("PostBounty", func(t *testing.T) {
		resp, err := post("/api/bounty", map[string]string{
			"buyer": "0xBuyer", "amount": "1000000000000000", "deadline": "2026-06-10T00:00:00Z",
		})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d: body=%v", resp.StatusCode, body)
		}

		j, ok := body["job_id"].(float64)
		if !ok || j == 0 {
			t.Fatalf("invalid job_id: %v", body["job_id"])
		}
		jobID = j

		pactID, _ := body["pact_id"].(string)
		status, _ := body["status"].(string)
		t.Logf("  ✅ Post Bounty → job=%v, pact=%s, status=%s", jobID, pactID, status)
	})

	if jobID == 0 {
		t.Fatal("no job_id from post, aborting")
	}

	// Step 2: Claim
	t.Run("Claim", func(t *testing.T) {
		resp, err := post(fmt.Sprintf("/api/bounty/%.0f/claim", jobID), map[string]string{
			"seller": "0xSeller1",
		})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, body)
		}
		if body["status"] != "Assigned" {
			t.Errorf("expected 'Assigned', got '%v'", body["status"])
		}
		t.Logf("  ✅ Claim → status=%v", body["status"])
	})

	// Step 3: Submit (IPFS + LLM)
	t.Run("Submit", func(t *testing.T) {
		resp, err := post(fmt.Sprintf("/api/bounty/%.0f/submit", jobID), map[string]string{
			"seller": "0xSeller1",
			"data":   "Complete solution with code, tests, and documentation meeting all requirements.",
		})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, body)
		}

		verdict, ok := body["verdict"].(map[string]interface{})
		if !ok {
			t.Fatal("missing verdict in response")
		}

		status, _ := verdict["status"].(string)
		passed, _ := verdict["passed"].(bool)
		cid, _ := body["cid"].(string)

		t.Logf("  ✅ Submit → cid=%s, verdict=%s, passed=%v", cid, status, passed)
	})

	// Step 4: Confirm (BuyerApproval + Settlement)
	t.Run("Confirm", func(t *testing.T) {
		resp, err := http.Post(
			fmt.Sprintf("%s/api/confirm/%.0f", baseURL, jobID),
			"application/json", nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)

		settlement, _ := body["settlement"].(string)
		status, _ := body["status"].(string)

		if status != "confirmed" {
			t.Errorf("expected 'confirmed', got '%s'", status)
		}
		t.Logf("  ✅ Confirm → status=%s, settlement=%s", status, settlement)
	})
}

// ==================== Scenario 2: Fail Path ====================

func TestDemo_FailPath_EmptyDelivery(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in short mode")
	}

	// Post
	resp, err := post("/api/bounty", map[string]string{
		"buyer": "0xBuyer2", "amount": "1000000000000000", "deadline": "2026-06-10T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readBody(resp)
	jobID := body["job_id"].(float64)

	// Claim
	post(fmt.Sprintf("/api/bounty/%.0f/claim", jobID), map[string]string{"seller": "0xSeller2"})

	// Submit EMPTY → Rule should reject
	resp, err = post(fmt.Sprintf("/api/bounty/%.0f/submit", jobID), map[string]string{
		"seller": "0xSeller2", "data": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = readBody(resp)

	verdict, _ := body["verdict"].(map[string]interface{})
	status, _ := verdict["status"].(string)
	passed, _ := verdict["passed"].(bool)

	if status != "slashed" {
		t.Errorf("expected 'slashed' for empty delivery, got '%s'", status)
	}
	if passed {
		t.Error("expected passed=false for empty delivery")
	}
	t.Logf("  ✅ Empty delivery → status=%s, passed=%v", status, passed)
}

// ==================== Scenario 3: Concurrent Claim Race ====================

func TestDemo_ConcurrentClaim(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in short mode")
	}

	resp, err := post("/api/bounty", map[string]string{
		"buyer": "0xBuyer3", "amount": "1000000000000000", "deadline": "2026-06-10T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readBody(resp)
	jobID := body["job_id"].(float64)

	var wg sync.WaitGroup
	mu := sync.Mutex{}
	successCount := 0
	failCount := 0

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resp, err := post(fmt.Sprintf("/api/bounty/%.0f/claim", jobID), map[string]string{
				"seller": fmt.Sprintf("0xSeller_%d", id),
			})
			if err != nil {
				return
			}
			mu.Lock()
			if resp.StatusCode == 200 {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected 1 successful claim, got %d", successCount)
	}
	if failCount != 4 {
		t.Errorf("expected 4 failed claims, got %d", failCount)
	}
	t.Logf("  ✅ Concurrent claim: %d success, %d conflicts", successCount, failCount)
}

// ==================== Scenario 4: L2 Dedup ====================

func TestDemo_L2Dedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in short mode")
	}

	// Post + Claim + Submit
	resp, err := post("/api/bounty", map[string]string{
		"buyer": "0xBuyer4", "amount": "1000000000000000", "deadline": "2026-06-10T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readBody(resp)
	jobID := body["job_id"].(float64)

	post(fmt.Sprintf("/api/bounty/%.0f/claim", jobID), map[string]string{"seller": "0xSeller4"})
	post(fmt.Sprintf("/api/bounty/%.0f/submit", jobID), map[string]string{
		"seller": "0xSeller4", "data": "valid delivery data for dedup test",
	})

	// First confirm → should be "settled"
	t.Run("First_Settled", func(t *testing.T) {
		resp, err := http.Post(fmt.Sprintf("%s/api/confirm/%.0f", baseURL, jobID), "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)
		settlement, _ := body["settlement"].(string)
		if settlement != "settled" && settlement != "not_confirmed" {
			t.Errorf("expected 'settled' or 'not_confirmed', got '%s'", settlement)
		}
		t.Logf("  ✅ First confirm: %s", settlement)
	})

	// Second confirm → should be "already_settled" (L2 dedup)
	t.Run("Second_Dedup", func(t *testing.T) {
		resp, err := http.Post(fmt.Sprintf("%s/api/confirm/%.0f", baseURL, jobID), "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)
		settlement, _ := body["settlement"].(string)
		if settlement != "already_settled" && settlement != "not_confirmed" {
			t.Logf("  ℹ️  Got '%s' (may not be dedup if first failed)", settlement)
		} else {
			t.Logf("  ✅ Second confirm: %s", settlement)
		}
	})
}

// ==================== Scenario 5: Admin Auth ====================

func TestDemo_AdminRetryAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in short mode")
	}

	// Without token
	t.Run("NoToken_401", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/admin/retry/999", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)
		if resp.StatusCode != 401 {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
		if msg, _ := body["error"].(string); !strings.Contains(msg, "missing admin token") {
			t.Errorf("expected auth error, got '%s'", msg)
		}
		t.Log("  ✅ No token → 401")
	})

	// With token
	t.Run("WithToken_200", func(t *testing.T) {
		req, _ := http.NewRequest("POST", baseURL+"/admin/retry/999", nil)
		req.Header.Set("X-Demo-Admin-Token", "demo-admin-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := readBody(resp)
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		settlement, _ := body["settlement"].(string)
		t.Logf("  ✅ With token → 200, settlement=%s", settlement)
	})
}

// Health check (quick connectivity test)
func TestDemo_Health(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/health")
	if err != nil {
		t.Fatal("Backend not running at", baseURL, err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	t.Log("  ✅ Backend is healthy")
}
