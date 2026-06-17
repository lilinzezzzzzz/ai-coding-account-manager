package entity_test

import (
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
)

func TestStorageIDForAccountIsStableAndDoesNotUseEmail(t *testing.T) {
	first := entity.StorageIDForAccount("codex", "account-1")
	second := entity.StorageIDForAccount("codex", "account-1")
	otherProvider := entity.StorageIDForAccount("other", "account-1")

	if first != second {
		t.Fatalf("StorageIDForAccount() is not stable: %q != %q", first, second)
	}
	if len(first) != 32 {
		t.Fatalf("storage id length = %d, want 32", len(first))
	}
	if first == otherProvider {
		t.Fatal("storage id should include provider id")
	}
}
