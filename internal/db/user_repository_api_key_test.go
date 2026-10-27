package db

import (
	"context"
	"testing"
	"time"
)

// API key storage (migration 042). The repository stores only the keyed
// authenticator, hands a row back by it, arms a shown-once reveal mark on every
// mint, and drops the key together with every other bearer credential on a
// compromise.

func TestAPIKeyLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := openRevealMarkRepoForTest(t)
	user := createUserForRevealMarkTest(t, repo, "api-key-lifecycle@example.com")
	now := time.Now().UTC().Truncate(time.Second)

	if err := repo.SaveAPIKey(ctx, user.ID, "mac-one", "read", now); err != nil {
		t.Fatalf("SaveAPIKey: %v", err)
	}
	found, ok, err := repo.FindByAPIKeyMAC(ctx, "mac-one")
	if err != nil || !ok || found.ID != user.ID || found.APIKeyScope != "read" {
		t.Fatalf("FindByAPIKeyMAC = %+v ok=%v err=%v, want the owner with scope read", found, ok, err)
	}
	if _, ok, _ := repo.FindByAPIKeyMAC(ctx, "mac-other"); ok {
		t.Fatal("an unknown authenticator matched a row")
	}
	// A key-off row stores nothing, and the empty value must never find it.
	if _, ok, _ := repo.FindByAPIKeyMAC(ctx, ""); ok {
		t.Fatal("the empty authenticator matched a row")
	}

	status, err := repo.LoadAPIKeyStatus(ctx, user.ID)
	if err != nil || status.APIKeyHash == "" || status.APIKeyScope != "read" || status.APIKeyCreatedAt == nil {
		t.Fatalf("LoadAPIKeyStatus = %+v err=%v, want a configured read key with a mint time", status, err)
	}

	// The reveal is single use per mint, and a new mint re-arms it.
	if claimed, err := repo.ClaimAPIKeyReveal(ctx, user.ID, now); err != nil || !claimed {
		t.Fatalf("first claim = %v err=%v, want true", claimed, err)
	}
	if claimed, _ := repo.ClaimAPIKeyReveal(ctx, user.ID, now); claimed {
		t.Fatal("a second claim of the same reveal succeeded")
	}
	if err := repo.SaveAPIKey(ctx, user.ID, "mac-two", "period", now); err != nil {
		t.Fatalf("SaveAPIKey (replace): %v", err)
	}
	if _, ok, _ := repo.FindByAPIKeyMAC(ctx, "mac-one"); ok {
		t.Fatal("the replaced key still authenticates")
	}
	if claimed, err := repo.ClaimAPIKeyReveal(ctx, user.ID, now); err != nil || !claimed {
		t.Fatalf("claim after a re-mint = %v err=%v, want true", claimed, err)
	}

	if err := repo.ClearAPIKey(ctx, user.ID); err != nil {
		t.Fatalf("ClearAPIKey: %v", err)
	}
	if _, ok, _ := repo.FindByAPIKeyMAC(ctx, "mac-two"); ok {
		t.Fatal("a revoked key still authenticates")
	}
	if err := repo.ClearAPIKey(ctx, user.ID); err != nil {
		t.Fatalf("ClearAPIKey is not idempotent: %v", err)
	}
}

func TestAPIKeyIsScopedToItsOwner(t *testing.T) {
	ctx := context.Background()
	repo := openRevealMarkRepoForTest(t)
	first := createUserForRevealMarkTest(t, repo, "api-key-first@example.com")
	second := createUserForRevealMarkTest(t, repo, "api-key-second@example.com")
	now := time.Now().UTC()

	if err := repo.SaveAPIKey(ctx, first.ID, "mac-first", "read", now); err != nil {
		t.Fatalf("SaveAPIKey: %v", err)
	}
	if err := repo.ClearAPIKey(ctx, second.ID); err != nil {
		t.Fatalf("ClearAPIKey on another owner: %v", err)
	}
	if _, ok, _ := repo.FindByAPIKeyMAC(ctx, "mac-first"); !ok {
		t.Fatal("clearing another owner's key revoked this one")
	}
	if err := repo.SaveAPIKey(ctx, 0, "mac-zero", "read", now); err == nil {
		t.Fatal("SaveAPIKey accepted a zero owner id")
	}
}

// TestEveryCompromiseWriteRevokesTheAPIKey holds the revocation arms: each write
// that already clears the calendar feed token on a compromise, plus the data
// wipe, must leave no working key behind.
func TestEveryCompromiseWriteRevokesTheAPIKey(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	arm := func(t *testing.T, email string) (*UserRepository, uint) {
		t.Helper()
		repo := openRevealMarkRepoForTest(t)
		user := createUserForRevealMarkTest(t, repo, email)
		if err := repo.SaveAPIKey(ctx, user.ID, "mac-"+email, "period", now); err != nil {
			t.Fatalf("SaveAPIKey: %v", err)
		}
		return repo, user.ID
	}
	revoked := func(t *testing.T, repo *UserRepository, userID uint, what string) {
		t.Helper()
		status, err := repo.LoadAPIKeyStatus(ctx, userID)
		if err != nil {
			t.Fatalf("LoadAPIKeyStatus: %v", err)
		}
		if status.APIKeyHash != "" || status.APIKeyScope != "" || status.APIKeyCreatedAt != nil {
			t.Fatalf("%s left an API key standing: %+v", what, status)
		}
	}

	t.Run("UpdateRecoveryCodeHashAndRevokeSessions", func(t *testing.T) {
		repo, userID := arm(t, "api-key-recovery-rotate@example.com")
		version := storedSessionVersionForTest(t, repo, userID)
		if err := repo.UpdateRecoveryCodeHashAndRevokeSessions(ctx, userID, version, "new-recovery-hash", nil); err != nil {
			t.Fatalf("UpdateRecoveryCodeHashAndRevokeSessions: %v", err)
		}
		revoked(t, repo, userID, "a recovery-code rotation")
	})

	t.Run("ForceResetPasswordAndRevokeSessions", func(t *testing.T) {
		repo, userID := arm(t, "api-key-force-reset@example.com")
		if err := repo.ForceResetPasswordAndRevokeSessions(ctx, userID, "new-password-hash"); err != nil {
			t.Fatalf("ForceResetPasswordAndRevokeSessions: %v", err)
		}
		revoked(t, repo, userID, "the operator-forced reset")
	})

	t.Run("UpdatePasswordRecoveryCodeAndRevokeSessionsCAS", func(t *testing.T) {
		repo, userID := arm(t, "api-key-recovery-cas@example.com")
		user, err := repo.FindByID(ctx, userID)
		if err != nil {
			t.Fatalf("reload user: %v", err)
		}
		version := storedSessionVersionForTest(t, repo, userID)
		if err := repo.UpdatePasswordRecoveryCodeAndRevokeSessionsCAS(ctx, userID, user.PasswordHash, version, "new-password-hash", "new-recovery-hash", nil); err != nil {
			t.Fatalf("UpdatePasswordRecoveryCodeAndRevokeSessionsCAS: %v", err)
		}
		revoked(t, repo, userID, "the recovery-code password reset")
	})

	t.Run("ClearAllDataAndResetSettings", func(t *testing.T) {
		repo, userID := arm(t, "api-key-clear-data@example.com")
		version := storedSessionVersionForTest(t, repo, userID)
		if err := repo.ClearAllDataAndResetSettings(ctx, userID, version); err != nil {
			t.Fatalf("ClearAllDataAndResetSettings: %v", err)
		}
		revoked(t, repo, userID, "the clear-data wipe")
	})
}
