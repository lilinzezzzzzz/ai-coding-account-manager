package dao

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/entity"
	"github.com/lilinzezzzzzz/ai-coding-account-manager/internal/infra/database"
)

func TestAccountDAOCreateGetAndDuplicateMapping(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	daos := NewDAOs(db.GORM())
	account := testAccount("codex", "acct-1", false)
	if err := daos.Accounts.Create(context.Background(), account); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := daos.Accounts.Get(context.Background(), "codex", "acct-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.StorageID != account.StorageID || got.Label != account.Label {
		t.Fatalf("Get() = %+v, want %+v", got, account)
	}

	err = daos.Accounts.Create(context.Background(), account)
	assertAppErrorCode(t, err, entity.ErrorCodeConflict)
}

func TestAccountDAOGetNotFoundMapping(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	_, err := NewAccountDAO(db.GORM()).Get(context.Background(), "codex", "missing")
	assertAppErrorCode(t, err, entity.ErrorCodeNotFound)
}

func TestSingleActiveAccountConstraint(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	accounts := NewAccountDAO(db.GORM())
	first := testAccount("codex", "acct-1", true)
	second := testAccount("codex", "acct-2", true)

	if err := accounts.Create(context.Background(), first); err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	err := accounts.Create(context.Background(), second)
	assertAppErrorCode(t, err, entity.ErrorCodeConflict)
}

func TestSetActiveSwitchesAccountWithinProvider(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	accounts := NewAccountDAO(db.GORM())
	if err := accounts.Create(context.Background(), testAccount("codex", "acct-1", true)); err != nil {
		t.Fatalf("Create(acct-1) error = %v", err)
	}
	if err := accounts.Create(context.Background(), testAccount("codex", "acct-2", false)); err != nil {
		t.Fatalf("Create(acct-2) error = %v", err)
	}

	if err := accounts.SetActive(context.Background(), "codex", "acct-2", 2000); err != nil {
		t.Fatalf("SetActive() error = %v", err)
	}

	first, err := accounts.Get(context.Background(), "codex", "acct-1")
	if err != nil {
		t.Fatalf("Get(acct-1) error = %v", err)
	}
	second, err := accounts.Get(context.Background(), "codex", "acct-2")
	if err != nil {
		t.Fatalf("Get(acct-2) error = %v", err)
	}
	if first.IsActive {
		t.Fatal("acct-1 is active, want inactive")
	}
	if !second.IsActive || second.LastUsedAt == nil || *second.LastUsedAt != 2000 {
		t.Fatalf("acct-2 = %+v, want active with last_used_at 2000", second)
	}
}

func TestListAllOrdersActiveFirstThenCreatedAtDescending(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	accounts := NewAccountDAO(db.GORM())
	activeOld := testAccount("codex", "acct-active-old", true)
	activeOld.CreatedAt = 1000
	inactiveNew := testAccount("codex", "acct-inactive-new", false)
	inactiveNew.CreatedAt = 3000
	inactiveOld := testAccount("codex", "acct-inactive-old", false)
	inactiveOld.CreatedAt = 2000

	for _, account := range []entity.Account{inactiveOld, inactiveNew, activeOld} {
		if err := accounts.Create(context.Background(), account); err != nil {
			t.Fatalf("Create(%s) error = %v", account.AccountID, err)
		}
	}

	got, err := accounts.ListAll(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	gotIDs := make([]string, 0, len(got))
	for _, account := range got {
		gotIDs = append(gotIDs, account.AccountID)
	}
	wantIDs := []string{"acct-active-old", "acct-inactive-new", "acct-inactive-old"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("ListAll() ids = %v, want %v", gotIDs, wantIDs)
	}
	for index, want := range wantIDs {
		if gotIDs[index] != want {
			t.Fatalf("ListAll() ids = %v, want %v", gotIDs, wantIDs)
		}
	}
}

func TestUpdateProviderMetadataPreservesManualPlan(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	accounts := NewAccountDAO(db.GORM())
	account := testAccount("codex", "acct-1", false)
	existingPlanDate := int64(1767225600000)
	account.PlanDate = &existingPlanDate
	autoRenew := true
	account.PlanAutoRenew = autoRenew
	if err := accounts.Create(context.Background(), account); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	email := "user@example.com"
	planType := "plus"
	if err := accounts.UpdateProviderMetadata(context.Background(), "codex", "acct-1", &email, &planType, 2000); err != nil {
		t.Fatalf("UpdateProviderMetadata() error = %v", err)
	}

	got, err := accounts.Get(context.Background(), "codex", "acct-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Email == nil || *got.Email != email {
		t.Fatalf("email = %v, want %s", got.Email, email)
	}
	if got.PlanType == nil || *got.PlanType != planType {
		t.Fatalf("plan type = %v, want %s", got.PlanType, planType)
	}
	if got.PlanDate == nil || *got.PlanDate != existingPlanDate {
		t.Fatalf("plan date = %v, want %d", got.PlanDate, existingPlanDate)
	}
	if !got.PlanAutoRenew {
		t.Fatalf("manual renewal metadata lost after provider refresh: %+v", got)
	}
	if got.UpdatedAt != 2000 {
		t.Fatalf("updated at = %d, want 2000", got.UpdatedAt)
	}
	// Reimporting metadata must not clear any of the manually maintained fields.
	if err := accounts.Upsert(context.Background(), testAccount("codex", "acct-1", false)); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	got, err = accounts.Get(context.Background(), "codex", "acct-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.PlanAutoRenew || got.PlanDate == nil || *got.PlanDate != existingPlanDate {
		t.Fatalf("manual plan lost after reimport: %+v", got)
	}
}

func TestUpdatePlanPersistsAndClearsDate(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	accounts := NewAccountDAO(db.GORM())
	if err := accounts.Create(context.Background(), testAccount("codex", "acct-1", false)); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	planDate := int64(1767225600000)
	if err := accounts.UpdatePlan(context.Background(), "codex", "acct-1", true, &planDate, 2000); err != nil {
		t.Fatalf("UpdatePlan(set) error = %v", err)
	}
	got, err := accounts.Get(context.Background(), "codex", "acct-1")
	if err != nil {
		t.Fatalf("Get() after set error = %v", err)
	}
	if got.PlanDate == nil || *got.PlanDate != planDate {
		t.Fatalf("plan date after set = %v, want %d", got.PlanDate, planDate)
	}

	if err := accounts.UpdatePlan(context.Background(), "codex", "acct-1", true, nil, 3000); err != nil {
		t.Fatalf("UpdatePlan(clear) error = %v", err)
	}
	got, err = accounts.Get(context.Background(), "codex", "acct-1")
	if err != nil {
		t.Fatalf("Get() after clear error = %v", err)
	}
	if got.PlanDate != nil {
		t.Fatalf("plan date after clear = %v, want nil", got.PlanDate)
	}
	if got.UpdatedAt != 3000 {
		t.Fatalf("updated at after clear = %d, want 3000", got.UpdatedAt)
	}
}

func TestUsageForeignKeyAndCascadeDelete(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	daos := NewDAOs(db.GORM())
	snapshot := testUsageSnapshot("codex", "acct-1")
	err := daos.UsageSnapshots.Upsert(context.Background(), snapshot)
	assertAppErrorCode(t, err, entity.ErrorCodeConflict)

	if err := daos.Accounts.Create(context.Background(), testAccount("codex", "acct-1", false)); err != nil {
		t.Fatalf("Create(account) error = %v", err)
	}
	if err := daos.UsageSnapshots.Upsert(context.Background(), snapshot); err != nil {
		t.Fatalf("Upsert(snapshot) error = %v", err)
	}
	if err := daos.Accounts.Delete(context.Background(), "codex", "acct-1"); err != nil {
		t.Fatalf("Delete(account) error = %v", err)
	}

	_, err = daos.UsageSnapshots.Get(context.Background(), "codex", "acct-1")
	assertAppErrorCode(t, err, entity.ErrorCodeNotFound)
}

func TestUnitOfWorkCommitAndRollback(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	uow := NewUnitOfWork(db.GORM())
	err := uow.WithinTransaction(context.Background(), func(daos DAOs) error {
		return daos.Accounts.Create(context.Background(), testAccount("codex", "commit", false))
	})
	if err != nil {
		t.Fatalf("commit transaction error = %v", err)
	}
	if _, err := NewAccountDAO(db.GORM()).Get(context.Background(), "codex", "commit"); err != nil {
		t.Fatalf("Get(committed) error = %v", err)
	}

	rollbackErr := entity.NewAppError(entity.ErrorCodeConflict)
	err = uow.WithinTransaction(context.Background(), func(daos DAOs) error {
		if err := daos.Accounts.Create(context.Background(), testAccount("codex", "rollback", false)); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback error = %v, want %v", err, rollbackErr)
	}
	_, err = NewAccountDAO(db.GORM()).Get(context.Background(), "codex", "rollback")
	assertAppErrorCode(t, err, entity.ErrorCodeNotFound)
}

func TestMapDatabaseErrorDetectsBusyString(t *testing.T) {
	err := mapDatabaseError(errors.New("database is locked"))
	assertAppErrorCode(t, err, entity.ErrorCodeStorageBusy)
}

func TestDAORespectsCanceledContext(t *testing.T) {
	db := openDAOTestDatabase(t)
	defer closeDAOTestDatabase(t, db)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewAccountDAO(db.GORM()).Create(ctx, testAccount("codex", "acct-1", false))
	if err == nil {
		t.Fatal("Create() error = nil, want context error")
	}
}

func testAccount(providerID string, accountID string, active bool) entity.Account {
	return entity.Account{
		ProviderID: providerID,
		AccountID:  accountID,
		StorageID:  entity.StorageIDForAccount(providerID, accountID),
		Label:      accountID,
		IsActive:   active,
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
}

func testUsageSnapshot(providerID string, accountID string) entity.UsageSnapshot {
	usedPercent := 42.5
	snapshotJSON := `{"buckets":[]}`
	return entity.UsageSnapshot{
		ProviderID:   providerID,
		AccountID:    accountID,
		Status:       entity.UsageStatusReady,
		UsedPercent:  &usedPercent,
		SnapshotJSON: &snapshotJSON,
		RefreshedAt:  1500,
	}
}

func openDAOTestDatabase(t *testing.T) *database.DB {
	t.Helper()

	db, err := database.Open(context.Background(), database.Config{
		Path: filepath.Join(t.TempDir(), "state.db"),
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return db
}

func closeDAOTestDatabase(t *testing.T, db *database.DB) {
	t.Helper()

	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func assertAppErrorCode(t *testing.T, err error, want entity.ErrorCode) {
	t.Helper()

	var appErr *entity.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, want AppError %s", err, want)
	}
	if appErr.ErrorCode() != want {
		t.Fatalf("ErrorCode() = %q, want %q", appErr.ErrorCode(), want)
	}
}
