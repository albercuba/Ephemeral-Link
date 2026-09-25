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
