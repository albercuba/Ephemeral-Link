package web

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ephemeral-link/internal/redisstore"
)

func TestValidateMicrosoftAccessToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discovery/v2.0/keys" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(jwksResponse{Keys: []jwkKey{jwkFromRSA(kid, &key.PublicKey)}})
	}))
	defer server.Close()

	cfg := redisstore.IntegrationConfig{MicrosoftAuthority: server.URL, MicrosoftTenantID: "tenant", MicrosoftAudience: "api://api-client/access_as_user"}
	claims := microsoftClaims{Audience: "api://api-client", Issuer: server.URL + "/v2.0", Expires: time.Now().Add(time.Hour).Unix(), NotBefore: time.Now().Add(-time.Minute).Unix(), ObjectID: "object-id"}

	t.Run("valid", func(t *testing.T) {
		token := signedMicrosoftTestJWT(t, key, kid, claims)
		got, err := validateMicrosoftAccessToken(context.Background(), cfg, token)
		if err != nil {
			t.Fatal(err)
		}
		if got.ObjectID != claims.ObjectID {
			t.Fatalf("object id = %q", got.ObjectID)
		}
	})

	t.Run("invalid audience", func(t *testing.T) {
		bad := claims
		bad.Audience = "api://other"
		token := signedMicrosoftTestJWT(t, key, kid, bad)
		if _, err := validateMicrosoftAccessToken(context.Background(), cfg, token); err == nil || !strings.Contains(err.Error(), "audience") {
			t.Fatalf("expected audience error, got %v", err)
		}
	})

	t.Run("invalid issuer", func(t *testing.T) {
		bad := claims
		bad.Issuer = "https://issuer.example.invalid/v2.0"
		token := signedMicrosoftTestJWT(t, key, kid, bad)
		if _, err := validateMicrosoftAccessToken(context.Background(), cfg, token); err == nil || !strings.Contains(err.Error(), "issuer") {
			t.Fatalf("expected issuer error, got %v", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		bad := claims
		bad.Expires = time.Now().Add(-time.Minute).Unix()
		token := signedMicrosoftTestJWT(t, key, kid, bad)
		if _, err := validateMicrosoftAccessToken(context.Background(), cfg, token); err == nil || !strings.Contains(err.Error(), "expired") {
			t.Fatalf("expected expiry error, got %v", err)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		token := signedMicrosoftTestJWT(t, key, kid, claims) + "tampered"
		if _, err := validateMicrosoftAccessToken(context.Background(), cfg, token); err == nil {
			t.Fatal("expected signature error")
		}
	})
}

func TestValidateMicrosoftIDTokenNonce(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksResponse{Keys: []jwkKey{jwkFromRSA(kid, &key.PublicKey)}})
	}))
	defer server.Close()

	cfg := redisstore.IntegrationConfig{MicrosoftAuthority: server.URL, MicrosoftTenantID: "tenant", MicrosoftClientID: "frontend-client"}
	claims := microsoftClaims{Audience: "frontend-client", Issuer: server.URL + "/v2.0", Expires: time.Now().Add(time.Hour).Unix(), Nonce: "expected-nonce"}
	token := signedMicrosoftTestJWT(t, key, kid, claims)
	if _, err := validateMicrosoftIDToken(context.Background(), cfg, token, "wrong-nonce"); err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("expected nonce error, got %v", err)
	}
}

func signedMicrosoftTestJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims microsoftClaims) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func jwkFromRSA(kid string, key *rsa.PublicKey) jwkKey {
	return jwkKey{Kid: kid, Kty: "RSA", Use: "sig", Alg: "RS256", N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}
}
