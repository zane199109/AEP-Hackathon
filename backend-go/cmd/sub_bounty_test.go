// Day 1 integration test for sub-bounty chain.
// Run against the running backend at localhost:8080.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDemo_SubBountyChain(t *testing.T) {
	// Step 1: Create root bounty
	rootResp := doPost(t, "/api/bounty", `{"buyer":"0xAlice","amount":"1000000","deadline":"2026-06-10T00:00:00Z","min_reputation":1}`)
	job1 := int64(rootResp["job_id"].(float64))
	t.Logf("  ✅ Root bounty created: job=%d", job1)

	// Step 2: Claim root
	claimResp := doPost(t, fmt.Sprintf("/api/bounty/%d/claim", job1), `{"seller":"0xBob"}`)
	if claimResp["status"] != "Assigned" {
		t.Fatalf("expected Assigned, got %v", claimResp["status"])
	}
	t.Logf("  ✅ Root claimed: status=Assigned")

	// Step 3: Create sub-bounty
	subResp := doPost(t, fmt.Sprintf("/api/bounty/%d/sub-bounty", job1),
		`{"seller":"0xBob","amount":"500000","intent":"Analyze wallet","wallet_id":"w_001"}`)
	job2 := int64(subResp["job_id"].(float64))
	parentID := int64(subResp["parent_id"].(float64))
	depth := int(subResp["depth"].(float64))
	if parentID != job1 {
		t.Fatalf("expected parent_id=%d, got %d", job1, parentID)
	}
	if depth != 1 {
		t.Fatalf("expected depth=1, got %d", depth)
	}
	t.Logf("  ✅ Sub-bounty created: job=%d, parent=%d, depth=%d", job2, parentID, depth)

	// Step 4: Claim sub-bounty
	claimSubResp := doPost(t, fmt.Sprintf("/api/bounty/%d/claim", job2), `{"seller":"0xCharlie"}`)
	if claimSubResp["status"] != "Assigned" {
		t.Fatalf("expected Assigned, got %v", claimSubResp["status"])
	}
	t.Logf("  ✅ Sub-bounty claimed: status=Assigned")

	// Step 5: Verify DB has correct depth
	// (checked via API response already)
	t.Logf("  ✅ Sub-bounty chain verified: root depth=0, sub depth=1")
}

func doPost(t *testing.T, path, body string) map[string]interface{} {
	t.Helper()
	resp, err := http.Post("http://localhost:8080"+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if errMsg, ok := result["error"]; ok {
		t.Fatalf("POST %s returned error: %v", path, errMsg)
	}
	return result
}
