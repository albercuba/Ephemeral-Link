package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ephemeral-link/internal/config"
)

func TestParseAPIKeyScopesValidatesAndDeduplicates(t *testing.T) {
	scopes, err := parseAPIKeyScopes([]string{"reports:read,reports:write", "reports:read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 2 || scopes[0] != "reports:read" || scopes[1] != "reports:write" {
		t.Fatalf("scopes = %#v", scopes)
	}
	if _, err := parseAPIKeyScopes([]string{"unknown"}); err == nil {
		t.Fatal("unsupported scope was accepted")
	}
}

func TestTrustedProxyMiddlewareUsesForwardedClientOnlyForTrustedPeer(t *testing.T) {
	trusted, err := parseTrustedProxies([]string{"10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{trustedProxies: trusted}
	handler := app.trustedProxyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Client", clientAddress(r))
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "trusted proxy", remoteAddr: "10.0.0.1:443", forwarded: "198.51.100.20, 10.0.0.1", want: "198.51.100.20"},
		{name: "untrusted peer", remoteAddr: "10.0.0.2:443", forwarded: "198.51.100.20", want: "10.0.0.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = tt.remoteAddr
			request.Header.Set("X-Forwarded-For", tt.forwarded)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if got := recorder.Header().Get("X-Test-Client"); got != tt.want {
				t.Fatalf("client address = %q, want %q", got, tt.want)
			}
		})
	}

	if !net.ParseIP("198.51.100.20").IsGlobalUnicast() {
		t.Fatal("test address should be a valid unicast address")
	}
}

func TestCSVSafeCellNeutralizesFormulas(t *testing.T) {
	tests := map[string]string{
		"=SUM(A1:A2)":  "'=SUM(A1:A2)",
		" +cmd":        "' +cmd",
		"\t@import":    "'\t@import",
		"normal value": "normal value",
		"":             "",
	}
	for input, want := range tests {
		if got := csvSafeCell(input); got != want {
			t.Fatalf("csvSafeCell(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSecurityHeadersForSensitivePages(t *testing.T) {
	app := &App{cfg: config.Config{SecureCookies: true}}
	handler := app.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	handler.ServeHTTP(recorder, request)

	headers := recorder.Result().Header
	if got := headers.Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := headers.Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q", got)
	}
	if got := headers.Get("Strict-Transport-Security"); !strings.Contains(got, "max-age=31536000") {
		t.Fatalf("Strict-Transport-Security = %q", got)
	}
	csp := headers.Get("Content-Security-Policy")
	if !strings.Contains(csp, "style-src 'self'") || strings.Contains(csp, "fonts.googleapis.com") || strings.Contains(csp, "cdnjs.cloudflare.com") {
		t.Fatalf("unexpected CSP: %q", csp)
	}
}

func TestSecurityHeadersNoStoreSensitiveRouteMatrix(t *testing.T) {
	app := &App{cfg: config.Config{SecureCookies: true}}
	handler := app.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{"/s/demo", "/s/demo/reveal", "/f/demo", "/f/demo/download", "/login", "/setup", "/admin", "/created"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			handler.ServeHTTP(recorder, request)
			if got := recorder.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
				t.Fatalf("Cache-Control = %q", got)
			}
			if got := recorder.Header().Get("Strict-Transport-Security"); !strings.Contains(got, "max-age=31536000") {
				t.Fatalf("Strict-Transport-Security = %q", got)
			}
		})
	}
}

func TestSecurityHeadersAllowStaticCaching(t *testing.T) {
	app := &App{}
	handler := app.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	handler.ServeHTTP(recorder, request)

	if got := recorder.Result().Header.Get("Cache-Control"); got != "" {
		t.Fatalf("Cache-Control for static asset = %q", got)
	}
}

func TestIntegrationSecretEncryptionRoundTrip(t *testing.T) {
	app := &App{cfg: config.Config{EncryptionMasterKey: []byte("0123456789abcdef0123456789abcdef")}}
	encrypted, err := app.encryptIntegrationSecret(" secret ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encrypted, encryptedSecretPrefix) {
		t.Fatalf("encrypted secret does not have prefix: %q", encrypted)
	}
	if encrypted == "secret" || strings.Contains(encrypted, " secret ") {
		t.Fatalf("encrypted secret appears to contain plaintext: %q", encrypted)
	}
	plain, err := app.decryptIntegrationSecret(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "secret" {
		t.Fatalf("decrypted secret = %q, want secret", plain)
	}
	again, err := app.encryptIntegrationSecret(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if again != encrypted {
		t.Fatal("already encrypted secret was encrypted again")
	}
}

func TestIntegrationSecretDecryptAllowsPlaintextLegacyValues(t *testing.T) {
	app := &App{cfg: config.Config{EncryptionMasterKey: []byte("0123456789abcdef0123456789abcdef")}}
	plain, err := app.decryptIntegrationSecret("legacy-secret")
	if err != nil {
		t.Fatal(err)
	}
	if plain != "legacy-secret" {
		t.Fatalf("legacy plaintext = %q", plain)
	}
}

func TestCSRFMiddlewareAllowsValidToken(t *testing.T) {
	app := &App{}
	called := false
	handler := app.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("csrf=token"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: "csrf", Value: "token"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if !called {
		t.Fatal("next handler was not called")
	}
	if got := recorder.Result().StatusCode; got != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", got, http.StatusNoContent)
	}
}

func TestCSRFMiddlewareRejectsMissingOrInvalidToken(t *testing.T) {
	app := &App{}
	handler := app.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))
	tests := []struct {
		name   string
		body   string
		cookie string
	}{
		{name: "missing", body: "", cookie: "token"},
		{name: "invalid", body: "csrf=wrong", cookie: "token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.AddCookie(&http.Cookie{Name: "csrf", Value: tt.cookie})
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if got := recorder.Result().StatusCode; got != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", got, http.StatusForbidden)
			}
		})
	}
}

func TestCSRFMiddlewareRejectsOversizedBody(t *testing.T) {
	app := &App{}
	handler := app.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))
	body := "csrf=token&payload=" + strings.Repeat("a", int(maxFormBodySize)+1)
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: "csrf", Value: "token"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Result().StatusCode; got != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", got, http.StatusRequestEntityTooLarge)
	}
}

func TestPostBodyLimitIsRouteAware(t *testing.T) {
	app := &App{cfg: config.Config{MaxFileSize: 10}}
	tests := map[string]int64{
		"/login":                maxFormBodySize,
		"/admin/logo":           maxLogoBodySize,
		"/secrets/file":         10 + maxFormBodySize,
		"/upload/request-token": 10 + maxFormBodySize,
	}
	for path, want := range tests {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		if got := app.postBodyLimit(request); got != want {
			t.Fatalf("postBodyLimit(%q) = %d, want %d", path, got, want)
		}
	}
}
