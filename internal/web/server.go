package web

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"ephemeral-link/internal/config"
	sec "ephemeral-link/internal/crypto"
	"ephemeral-link/internal/i18n"
	"ephemeral-link/internal/ratelimit"
	"ephemeral-link/internal/redisstore"
	"ephemeral-link/internal/storage"
)

type App struct {
	cfg            config.Config
	store          *redisstore.Store
	files          *storage.Local
	i18n           *i18n.Bundle
	log            *slog.Logger
	templates      *template.Template
	trustedProxies []*net.IPNet
}

const appVersion = "v1.0.0"
const encryptedSecretPrefix = "enc:v1:"
const createdReceiptCookieName = "ephemeral_created_receipt"
const createdReceiptTTL = 10 * time.Minute
const maxFormBodySize int64 = 1024 * 1024
const maxLogoBodySize int64 = 2 * 1024 * 1024

type Page struct {
	Title, Lang, CSRF, Error, Link, ID, Kind, Secret, Filename, Mime, Message, LogoURL, Version string
	Size                                                                                        int64
	ExpiresAt                                                                                   string
	HasPassphrase                                                                               bool
	EmailConfigured                                                                             bool
	MicrosoftLoginEnabled                                                                       bool
	ADLoginEnabled                                                                              bool
	User                                                                                        *redisstore.User
	Users                                                                                       []redisstore.User
	Items                                                                                       []redisstore.Item
	UploadRequests                                                                              []redisstore.UploadRequest
	AuditEvents                                                                                 []redisstore.AuditEvent
	AuditFilterEvent                                                                            string
	AuditCSVURL                                                                                 string
	AuditFilterResult                                                                           string
	AuditFilterActor                                                                            string
	AuditFilterQuery                                                                            string
	AuditPage                                                                                   int
	AuditTotal                                                                                  int
	AuditStart                                                                                  int
	AuditEnd                                                                                    int
	AuditPrevURL                                                                                string
	AuditNextURL                                                                                string
	AuditHasPrev                                                                                bool
	AuditHasNext                                                                                bool
	HasLinks                                                                                    bool
	UploadRequest                                                                               redisstore.UploadRequest
	Integration                                                                                 redisstore.IntegrationConfig
	Disk                                                                                        DiskInfo
	Analytics                                                                                   AdminAnalytics
	T                                                                                           func(string) string
	Languages                                                                                   []string
	TTLs                                                                                        []ttlOpt
}
type ttlOpt struct {
	Label   string
	Seconds int64
}
type DiskInfo struct{ Total, Available, Used, UsedPercent string }

type AdminAnalytics struct {
	ActiveTextLinks       int
	ActiveFileLinks       int
	ActiveUploadRequests  int
	ReceivedFilesReady    int
	RecentLinksCreated    int
	RecentLinksConsumed   int
	RecentUploadRequests  int
	RecentEmailDeliveries int
	RecentEmailFailures   int
	RecentBurns           int
	RecentFailedAccesses  int
	RecentAuditEvents     int
	StorageBytesActive    int64
}

func New(cfg config.Config, store *redisstore.Store, files *storage.Local, bundle *i18n.Bundle, log *slog.Logger) (*App, error) {
	t := template.New("").Funcs(template.FuncMap{
		"humanSize":   humanSize,
		"formatBytes": humanSize,
		"formatTime":  formatTime,
		"t": func(key string, page Page) string {
			if page.T == nil {
				return key
			}
			return page.T(key)
		},
	})
	t, err := t.ParseGlob("web/templates/*.html")
	if err != nil {
		return nil, err
	}
	trustedProxies, err := parseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		return nil, err
	}
	return &App{cfg: cfg, store: store, files: files, i18n: bundle, log: log, templates: t, trustedProxies: trustedProxies}, nil
}

func (a *App) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(a.trustedProxyMiddleware, middleware.RequestID, middleware.Recoverer, a.securityHeaders, ratelimit.New(a.cfg.RateLimitPerMinute).Middleware, a.csrfMiddleware)
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))).ServeHTTP(w, r)
	})
	r.Get("/brand/logo", a.logo)
	r.Get("/brand/favicon", a.favicon)
	r.Get("/favicon.ico", a.favicon)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	r.Get("/api/v1/audit", a.apiAudit)
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.store.Ping(r.Context()); err != nil {
			http.Error(w, "not ready", 503)
			return
		}
		w.WriteHeader(204)
	})
	r.Get("/privacy", a.privacy)
	r.Get("/terms", a.terms)
	r.Get("/setup", a.setup)
	r.Post("/setup", a.setupPost)
	r.Get("/login", a.login)
	r.Post("/login", a.loginPost)
	r.Post("/logout", a.logout)
	r.Get("/auth/microsoft/config", a.microsoftConfig)
	r.Post("/auth/microsoft", a.microsoftPost)
	r.Get("/auth/microsoft", a.microsoftLogin)
	r.Get("/auth/microsoft/callback", a.microsoftCallback)
	r.Get("/auth/ad", a.adLogin)
	r.Post("/auth/ad", a.adPost)
	r.Group(func(r chi.Router) {
		r.Use(a.requireAuth)
		r.Get("/", a.home)
		r.Post("/language", a.language)
		r.Post("/create", a.create)
		r.Post("/secrets/text", a.createText)
		r.Post("/secrets/file", a.createFile)
		r.Post("/upload-requests", a.createUploadRequest)
		r.Get("/upload-requests/{id}/status", a.uploadRequestStatus)
		r.Post("/upload-requests/{id}/download", a.downloadUploadRequestFile)
		r.Get("/created", a.created)
		r.Get("/admin", a.admin)
		r.Get("/admin/api-keys", a.adminAPIKeys)
		r.Post("/admin/api-keys", a.adminCreateAPIKey)
		r.Post("/admin/api-keys/{id}/revoke", a.adminRevokeAPIKey)
		r.Get("/admin/audit.csv", a.adminAuditCSV)
		r.Post("/admin/links/{id}/burn", a.adminBurnLink)
		r.Post("/admin/upload-requests/{id}/burn", a.adminBurnUploadRequest)
		r.Post("/admin/users", a.adminSaveUser)
		r.Post("/admin/users/{username}/delete", a.adminDeleteUser)
		r.Post("/admin/integrations", a.adminSaveIntegrations)
		r.Post("/admin/logo", a.adminUploadLogo)
	})
	r.Get("/s/{id}", a.viewText)
	r.Post("/s/{id}/reveal", a.revealText)
	r.Get("/f/{id}", a.viewFile)
	r.Post("/f/{id}/download", a.downloadFile)
	r.Get("/upload/{id}", a.viewUploadRequest)
	r.Post("/upload/{id}", a.submitUploadRequest)
	r.Get("/expired", a.expired)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		a.render(w, r, 404, "error.html", Page{Title: "404", Error: a.t(r, "not_found")})
	})
	return r
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, 200, "home.html", Page{Title: a.t(r, "app_name")})
}
func (a *App) expired(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, 410, "expired.html", Page{Title: a.t(r, "gone_title")})
}
func (a *App) privacy(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, 200, "privacy.html", Page{Title: a.t(r, "privacy_title")})
}
func (a *App) terms(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, 200, "terms.html", Page{Title: a.t(r, "terms_title")})
}
func (a *App) created(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(createdReceiptCookieName)
	if err != nil || cookie.Value == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	receipt, err := a.store.GetCreatedReceipt(r.Context(), cookie.Value)
	if err != nil || receipt.Link == "" {
		http.SetCookie(w, &http.Cookie{Name: createdReceiptCookieName, Value: "", Path: "/created", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	message := ""
	switch receipt.EmailStatus {
	case "sent":
		message = a.t(r, "created_email_sent")
	case "failed":
		message = a.t(r, "created_email_failed")
	}
	a.render(w, r, 200, "created.html", Page{Title: a.t(r, "created_title"), Link: receipt.Link, ID: displayCodeFromLink(receipt.Link), ExpiresAt: receipt.ExpiresAt, Size: receipt.TTLSeconds, Message: message})
}
func (a *App) language(w http.ResponseWriter, r *http.Request) {
	lang := a.i18n.Normalize(r.FormValue("language"))
	http.SetCookie(w, &http.Cookie{Name: "lang", Value: lang, Path: "/", MaxAge: 31536000, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	http.Redirect(w, r, referer(r), 303)
}

func (a *App) create(w http.ResponseWriter, r *http.Request) {
	if r.FormValue("secret_type") == "file" {
		a.createFile(w, r)
		return
	}
	a.createText(w, r)
}

func (a *App) createText(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.bad(w, r, err)
		return
	}
	secret := r.FormValue("secret")
	if secret == "" || int64(len(secret)) > a.cfg.MaxTextSecretSize {
		a.formError(w, r, "invalid_text_secret")
		return
	}
	item, err := a.newBaseItem(r, "text")
	if err != nil {
		a.bad(w, r, err)
		return
	}
	dataKey, _ := sec.NewDataKey()
	wrapped, err := sec.WrapKey(dataKey, a.cfg.EncryptionMasterKey)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	payload, err := sec.Encrypt([]byte(secret), dataKey)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	item.WrappedKeyNonce, item.WrappedKeyCiphertext = wrapped.Nonce, wrapped.Ciphertext
	item.PayloadNonce, item.PayloadCiphertext = payload.Nonce, payload.Ciphertext
	if err := a.store.Create(r.Context(), item, time.Until(time.Unix(item.ExpiresAt, 0))); err != nil {
		a.bad(w, r, err)
		return
	}
	link := a.publicBaseURL(r) + "/s/" + item.ID
	emailStatus, ok := a.sendCreatedLinkIfRequested(w, r, item, link, "text")
	if !ok {
		return
	}
	a.audit(r, "create_text_link", item.ID, "success", "")
	a.redirectCreated(w, r, "/s/"+item.ID, item, emailStatus)
}

func (a *App) createFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxFileSize+1024*1024)
	if err := r.ParseMultipartForm(512 * 1024); err != nil {
		a.formError(w, r, "invalid_file")
		return
	}
	fh, header, err := r.FormFile("file")
	if err != nil {
		a.formError(w, r, "invalid_file")
		return
	}
	defer fh.Close()
	if header.Size <= 0 || header.Size > a.cfg.MaxFileSize {
		a.formError(w, r, "invalid_file")
		return
	}
	prefix, err := io.ReadAll(io.LimitReader(fh, 512))
	if err != nil || len(prefix) == 0 {
		a.formError(w, r, "invalid_file")
		return
	}
	if !diskCanAccept(a.cfg.StoragePath, header.Size*2) {
		a.formError(w, r, "disk_space_limit")
		return
	}
	item, err := a.newBaseItem(r, "file")
	if err != nil {
		a.bad(w, r, err)
		return
	}
	dataKey, _ := sec.NewDataKey()
	wrapped, err := sec.WrapKey(dataKey, a.cfg.EncryptionMasterKey)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	p, err := a.files.WriteStream(item.ID, func(dst io.Writer) error {
		_, streamErr := sec.EncryptReader(io.MultiReader(bytes.NewReader(prefix), fh), dst, dataKey, sec.DefaultChunkSize)
		return streamErr
	})
	if err != nil {
		a.bad(w, r, err)
		return
	}
	name := storage.SanitizeFilename(header.Filename)
	item.WrappedKeyNonce, item.WrappedKeyCiphertext = wrapped.Nonce, wrapped.Ciphertext
	item.OriginalFilename = header.Filename
	item.SanitizedFilename = name
	item.FileSize = header.Size
	item.MimeType = http.DetectContentType(prefix)
	item.StorageObjectPath = p
	if err := a.store.Create(r.Context(), item, time.Until(time.Unix(item.ExpiresAt, 0))); err != nil {
		a.files.Delete(p)
		a.bad(w, r, err)
		return
	}
	link := a.publicBaseURL(r) + "/f/" + item.ID
	emailStatus, ok := a.sendCreatedLinkIfRequested(w, r, item, link, "file")
	if !ok {
		return
	}
	a.audit(r, "create_file_link", item.ID, "success", fmt.Sprintf("size=%d", item.FileSize))
	a.redirectCreated(w, r, "/f/"+item.ID, item, emailStatus)
}

func (a *App) createUploadRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login", 303)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.bad(w, r, err)
		return
	}
	delivery := r.FormValue("delivery")
	if delivery == "" {
		delivery = "email"
	}
	recipient := strings.TrimSpace(r.FormValue("recipient_email"))
	if delivery == "email" {
		if _, err := mail.ParseAddress(recipient); err != nil {
			a.formError(w, r, "invalid_email")
			return
		}
	}
	requesterEmail := strings.TrimSpace(user.Email)
	if requesterEmail == "" {
		requesterEmail = strings.TrimSpace(r.FormValue("requester_email"))
	}
	if requesterEmail != "" {
		if _, err := mail.ParseAddress(requesterEmail); err != nil {
			a.formError(w, r, "requester_email_required")
			return
		}
	}
	id, err := sec.Token()
	if err != nil {
		a.bad(w, r, err)
		return
	}
	now := time.Now()
	ttl := a.ttl(r)
	req := redisstore.UploadRequest{ID: id, Status: "available", CreatedAt: now.Unix(), ExpiresAt: now.Add(ttl).Unix(), RequestedBy: user.Username, RequesterEmail: requesterEmail, RecipientEmail: recipient, Message: strings.TrimSpace(r.FormValue("message"))}
	if err := a.store.CreateUploadRequest(r.Context(), req, ttl); err != nil {
		a.bad(w, r, err)
		return
	}
	link := a.publicBaseURL(r) + "/upload/" + id
	if delivery == "email" {
		if err := a.sendUploadRequestEmail(r.Context(), a.lang(r), recipient, link, displayUser(user), req.Message); err != nil {
			a.bad(w, r, err)
			return
		}
	}
	a.audit(r, "create_upload_request", id, "success", "delivery="+delivery)
	a.redirectCreated(w, r, "/upload/"+id, redisstore.Item{ID: id, CreatedAt: req.CreatedAt, ExpiresAt: req.ExpiresAt}, "")
}

func (a *App) viewUploadRequest(w http.ResponseWriter, r *http.Request) {
	req, err := a.availableUploadRequest(r)
	if err != nil {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	a.render(w, r, 200, "upload_request.html", Page{Title: a.t(r, "upload_request_title"), UploadRequest: req, ExpiresAt: formatTime(req.ExpiresAt)})
}

func (a *App) uploadRequestStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	req, err := a.store.GetUploadRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil || req.RequestedBy != user.Username {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	uploaded := false
	downloadURL := ""
	if req.UploadedItemID != "" {
		if item, err := a.store.Get(r.Context(), req.UploadedItemID); err == nil && item.Status == "available" && time.Now().Unix() <= item.ExpiresAt {
			uploaded = true
			downloadURL = "/upload-requests/" + req.ID + "/download"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"uploaded": uploaded, "download_url": downloadURL})
}

func (a *App) downloadUploadRequestFile(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	req, err := a.store.GetUploadRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil || req.RequestedBy != user.Username || req.UploadedItemID == "" {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	item, err := a.store.Get(r.Context(), req.UploadedItemID)
	if err != nil || item.Type != "file" || item.Status != "available" {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	claimed, err := a.store.Claim(r.Context(), item.ID)
	if err != nil {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	a.audit(r, "download_uploaded_file", item.ID, "success", "upload_request="+req.ID)
	a.serveClaimedFile(w, r, claimed)
}

func (a *App) submitUploadRequest(w http.ResponseWriter, r *http.Request) {
	req, err := a.availableUploadRequest(r)
	if err != nil {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxFileSize+1024*1024)
	if err := r.ParseMultipartForm(a.cfg.MaxFileSize); err != nil {
		a.render(w, r, 400, "upload_request.html", Page{Title: a.t(r, "upload_request_title"), UploadRequest: req, Error: a.t(r, "invalid_file")})
		return
	}
	fh, header, err := r.FormFile("file")
	if err != nil {
		a.render(w, r, 400, "upload_request.html", Page{Title: a.t(r, "upload_request_title"), UploadRequest: req, Error: a.t(r, "invalid_file")})
		return
	}
	defer fh.Close()
	plaintext, err := io.ReadAll(io.LimitReader(fh, a.cfg.MaxFileSize+1))
	if err != nil || int64(len(plaintext)) > a.cfg.MaxFileSize || len(plaintext) == 0 {
		a.render(w, r, 400, "upload_request.html", Page{Title: a.t(r, "upload_request_title"), UploadRequest: req, Error: a.t(r, "invalid_file")})
		return
	}
	if !diskCanAccept(a.cfg.StoragePath, int64(len(plaintext))*2) {
		a.render(w, r, 507, "upload_request.html", Page{Title: a.t(r, "upload_request_title"), UploadRequest: req, Error: a.t(r, "disk_space_limit")})
		return
	}
	req, err = a.store.ClaimUploadRequest(r.Context(), req.ID)
	if err != nil {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	item, err := a.newUploadedFileItem(r, req, header.Filename, plaintext)
	if err != nil {
		_ = a.store.ReleaseUploadRequest(r.Context(), req.ID)
		a.bad(w, r, err)
		return
	}
	if err := a.store.MarkUploadRequestUploaded(r.Context(), req.ID, item.ID); err != nil {
		_ = a.store.ReleaseUploadRequest(r.Context(), req.ID)
		_ = a.store.BurnItem(r.Context(), item.ID)
		a.files.Delete(item.StorageObjectPath)
		a.bad(w, r, err)
		return
	}
	if r.FormValue("notify_requester") == "on" && req.RequesterEmail != "" {
		link := a.publicBaseURL(r) + "/f/" + item.ID
		if err := a.sendUploadNotificationEmail(r.Context(), a.lang(r), req.RequesterEmail, link); err != nil {
			a.log.Warn("upload notification failed", "error", err)
		}
	}
	a.audit(r, "submit_upload_request", req.ID, "success", fmt.Sprintf("item=%s size=%d", item.ID, item.FileSize))
	a.render(w, r, 200, "upload_request_done.html", Page{Title: a.t(r, "upload_complete_title")})
}

func (a *App) viewText(w http.ResponseWriter, r *http.Request) {
	item, err := a.getAvailable(r)
	if err != nil || item.Type != "text" {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	a.render(w, r, 200, "view_text.html", Page{Title: a.t(r, "view_secret_title"), ID: item.ID, HasPassphrase: item.HasPassphrase, ExpiresAt: formatTime(item.ExpiresAt)})
}
func (a *App) revealText(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	item, err := a.store.Get(r.Context(), id)
	if err != nil || item.Type != "text" || item.Status != "available" {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	throttleID := id + "|" + clientAddress(r)
	limited, err := a.store.FailureLimitExceeded(r.Context(), "passphrase", throttleID, passphraseFailureLimit)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if limited {
		a.audit(r, "reveal_text_link", id, "blocked", "too many failed passphrase attempts")
		a.render(w, r, http.StatusTooManyRequests, "view_text.html", Page{Title: a.t(r, "view_secret_title"), ID: id, HasPassphrase: item.HasPassphrase, Error: a.t(r, "too_many_attempts")})
		return
	}
	if !sec.VerifyPassphrase(item.PassphraseHash, r.FormValue("passphrase")) {
		limited, limitErr := a.store.RegisterFailure(r.Context(), "passphrase", throttleID, passphraseFailureLimit, authFailureWindow)
		if limitErr != nil {
			a.bad(w, r, limitErr)
			return
		}
		status := http.StatusForbidden
		errorMessage := a.t(r, "bad_passphrase")
		if limited {
			status = http.StatusTooManyRequests
			errorMessage = a.t(r, "too_many_attempts")
		}
		a.render(w, r, status, "view_text.html", Page{Title: a.t(r, "view_secret_title"), ID: id, HasPassphrase: item.HasPassphrase, Error: errorMessage})
		return
	}
	_ = a.store.ResetFailures(r.Context(), "passphrase", throttleID)
	claimed, err := a.store.Claim(r.Context(), id)
	if err != nil {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	a.audit(r, "reveal_text_link", id, "success", "")
	key, err := sec.UnwrapKey(sec.WrappedKey{Nonce: claimed.WrappedKeyNonce, Ciphertext: claimed.WrappedKeyCiphertext}, a.cfg.EncryptionMasterKey)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	plain, err := sec.Decrypt(sec.EncryptedPayload{Nonce: claimed.PayloadNonce, Ciphertext: claimed.PayloadCiphertext}, key)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	a.render(w, r, 200, "secret_revealed.html", Page{Title: a.t(r, "revealed_title"), Secret: string(plain)})
}
func (a *App) viewFile(w http.ResponseWriter, r *http.Request) {
	item, err := a.getAvailable(r)
	if err != nil || item.Type != "file" {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	a.render(w, r, 200, "view_file.html", Page{Title: a.t(r, "download_title"), ID: item.ID, Filename: item.SanitizedFilename, Size: item.FileSize, Mime: item.MimeType, HasPassphrase: item.HasPassphrase, ExpiresAt: formatTime(item.ExpiresAt)})
}
func (a *App) downloadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	item, err := a.store.Get(r.Context(), id)
	if err != nil || item.Type != "file" || item.Status != "available" {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	throttleID := id + "|" + clientAddress(r)
	limited, err := a.store.FailureLimitExceeded(r.Context(), "passphrase", throttleID, passphraseFailureLimit)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if limited {
		a.audit(r, "download_file_link", id, "blocked", "too many failed passphrase attempts")
		a.render(w, r, http.StatusTooManyRequests, "view_file.html", Page{Title: a.t(r, "download_title"), ID: id, Filename: item.SanitizedFilename, Size: item.FileSize, Mime: item.MimeType, HasPassphrase: item.HasPassphrase, Error: a.t(r, "too_many_attempts")})
		return
	}
	if !sec.VerifyPassphrase(item.PassphraseHash, r.FormValue("passphrase")) {
		limited, limitErr := a.store.RegisterFailure(r.Context(), "passphrase", throttleID, passphraseFailureLimit, authFailureWindow)
		if limitErr != nil {
			a.bad(w, r, limitErr)
			return
		}
		status := http.StatusForbidden
		errorMessage := a.t(r, "bad_passphrase")
		if limited {
			status = http.StatusTooManyRequests
			errorMessage = a.t(r, "too_many_attempts")
		}
		a.render(w, r, status, "view_file.html", Page{Title: a.t(r, "download_title"), ID: id, Filename: item.SanitizedFilename, Size: item.FileSize, Mime: item.MimeType, HasPassphrase: item.HasPassphrase, Error: errorMessage})
		return
	}
	_ = a.store.ResetFailures(r.Context(), "passphrase", throttleID)
	claimed, err := a.store.Claim(r.Context(), id)
	if err != nil {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	a.audit(r, "download_file_link", id, "success", fmt.Sprintf("size=%d", item.FileSize))
	a.serveClaimedFile(w, r, claimed)
}

func (a *App) serveClaimedFile(w http.ResponseWriter, r *http.Request, claimed redisstore.Item) {
	defer a.files.Delete(claimed.StorageObjectPath)
	file, err := a.files.Open(claimed.StorageObjectPath)
	if err != nil {
		http.Redirect(w, r, "/expired", 303)
		return
	}
	defer file.Close()
	key, err := sec.UnwrapKey(sec.WrappedKey{Nonce: claimed.WrappedKeyNonce, Ciphertext: claimed.WrappedKeyCiphertext}, a.cfg.EncryptionMasterKey)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	reader := bufio.NewReader(file)
	w.Header().Set("Content-Type", safeMime(claimed.MimeType))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": claimed.SanitizedFilename}))
	w.Header().Set("Content-Length", strconv.FormatInt(claimed.FileSize, 10))
	if header, peekErr := reader.Peek(len(sec.ChunkedMagic)); peekErr == nil && bytes.Equal(header, sec.ChunkedMagic) {
		if _, err := sec.DecryptReader(reader, w, key); err != nil {
			a.log.Warn("streaming file decryption failed", "error", err)
		}
		return
	}
	enc, err := io.ReadAll(reader)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	plain, err := sec.Decrypt(sec.EncryptedPayload{Nonce: claimed.PayloadNonce, Ciphertext: string(enc)}, key)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	_, _ = w.Write(plain)
}

func (a *App) newUploadedFileItem(r *http.Request, req redisstore.UploadRequest, filename string, plaintext []byte) (redisstore.Item, error) {
	item, err := a.newBaseItem(r, "file")
	if err != nil {
		return redisstore.Item{}, err
	}
	item.CreatedBy = req.RequestedBy
	item.Direction = "receive"
	dataKey, _ := sec.NewDataKey()
	wrapped, err := sec.WrapKey(dataKey, a.cfg.EncryptionMasterKey)
	if err != nil {
		return redisstore.Item{}, err
	}
	payload, err := sec.Encrypt(plaintext, dataKey)
	if err != nil {
		return redisstore.Item{}, err
	}
	p, err := a.files.Write(item.ID, []byte(payload.Ciphertext))
	if err != nil {
		return redisstore.Item{}, err
	}
	item.WrappedKeyNonce, item.WrappedKeyCiphertext = wrapped.Nonce, wrapped.Ciphertext
	item.PayloadNonce = payload.Nonce
	item.OriginalFilename = filename
	item.SanitizedFilename = storage.SanitizeFilename(filename)
	item.FileSize = int64(len(plaintext))
	item.MimeType = http.DetectContentType(plaintext[:min(len(plaintext), 512)])
	item.StorageObjectPath = p
	if err := a.store.Create(r.Context(), item, time.Until(time.Unix(item.ExpiresAt, 0))); err != nil {
		a.files.Delete(p)
		return redisstore.Item{}, err
	}
	return item, nil
}
func (a *App) newBaseItem(r *http.Request, typ string) (redisstore.Item, error) {
	id, err := sec.Token()
	if err != nil {
		return redisstore.Item{}, err
	}
	ttl := a.ttl(r)
	now := time.Now()
	ph, err := sec.HashPassphrase(r.FormValue("passphrase"))
	if err != nil {
		return redisstore.Item{}, err
	}
	createdBy := "anonymous"
	if user, ok := currentUser(r); ok {
		createdBy = user.Username
	}
	return redisstore.Item{ID: id, Type: typ, Status: "available", CreatedAt: now.Unix(), ExpiresAt: now.Add(ttl).Unix(), CreatedBy: createdBy, Direction: "send", HasPassphrase: ph != "", PassphraseHash: ph}, nil
}
func (a *App) ttl(r *http.Request) time.Duration {
	raw := r.FormValue("ttl")
	if raw == "" {
		raw = r.FormValue("ttl_seconds")
	}
	n, _ := strconv.ParseInt(raw, 10, 64)
	d := time.Duration(n) * time.Second
	if d <= 0 {
		d = a.cfg.DefaultTTL
	}
	if d > a.cfg.MaxTTL {
		d = a.cfg.MaxTTL
	}
	return d
}
func (a *App) getAvailable(r *http.Request) (redisstore.Item, error) {
	item, err := a.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil || item.Status != "available" || time.Now().Unix() > item.ExpiresAt {
		return item, redisstore.ErrGone
	}
	return item, nil
}
func (a *App) availableUploadRequest(r *http.Request) (redisstore.UploadRequest, error) {
	req, err := a.store.GetUploadRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil || req.Status != "available" || time.Now().Unix() > req.ExpiresAt {
		return req, redisstore.ErrGone
	}
	return req, nil
}
func (a *App) sendCreatedLinkIfRequested(w http.ResponseWriter, r *http.Request, item redisstore.Item, link, kind string) (string, bool) {
	recipient := strings.TrimSpace(r.FormValue("recipient_email"))
	if recipient == "" {
		return "", true
	}
	if _, err := mail.ParseAddress(recipient); err != nil {
		a.formError(w, r, "invalid_email")
		return "", false
	}
	creator := item.CreatedBy
	if user, ok := currentUser(r); ok {
		creator = displayUser(user)
	}
	if err := a.sendCreatedLinkEmail(r.Context(), a.lang(r), recipient, link, creator, kind); err != nil {
		a.audit(r, "send_created_link_email", item.ID, "failure", kind)
		a.log.Warn("created link email failed", "error", err, "item", item.ID, "kind", kind)
		return "failed", true
	}
	a.audit(r, "send_created_link_email", item.ID, "success", kind)
	return "sent", true
}

func (a *App) publicBaseURL(r *http.Request) string {
	fallback := strings.TrimRight(a.cfg.AppBaseURL, "/")
	parsed, err := url.Parse(fallback)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || r == nil {
		return fallback
	}
	requestHost := strings.ToLower(strings.TrimSpace(r.Host))
	if host, _, splitErr := net.SplitHostPort(requestHost); splitErr == nil {
		requestHost = host
	}
	requestHost = strings.TrimSuffix(requestHost, ".")
	for _, allowed := range a.cfg.CustomDomains {
		allowed = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(allowed), "."))
		if allowed == "" || allowed != requestHost {
			continue
		}
		parsed.Host = r.Host
		if a.cfg.SecureCookies || r.TLS != nil {
			parsed.Scheme = "https"
		}
		return strings.TrimRight(parsed.String(), "/")
	}
	return fallback
}

func (a *App) redirectCreated(w http.ResponseWriter, r *http.Request, path string, item redisstore.Item, emailStatus string) {
	link := a.publicBaseURL(r) + path
	token, err := sec.Token()
	if err != nil {
		a.bad(w, r, err)
		return
	}
	receipt := redisstore.CreatedReceipt{Token: token, Link: link, ExpiresAt: time.Unix(item.ExpiresAt, 0).Format(time.RFC1123), TTLSeconds: item.ExpiresAt - item.CreatedAt, EmailStatus: emailStatus}
	if err := a.store.SaveCreatedReceipt(r.Context(), receipt, createdReceiptTTL); err != nil {
		a.bad(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: createdReceiptCookieName, Value: token, Path: "/created", MaxAge: int(createdReceiptTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	http.Redirect(w, r, "/created", 303)
}
func (a *App) render(w http.ResponseWriter, r *http.Request, status int, name string, p Page) {
	lang := a.lang(r)
	p.Lang = lang
	p.CSRF = a.csrf(w, r)
	if p.User == nil {
		if user, ok := currentUser(r); ok {
			p.User = user
		}
	}
	p.T = func(k string) string { return a.i18n.T(lang, k) }
	p.Version = appVersion
	p.Languages = a.cfg.AllowedLanguages
	p.TTLs = []ttlOpt{{"1 minute", 60}, {"5 minutes", 300}, {"30 minutes", 1800}, {"1 hour", 3600}, {"4 hours", 14400}, {"12 hours", 43200}, {"1 day", 86400}, {"3 days", 259200}, {"7 days", 604800}, {"14 days", 1209600}, {"30 days", 2592000}}
	if _, err := os.Stat(a.logoPath()); err == nil {
		p.LogoURL = "/brand/logo"
	}
	if cfg, err := a.getIntegrationConfig(r.Context()); err == nil {
		p.EmailConfigured = cfg.SMTPEnabled || cfg.GraphEnabled
		p.MicrosoftLoginEnabled = cfg.MicrosoftEnabled && cfg.MicrosoftTenantID != "" && cfg.MicrosoftClientID != "" && cfg.MicrosoftAudience != ""
		p.ADLoginEnabled = cfg.ADEnabled && cfg.ADHost != "" && cfg.ADBaseDN != ""
	}
	w.WriteHeader(status)
	if err := a.templates.ExecuteTemplate(w, name, p); err != nil {
		a.log.Error("render failed", "error", err)
	}
}
func (a *App) lang(r *http.Request) string {
	if c, err := r.Cookie("lang"); err == nil {
		return a.i18n.Normalize(c.Value)
	}
	return a.i18n.Match(r.Header.Get("Accept-Language"))
}
func (a *App) t(r *http.Request, k string) string { return a.i18n.T(a.lang(r), k) }
func (a *App) bad(w http.ResponseWriter, r *http.Request, err error) {
	a.log.Error("request failed", "error", err)
	a.render(w, r, 500, "error.html", Page{Title: "500", Error: a.t(r, "server_error")})
}
func (a *App) formError(w http.ResponseWriter, r *http.Request, key string) {
	a.render(w, r, 400, "home.html", Page{Title: a.t(r, "app_name"), Error: a.t(r, key)})
}
func displayUser(user *redisstore.User) string {
	if user.FirstName != "" || user.LastName != "" {
		return strings.TrimSpace(user.FirstName + " " + user.LastName)
	}
	return user.Username
}
func (a *App) logoPath() string                            { return filepath.Join(a.cfg.StoragePath, "brand-logo") }
func (a *App) logo(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, a.logoPath()) }
func (a *App) favicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	if _, err := os.Stat(a.logoPath()); err == nil {
		http.ServeFile(w, r, a.logoPath())
		return
	}
	http.NotFound(w, r)
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://alcdn.msauth.net https://cdn.jsdelivr.net; connect-src 'self' https://login.microsoftonline.com; style-src 'self' 'unsafe-inline'; font-src 'self'; object-src 'none'; base-uri 'none'")
		if a.cfg.SecureCookies {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if !strings.HasPrefix(r.URL.Path, "/static/") && !strings.HasPrefix(r.URL.Path, "/brand/") && r.URL.Path != "/favicon.ico" {
			h.Set("Cache-Control", "no-store, max-age=0")
			h.Set("Pragma", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, a.postBodyLimit(r))
			if err := parseCSRFForm(r); err != nil {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			if r.Form.Get("csrf") == "" || r.Form.Get("csrf") != csrfCookie(r) {
				http.Error(w, "invalid csrf token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) postBodyLimit(r *http.Request) int64 {
	path := r.URL.Path
	if path == "/admin/logo" {
		return maxLogoBodySize
	}
	if path == "/create" || path == "/secrets/file" || strings.HasPrefix(path, "/upload/") {
		return a.cfg.MaxFileSize + maxFormBodySize
	}
	return maxFormBodySize
}

func parseCSRFForm(r *http.Request) error {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/") {
		return r.ParseMultipartForm(maxFormBodySize)
	}
	return r.ParseForm()
}
func (a *App) csrf(w http.ResponseWriter, r *http.Request) string {
	if v := csrfCookie(r); v != "" {
		return v
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	v := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: "csrf", Value: v, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	return v
}
func csrfCookie(r *http.Request) string {
	c, err := r.Cookie("csrf")
	if err != nil {
		return ""
	}
	return c.Value
}

func parseTrustedProxies(values []string) ([]*net.IPNet, error) {
	proxies := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(value, "/") {
			_, network, err := net.ParseCIDR(value)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy %q", value)
			}
			proxies = append(proxies, network)
			continue
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy %q", value)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		proxies = append(proxies, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return proxies, nil
}

func (a *App) trustedProxyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(a.trustedProxies) == 0 || !a.remoteAddrIsTrusted(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		if ip := firstForwardedIP(r.Header.Get("X-Forwarded-For")); ip != nil {
			r.RemoteAddr = net.JoinHostPort(ip.String(), "0")
		} else if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			r.RemoteAddr = net.JoinHostPort(ip.String(), "0")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) remoteAddrIsTrusted(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, proxy := range a.trustedProxies {
		if proxy.Contains(ip) {
			return true
		}
	}
	return false
}

func firstForwardedIP(header string) net.IP {
	for _, part := range strings.Split(header, ",") {
		ip := net.ParseIP(strings.TrimSpace(part))
		if ip != nil {
			return ip
		}
	}
	return nil
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (a *App) getIntegrationConfig(ctx context.Context) (redisstore.IntegrationConfig, error) {
	cfg, err := a.store.GetIntegrationConfig(ctx)
	if err != nil {
		return cfg, err
	}
	if cfg.SMTPPassword, err = a.decryptIntegrationSecret(cfg.SMTPPassword); err != nil {
		return cfg, err
	}
	if cfg.GraphClientSecret, err = a.decryptIntegrationSecret(cfg.GraphClientSecret); err != nil {
		return cfg, err
	}
	if cfg.ADBindPassword, err = a.decryptIntegrationSecret(cfg.ADBindPassword); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (a *App) saveIntegrationConfig(ctx context.Context, cfg redisstore.IntegrationConfig) error {
	var err error
	if cfg.SMTPPassword, err = a.encryptIntegrationSecret(cfg.SMTPPassword); err != nil {
		return err
	}
	if cfg.GraphClientSecret, err = a.encryptIntegrationSecret(cfg.GraphClientSecret); err != nil {
		return err
	}
	if cfg.ADBindPassword, err = a.encryptIntegrationSecret(cfg.ADBindPassword); err != nil {
		return err
	}
	return a.store.SaveIntegrationConfig(ctx, cfg)
}

func (a *App) encryptIntegrationSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, encryptedSecretPrefix) {
		return value, nil
	}
	payload, err := sec.Encrypt([]byte(value), a.cfg.EncryptionMasterKey)
	if err != nil {
		return "", err
	}
	return encryptedSecretPrefix + payload.Nonce + ":" + payload.Ciphertext, nil
}

func (a *App) decryptIntegrationSecret(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, encryptedSecretPrefix) {
		return value, nil
	}
	parts := strings.SplitN(strings.TrimPrefix(value, encryptedSecretPrefix), ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid encrypted integration secret")
	}
	plain, err := sec.Decrypt(sec.EncryptedPayload{Nonce: parts[0], Ciphertext: parts[1]}, a.cfg.EncryptionMasterKey)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (a *App) audit(r *http.Request, event, target, result, details string) {
	actor := "anonymous"
	if user, ok := currentUser(r); ok && user.Username != "" {
		actor = user.Username
	}
	a.auditAs(r, actor, event, target, result, details)
}

func (a *App) auditAs(r *http.Request, actor, event, target, result, details string) {
	if actor == "" {
		actor = "anonymous"
	}
	if len(details) > 240 {
		details = details[:240]
	}
	if err := a.store.AddAuditEvent(r.Context(), redisstore.AuditEvent{Actor: actor, IP: r.RemoteAddr, Event: event, Target: target, Result: result, Details: details}); err != nil {
		a.log.Warn("audit event write failed", "error", err, "event", event)
	}
}
func referer(r *http.Request) string {
	if v := r.Header.Get("Referer"); v != "" {
		return v
	}
	return "/"
}
func formatTime(ts int64) string { return time.Unix(ts, 0).Format(time.RFC1123) }
func humanSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
}
func safeMime(v string) string {
	if v == "" {
		return "application/octet-stream"
	}
	return v
}
func displayCodeFromLink(link string) string {
	parts := strings.Split(strings.TrimRight(link, "/"), "/")
	if len(parts) == 0 {
		return "LINK"
	}
	tail := parts[len(parts)-1]
	if len(tail) > 4 {
		tail = tail[:4]
	}
	return strings.ToUpper(tail)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = context.Background
var _ = errors.New
var _ = filepath.Base
