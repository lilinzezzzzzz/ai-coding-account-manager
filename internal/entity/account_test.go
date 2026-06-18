package entity_test

import (
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

func TestStorageIDForAccountUsesAccountID(t *testing.T) {
	first := entity.StorageIDForAccount("codex", "account-1")
	second := entity.StorageIDForAccount("codex", "account-1")

	if first != second {
		t.Fatalf("StorageIDForAccount() is not stable: %q != %q", first, second)
	}
	if first != "account-1" {
		t.Fatalf("storage id = %q, want account id", first)
	}
}
