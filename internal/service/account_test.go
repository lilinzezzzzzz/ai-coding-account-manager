package service

import (
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

func TestNormalizeUsageSnapshotConvertsResetsAtSecondsToMillis(t *testing.T) {
	resetsAtSeconds := int64(1700000000)
	snapshot := normalizeUsageSnapshot(entity.Account{
		ProviderID: "codex",
		AccountID:  "acct-1",
	}, entity.UsageSnapshot{
		ResetsAt: &resetsAtSeconds,
	})

	if snapshot.ResetsAt == nil || *snapshot.ResetsAt != 1700000000000 {
		t.Fatalf("resets at = %v, want milliseconds", snapshot.ResetsAt)
	}
}
