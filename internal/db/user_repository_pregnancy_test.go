package db

import (
	"context"
	"testing"
	"time"

	"github.com/ovumcy/ovumcy-web/internal/models"
)

func pregnancyTestDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func reloadPregnancy(t *testing.T, repo *UserRepository, userID uint) models.User {
	t.Helper()
	var reloaded models.User
	if err := repo.database.First(&reloaded, userID).Error; err != nil {
		t.Fatalf("reload user %d: %v", userID, err)
	}
	return reloaded
}

func TestStartPregnancyStoresTheDueDateAndClearsAPriorEnding(t *testing.T) {
	repo := openTimezoneRepoForTest(t)
	user := createUserForTimezoneTest(t, repo, "pregnancy-start-repo@example.com")
	ended := pregnancyTestDate(2026, time.January, 5)
	if err := repo.database.Model(&models.User{}).Where("id = ?", user.ID).Update("pregnancy_ended_on", ended).Error; err != nil {
		t.Fatalf("seed ended: %v", err)
	}
	before := reloadPregnancy(t, repo, user.ID).WebhookConfigVersion

	due := pregnancyTestDate(2026, time.October, 8)
	if err := repo.StartPregnancy(context.Background(), user.ID, due); err != nil {
		t.Fatalf("StartPregnancy: %v", err)
	}

	got := reloadPregnancy(t, repo, user.ID)
	if got.PregnancyDueDate == nil || !got.PregnancyDueDate.Equal(due) {
		t.Fatalf("due date = %v, want %s", got.PregnancyDueDate, due)
	}
	if got.PregnancyEndedOn != nil {
		t.Fatalf("a new declaration must clear the prior ending, got %v", got.PregnancyEndedOn)
	}
	if got.WebhookConfigVersion != before+1 {
		t.Fatalf("webhook_config_version %d -> %d, want +1", before, got.WebhookConfigVersion)
	}
}

func TestEndPregnancyIsACompareAndSetOnAnActivePregnancy(t *testing.T) {
	repo := openTimezoneRepoForTest(t)
	user := createUserForTimezoneTest(t, repo, "pregnancy-end-repo@example.com")
	ctx := context.Background()

	ended, err := repo.EndPregnancy(ctx, user.ID, pregnancyTestDate(2026, time.March, 1))
	if err != nil || ended {
		t.Fatalf("ending with nothing active: ended=%t err=%v, want a false no-op", ended, err)
	}
	if got := reloadPregnancy(t, repo, user.ID); got.PregnancyEndedOn != nil {
		t.Fatalf("a no-op end must not record an ending, got %v", got.PregnancyEndedOn)
	}

	if err := repo.StartPregnancy(ctx, user.ID, pregnancyTestDate(2026, time.October, 8)); err != nil {
		t.Fatalf("StartPregnancy: %v", err)
	}
	version := reloadPregnancy(t, repo, user.ID).WebhookConfigVersion

	endDay := pregnancyTestDate(2026, time.June, 1)
	ended, err = repo.EndPregnancy(ctx, user.ID, endDay)
	if err != nil || !ended {
		t.Fatalf("ending an active pregnancy: ended=%t err=%v", ended, err)
	}
	got := reloadPregnancy(t, repo, user.ID)
	if got.PregnancyDueDate != nil || got.PregnancyEndedOn == nil || !got.PregnancyEndedOn.Equal(endDay) {
		t.Fatalf("got due=%v ended=%v, want cleared due and ended %s", got.PregnancyDueDate, got.PregnancyEndedOn, endDay)
	}
	if got.WebhookConfigVersion != version+1 {
		t.Fatalf("webhook_config_version %d -> %d, want +1", version, got.WebhookConfigVersion)
	}

	again, err := repo.EndPregnancy(ctx, user.ID, pregnancyTestDate(2026, time.June, 9))
	if err != nil || again {
		t.Fatalf("a second end must be a no-op, got ended=%t err=%v", again, err)
	}
	if final := reloadPregnancy(t, repo, user.ID); !final.PregnancyEndedOn.Equal(endDay) {
		t.Fatalf("a second end must not rewrite the ending day, got %v", final.PregnancyEndedOn)
	}
}

func TestPregnancyWritesAreScopedToTheTargetUserAndRefuseAZeroID(t *testing.T) {
	repo := openTimezoneRepoForTest(t)
	owner := createUserForTimezoneTest(t, repo, "pregnancy-scope-a@example.com")
	bystander := createUserForTimezoneTest(t, repo, "pregnancy-scope-b@example.com")
	ctx := context.Background()
	due := pregnancyTestDate(2026, time.October, 8)

	if err := repo.StartPregnancy(ctx, owner.ID, due); err != nil {
		t.Fatalf("StartPregnancy: %v", err)
	}
	if got := reloadPregnancy(t, repo, bystander.ID); got.PregnancyDueDate != nil {
		t.Fatalf("the bystander's row must be untouched, got %v", got.PregnancyDueDate)
	}

	if err := repo.StartPregnancy(ctx, 0, due); err == nil {
		t.Fatal("StartPregnancy with a zero id must be refused")
	}
	if _, err := repo.EndPregnancy(ctx, 0, due); err == nil {
		t.Fatal("EndPregnancy with a zero id must be refused")
	}
}

func TestLoadSettingsByIDReturnsThePregnancyRecord(t *testing.T) {
	repo := openTimezoneRepoForTest(t)
	user := createUserForTimezoneTest(t, repo, "pregnancy-load@example.com")
	due := pregnancyTestDate(2026, time.October, 8)
	if err := repo.StartPregnancy(context.Background(), user.ID, due); err != nil {
		t.Fatalf("StartPregnancy: %v", err)
	}

	loaded, err := repo.LoadSettingsByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("LoadSettingsByID: %v", err)
	}
	if loaded.PregnancyDueDate == nil || !loaded.PregnancyDueDate.Equal(due) {
		t.Fatalf("LoadSettingsByID dropped pregnancy_due_date: %v", loaded.PregnancyDueDate)
	}
}

// The notify projection is a whitelist: a pregnancy column left off it would
// build the pass's user without a pregnancy and let a reminder leave the host.
func TestListAllForNotifyProjectsThePregnancyRecord(t *testing.T) {
	repo := openTimezoneRepoForTest(t)
	user := createUserForTimezoneTest(t, repo, "pregnancy-notify@example.com")
	due := pregnancyTestDate(2026, time.October, 8)
	ended := pregnancyTestDate(2026, time.January, 5)
	if err := repo.database.Model(&models.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{"pregnancy_due_date": due, "pregnancy_ended_on": ended}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	records, err := repo.ListAllForNotify(context.Background())
	if err != nil {
		t.Fatalf("ListAllForNotify: %v", err)
	}
	for _, record := range records {
		if record.ID != user.ID {
			continue
		}
		if record.PregnancyDueDate == nil || !record.PregnancyDueDate.Equal(due) || record.PregnancyEndedOn == nil || !record.PregnancyEndedOn.Equal(ended) {
			t.Fatalf("the notify projection lost the pregnancy record: %+v", record)
		}
		return
	}
	t.Fatal("the seeded owner is missing from the notify projection")
}

func TestClearAllDataResetsThePregnancyRecord(t *testing.T) {
	repo := openTimezoneRepoForTest(t)
	user := createUserForTimezoneTest(t, repo, "pregnancy-clear@example.com")
	ctx := context.Background()
	if err := repo.StartPregnancy(ctx, user.ID, pregnancyTestDate(2026, time.October, 8)); err != nil {
		t.Fatalf("StartPregnancy: %v", err)
	}
	if err := repo.database.Model(&models.User{}).Where("id = ?", user.ID).
		Update("pregnancy_ended_on", pregnancyTestDate(2026, time.January, 5)).Error; err != nil {
		t.Fatalf("seed ended: %v", err)
	}

	if err := repo.ClearAllDataAndResetSettings(ctx, user.ID, storedSessionVersionForTest(t, repo, user.ID)); err != nil {
		t.Fatalf("ClearAllDataAndResetSettings: %v", err)
	}
	if got := reloadPregnancy(t, repo, user.ID); got.PregnancyDueDate != nil || got.PregnancyEndedOn != nil {
		t.Fatalf("a wipe must erase the pregnancy record, got due=%v ended=%v", got.PregnancyDueDate, got.PregnancyEndedOn)
	}
}
