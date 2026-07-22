package httpcontract

import "testing"

func TestRateLimitResetCreditsResponseIncludesVisibleDetails(t *testing.T) {
	title := "Full reset"
	description := "Ready to redeem"
	snapshotJSON := `{"rateLimitResetCredits":{"availableCount":2,"credits":[{"id":"credit-1","resetType":"codexRateLimits","status":"available","grantedAt":1700000000,"expiresAt":1700600000,"title":"Full reset","description":"Ready to redeem"}]}}`

	response := rateLimitResetCreditsResponse(&snapshotJSON)
	if response == nil {
		t.Fatal("rateLimitResetCreditsResponse() = nil, want details")
	}
	if response.AvailableCount != 2 || len(response.Credits) != 1 {
		t.Fatalf("response = %#v, want count 2 and one visible credit", response)
	}
	credit := response.Credits[0]
	if credit.ID != "credit-1" || credit.ResetType != "codexRateLimits" || credit.Status != "available" {
		t.Fatalf("credit identity = %#v", credit)
	}
	if credit.GrantedAt != 1700000000 || credit.ExpiresAt != 1700600000 {
		t.Fatalf("credit timestamps = %#v", credit)
	}
	if credit.Title == nil || *credit.Title != title || credit.Description == nil || *credit.Description != description {
		t.Fatalf("credit copy = %#v", credit)
	}
}

func TestRateLimitResetCreditsResponseSupportsCountOnlySnapshot(t *testing.T) {
	snapshotJSON := `{"rateLimitResetCredits":{"availableCount":3}}`

	response := rateLimitResetCreditsResponse(&snapshotJSON)
	if response == nil || response.AvailableCount != 3 || len(response.Credits) != 0 {
		t.Fatalf("response = %#v, want count-only summary", response)
	}
}
