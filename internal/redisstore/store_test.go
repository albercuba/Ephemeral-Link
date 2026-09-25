package redisstore

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	store, err := New("redis://" + server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, server
}

func TestLegacyRecordsNormalizeToDefaultWorkspace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	item := Item{ID: "workspace-default", Type: "text", CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if err := store.Create(ctx, item, time.Hour); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WorkspaceID != DefaultWorkspaceID {
		t.Fatalf("workspace = %q, want %q", loaded.WorkspaceID, DefaultWorkspaceID)
	}
}

func TestClaimIsAtomicAndWipesPayloadFields(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	item := Item{
		ID:                   "claim-test",
		Type:                 "secret",
		CreatedAt:            time.Now().Unix(),
		ExpiresAt:            time.Now().Add(time.Hour).Unix(),
		WrappedKeyNonce:      "wrapped-nonce",
		WrappedKeyCiphertext: "wrapped-ciphertext",
		PayloadNonce:         "payload-nonce",
		PayloadCiphertext:    "payload-ciphertext",
		StorageObjectPath:    "/private/claim-test.bin",
	}
	if err := store.Create(ctx, item, time.Hour); err != nil {
		t.Fatal(err)
	}

	const attempts = 16
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := store.Claim(ctx, item.ID)
			if err == nil {
				if claimed.ID != item.ID || claimed.PayloadCiphertext != item.PayloadCiphertext {
					results <- errors.New("winning claim did not return the payload")
					return
				}
			}
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	wins := 0
	gone := 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrGone):
			gone++
		default:
			t.Fatalf("claim failed unexpectedly: %v", err)
		}
	}
	if wins != 1 || gone != attempts-1 {
		t.Fatalf("claims: wins=%d gone=%d, want one winner and %d gone", wins, gone, attempts-1)
	}

	stored, err := store.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "consumed" {
		t.Fatalf("stored status = %q, want consumed", stored.Status)
	}
	if stored.WrappedKeyNonce != "" || stored.WrappedKeyCiphertext != "" || stored.PayloadNonce != "" || stored.PayloadCiphertext != "" || stored.StorageObjectPath != "" {
		t.Fatalf("claimed payload fields were retained: %#v", stored)
	}
}

func TestWorkspaceInvitationIsEmailBoundAndSingleUse(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	invitation, token, err := store.CreateWorkspaceInvitation(ctx, "team-a", "member@example.com", "user", time.Now().Add(time.Hour).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if invitation.WorkspaceID != "team-a" || token == "" {
		t.Fatalf("invitation = %#v", invitation)
	}
	if err := store.AddWorkspaceMembership(ctx, "member", "team-a", "user"); err != nil {
		t.Fatal(err)
	}
	member, err := store.HasWorkspaceMembership(ctx, "member", "team-a")
	if err != nil || !member {
		t.Fatalf("membership = %v, %v", member, err)
	}
	accepted, err := store.AcceptWorkspaceInvitation(ctx, token)
	if err != nil || accepted.ID != invitation.ID {
		t.Fatalf("accepted invitation = %#v, %v", accepted, err)
	}
	if _, err := store.AcceptWorkspaceInvitation(ctx, token); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("second invitation acceptance error = %v", err)
	}
}

func TestAPIKeyIsOneTimePresentedScopedRevocableAndExpiring(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	key, value, err := store.CreateAPIKey(ctx, "reporting", []string{"reports:read"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if key.Hash == "" || value == "" {
		t.Fatal("API key was not generated")
	}
	if _, err := store.AuthenticateAPIKey(ctx, value, "admin"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("wrong scope error = %v", err)
	}
	authenticated, err := store.AuthenticateAPIKey(ctx, value, "reports:read")
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.ID != key.ID || authenticated.Name != "reporting" {
		t.Fatalf("authenticated key = %#v", authenticated)
	}
	if err := store.RevokeAPIKey(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthenticateAPIKey(ctx, value, "reports:read"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("revoked key error = %v", err)
	}

	expiring, value, err := store.CreateAPIKey(ctx, "temporary", []string{"reports:read"}, time.Now().Add(-time.Minute).Unix())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthenticateAPIKey(ctx, value, "reports:read"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("expired key error = %v", err)
	}
	if expiring.ID == key.ID {
		t.Fatal("API key IDs were duplicated")
	}
}

func TestWorkspaceScopedAPIKeyAdministration(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	teamA, _, err := store.CreateAPIKeyInWorkspace(ctx, "team-a", "a", []string{"reports:read"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	teamB, _, err := store.CreateAPIKeyInWorkspace(ctx, "team-b", "b", []string{"reports:read"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := store.ListAPIKeysInWorkspace(ctx, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != teamA.ID {
		t.Fatalf("team-a keys = %#v", keys)
	}
	if err := store.RevokeAPIKeyInWorkspace(ctx, teamB.ID, "team-a"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("cross-workspace revoke error = %v", err)
	}
	if key, err := store.ListAPIKeysInWorkspace(ctx, "team-b"); err != nil || len(key) != 1 || key[0].RevokedAt != 0 {
		t.Fatalf("team-b key changed after denied revoke: keys=%#v err=%v", key, err)
	}
}

func TestWorkspaceMembershipScopedUserAdministration(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	user := User{Username: "shared", WorkspaceID: "team-a", Role: "user", PasswordHash: "hash", CreatedAt: 1}
	if err := store.SaveUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := store.AddWorkspaceMembership(ctx, user.Username, "team-a", "user"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddWorkspaceMembership(ctx, user.Username, "team-b", "administrator"); err != nil {
		t.Fatal(err)
	}
	users, err := store.ListUsersInWorkspace(ctx, "team-b")
	if err != nil || len(users) != 1 || users[0].Username != user.Username {
		t.Fatalf("team-b users = %#v err=%v", users, err)
	}
	if err := store.RemoveWorkspaceMembership(ctx, user.Username, "team-a"); err != nil {
		t.Fatal(err)
	}
	member, err := store.HasWorkspaceMembership(ctx, user.Username, "team-a")
	if err != nil || member {
		t.Fatalf("team-a membership remains: member=%v err=%v", member, err)
	}
	member, err = store.HasWorkspaceMembership(ctx, user.Username, "team-b")
	if err != nil || !member {
		t.Fatalf("team-b membership missing: member=%v err=%v", member, err)
	}
}

func TestFailureThrottleBlocksAtLimitAndResets(t *testing.T) {
	store, server := newTestStore(t)
	ctx := context.Background()
	const limit int64 = 3
	for attempt := int64(1); attempt <= limit; attempt++ {
		blocked, err := store.RegisterFailure(ctx, "login", "user|198.51.100.10", limit, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if blocked != (attempt == limit) {
			t.Fatalf("attempt %d blocked = %v, want %v", attempt, blocked, attempt == limit)
		}
	}
	blocked, err := store.FailureLimitExceeded(ctx, "login", "user|198.51.100.10", limit)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("failure limit should be exceeded")
	}

	server.FastForward(2 * time.Minute)
	blocked, err = store.FailureLimitExceeded(ctx, "login", "user|198.51.100.10", limit)
	if err != nil {
		t.Fatal(err)
	}
	if blocked {
		t.Fatal("expired failure counter should not remain blocked")
	}
	if err := store.ResetFailures(ctx, "login", "user|198.51.100.10"); err != nil {
		t.Fatal(err)
	}
	blocked, err = store.FailureLimitExceeded(ctx, "login", "user|198.51.100.10", limit)
	if err != nil {
		t.Fatal(err)
	}
	if blocked {
		t.Fatal("reset failure counter should not remain blocked")
	}
}

func TestCreateInitialAdminAllowsOnlyOneAdministrator(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	const attempts = 12
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- store.CreateInitialAdmin(ctx, User{
				Username:     "admin-" + strconv.Itoa(i),
				PasswordHash: "test-hash",
				CreatedAt:    time.Now().Unix(),
			})
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful initial-admin creations = %d, want 1", successes)
	}

	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	admins := 0
	for _, user := range users {
		if user.Role == "administrator" {
			admins++
		}
	}
	if admins != 1 {
		t.Fatalf("administrator count = %d, want 1", admins)
	}
}
