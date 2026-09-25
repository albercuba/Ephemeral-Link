package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ephemeral-link/internal/config"
)

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
