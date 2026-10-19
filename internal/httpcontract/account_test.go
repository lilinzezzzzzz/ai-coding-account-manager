package httpcontract

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestPlanDatesPreserveMilliseconds(t *testing.T) {
	for _, value := range []int64{1, 57600000, 31507200000, 99999999999, 100000000000, 100000000001, 1794153600000, 253402214400000} {
		t.Run(strconv.FormatInt(value, 10), func(t *testing.T) {
			raw := json.RawMessage(strconv.FormatInt(value, 10))
			got, err := (UpdatePlanRequest{PlanDate: raw}).NormalizedPlanDate()
			if err != nil || got == nil {
				t.Fatalf("input milliseconds %d rejected (error %v)", value, err)
			}
			if *got != value {
				t.Fatalf("input milliseconds %d became %d", value, *got)
			}
		})
	}
}

func TestPlanDateValidation(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`0`), json.RawMessage(`-1`), json.RawMessage(`1.5`), json.RawMessage(`"1971-01-01"`), json.RawMessage(`true`)} {
		if _, err := (UpdatePlanRequest{PlanDate: raw}).NormalizedPlanDate(); err == nil {
			t.Fatalf("expected invalid timestamp %s to be rejected", raw)
		}
	}
	value, err := (UpdatePlanRequest{PlanDate: json.RawMessage(`null`)}).NormalizedPlanDate()
	if err != nil || value != nil {
		t.Fatalf("null must clear the date: %v, %v", value, err)
	}
}

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
