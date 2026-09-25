package web

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-ldap/ldap/v3"

	sec "ephemeral-link/internal/crypto"
	"ephemeral-link/internal/redisstore"
)

type userContextKey struct{}

const sessionCookieName = "ephemeral_session"
const sessionTTL = 8 * time.Hour
const loginFailureLimit int64 = 5
const passphraseFailureLimit int64 = 5
const authFailureWindow = 15 * time.Minute

func (a *App) setupRequired(ctx context.Context) (bool, error) {
	hasAdmin, err := a.store.HasAdmin(ctx)
	if err != nil {
		return false, err
	}
	return !hasAdmin, nil
}

func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	required, err := a.setupRequired(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if !required {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	loginNoStore(w)
	a.render(w, r, http.StatusOK, "setup.html", Page{Title: a.t(r, "setup_title")})
}

func (a *App) setupPost(w http.ResponseWriter, r *http.Request) {
	required, err := a.setupRequired(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if !required {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	firstName := strings.TrimSpace(r.FormValue("first_name"))
	lastName := strings.TrimSpace(r.FormValue("last_name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")
	if username == "" || password == "" || len(password) < 12 || password != confirm {
		loginNoStore(w)
		a.render(w, r, http.StatusBadRequest, "setup.html", Page{Title: a.t(r, "setup_title"), Error: a.t(r, "setup_invalid")})
		return
	}
	hash, err := sec.HashPassphrase(password)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	user := redisstore.User{Username: username, PasswordHash: hash, Role: "administrator", FirstName: firstName, LastName: lastName, Email: email, CreatedAt: time.Now().Unix()}
	if err := a.store.CreateInitialAdmin(r.Context(), user); err != nil {
		loginNoStore(w)
		a.render(w, r, http.StatusConflict, "setup.html", Page{Title: a.t(r, "setup_title"), Error: a.t(r, "setup_unavailable")})
		return
	}
	token, err := sec.Token()
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if err := a.store.CreateSession(r.Context(), token, username, sessionTTL); err != nil {
		a.bad(w, r, err)
		return
	}
	a.auditAs(r, username, "initial_admin_setup", username, "success", "")
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if required, err := a.setupRequired(r.Context()); err != nil {
		a.bad(w, r, err)
		return
	} else if required {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if _, ok := a.authenticatedUser(r); ok {
		http.Redirect(w, r, "/", 303)
		return
	}
	loginNoStore(w)
	a.render(w, r, 200, "login.html", Page{Title: a.t(r, "sign_in")})
}

func (a *App) loginPost(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	throttleID := strings.ToLower(username) + "|" + clientAddress(r)
	limited, err := a.store.FailureLimitExceeded(r.Context(), "login", throttleID, loginFailureLimit)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if limited {
		a.audit(r, "local_login", username, "blocked", "too many failed attempts")
		loginNoStore(w)
		a.render(w, r, http.StatusTooManyRequests, "login.html", Page{Title: a.t(r, "sign_in"), Error: a.t(r, "too_many_attempts")})
		return
	}
	user, err := a.store.GetUser(r.Context(), username)
	if err != nil || user.PasswordHash == "" || !sec.VerifyPassphrase(user.PasswordHash, password) {
		limited, limitErr := a.store.RegisterFailure(r.Context(), "login", throttleID, loginFailureLimit, authFailureWindow)
		if limitErr != nil {
			a.bad(w, r, limitErr)
			return
		}
		a.audit(r, "local_login", username, "failed", "invalid credentials")
		loginNoStore(w)
		status := http.StatusUnauthorized
		errorMessage := a.t(r, "invalid_login")
		if limited {
			status = http.StatusTooManyRequests
			errorMessage = a.t(r, "too_many_attempts")
		}
		a.render(w, r, status, "login.html", Page{Title: a.t(r, "sign_in"), Error: errorMessage})
		return
	}
	token, err := sec.Token()
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if err := a.store.CreateSession(r.Context(), token, user.Username, sessionTTL); err != nil {
		a.bad(w, r, err)
		return
	}
	_ = a.store.ResetFailures(r.Context(), "login", throttleID)
	a.audit(r, "local_login", user.Username, "success", "")
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	http.Redirect(w, r, "/", 303)
}

func loginNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if user, ok := a.authenticatedUser(r); ok {
		a.auditAs(r, user.Username, "logout", user.Username, "success", "")
	}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = a.store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	http.Redirect(w, r, "/login", 303)
}

func (a *App) microsoftConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.getIntegrationConfig(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled":      cfg.MicrosoftEnabled,
		"tenantId":     cfg.MicrosoftTenantID,
		"clientId":     cfg.MicrosoftClientID,
		"apiClientId":  cfg.MicrosoftAudience,
		"authorityUrl": microsoftAuthority(cfg),
		"scope":        microsoftAccessScope(cfg),
	})
}

func (a *App) microsoftPost(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.getIntegrationConfig(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if !cfg.MicrosoftEnabled || cfg.MicrosoftTenantID == "" || cfg.MicrosoftClientID == "" || cfg.MicrosoftAudience == "" {
		http.Error(w, a.t(r, "microsoft_login_not_configured"), http.StatusNotImplemented)
		return
	}
	claims, err := validateMicrosoftAccessToken(r.Context(), cfg, r.FormValue("token"))
	if err != nil {
		a.audit(r, "microsoft_login", "microsoft", "failed", "token validation failed")
		http.Error(w, a.t(r, "microsoft_token_validation_failed"), http.StatusUnauthorized)
		return
	}
	objectID := strings.TrimSpace(claims.ObjectID)
	if objectID == "" {
		a.audit(r, "microsoft_login", "microsoft", "failed", "missing oid claim")
		http.Error(w, a.t(r, "microsoft_token_validation_failed"), http.StatusUnauthorized)
		return
	}
	username := "entra:" + objectID
	email := strings.TrimSpace(claims.Email)
	if email == "" {
		email = strings.TrimSpace(claims.PreferredUsername)
	}
	if email == "" {
		email = strings.TrimSpace(claims.UPN)
	}
	first, last := splitName(claims.Name)
	existing, _ := a.store.GetUser(r.Context(), username)
	createdAt := existing.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	role, allowed := microsoftRoleFromGroups(cfg, claims.Groups, existing.Role)
	if !allowed {
		a.auditAs(r, username, "microsoft_login", username, "denied", "no mapped Entra group")
		http.Error(w, a.t(r, "microsoft_group_access_denied"), http.StatusForbidden)
		return
	}
	if err := a.store.SaveUser(r.Context(), redisstore.User{Username: username, PasswordHash: "", Role: role, FirstName: first, LastName: last, Email: email, AuthProvider: "microsoft", ExternalID: objectID, CreatedAt: createdAt}); err != nil {
		a.bad(w, r, err)
		return
	}
	session, err := sec.Token()
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if err := a.store.CreateSession(r.Context(), session, username, sessionTTL); err != nil {
		a.bad(w, r, err)
		return
	}
	a.auditAs(r, username, "microsoft_login", username, "success", "")
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: session, Path: "/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *App) microsoftLogin(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, 400, "login.html", Page{Title: a.t(r, "sign_in"), Error: a.t(r, "microsoft_browser_flow_required")})
}

func (a *App) microsoftCallback(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", 303)
}

func (a *App) adLogin(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.getIntegrationConfig(r.Context())
	if err != nil || !cfg.ADEnabled || cfg.ADHost == "" || cfg.ADBaseDN == "" {
		a.render(w, r, http.StatusNotImplemented, "login.html", Page{Title: a.t(r, "sign_in"), Error: a.t(r, "ad_login_not_configured")})
		return
	}
	a.render(w, r, http.StatusOK, "login.html", Page{Title: a.t(r, "sign_in"), Error: a.t(r, "ad_login_prompt")})
}

func (a *App) adPost(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	throttleID := strings.ToLower(username) + "|" + clientAddress(r)
	limited, err := a.store.FailureLimitExceeded(r.Context(), "ad_login", throttleID, loginFailureLimit)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if limited {
		a.render(w, r, http.StatusTooManyRequests, "login.html", Page{Title: a.t(r, "sign_in"), Error: a.t(r, "too_many_attempts")})
		return
	}
	user, err := a.authenticateAD(username, password)
	if err != nil {
		limited, registerErr := a.store.RegisterFailure(r.Context(), "ad_login", throttleID, loginFailureLimit, authFailureWindow)
		if registerErr != nil {
			a.bad(w, r, registerErr)
			return
		}
		a.audit(r, "ad_login", username, "failed", "invalid directory credentials")
		status := http.StatusUnauthorized
		message := a.t(r, "invalid_login")
		if limited {
			status = http.StatusTooManyRequests
			message = a.t(r, "too_many_attempts")
		}
		a.render(w, r, status, "login.html", Page{Title: a.t(r, "sign_in"), Error: message})
		return
	}
	_ = a.store.ResetFailures(r.Context(), "ad_login", throttleID)
	token, err := sec.Token()
	if err != nil {
		a.bad(w, r, err)
		return
	}
	if err := a.store.CreateSession(r.Context(), token, user.Username, sessionTTL); err != nil {
		a.bad(w, r, err)
		return
	}
	a.auditAs(r, user.Username, "ad_login", user.Username, "success", "")
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: a.cfg.SecureCookies})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) authenticateAD(username, password string) (redisstore.User, error) {
	if username == "" || password == "" {
		return redisstore.User{}, fmt.Errorf("invalid directory credentials")
	}
	cfg, err := a.getIntegrationConfig(context.Background())
	if err != nil || !cfg.ADEnabled || cfg.ADHost == "" || cfg.ADBaseDN == "" {
		return redisstore.User{}, fmt.Errorf("directory authentication unavailable")
	}
	endpoint := strings.TrimSpace(cfg.ADHost)
	if !strings.Contains(endpoint, "://") {
		endpoint = "ldaps://" + endpoint
	}
	options := []ldap.DialOpt{}
	if strings.HasPrefix(strings.ToLower(endpoint), "ldaps://") {
		options = append(options, ldap.DialWithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}))
	}
	conn, err := ldap.DialURL(endpoint, options...)
	if err != nil {
		return redisstore.User{}, fmt.Errorf("directory unavailable")
	}
	defer conn.Close()
	if cfg.ADBindDN != "" {
		if err := conn.Bind(cfg.ADBindDN, cfg.ADBindPassword); err != nil {
			return redisstore.User{}, fmt.Errorf("directory unavailable")
		}
	}
	filter := "(&(objectClass=user)(sAMAccountName=" + ldap.EscapeFilter(username) + "))"
	request := ldap.NewSearchRequest(cfg.ADBaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 5, false, filter, []string{"mail", "displayName", "givenName", "sn"}, nil)
	result, err := conn.Search(request)
	if err != nil || len(result.Entries) != 1 {
		return redisstore.User{}, fmt.Errorf("invalid directory credentials")
	}
	entry := result.Entries[0]
	if err := conn.Bind(entry.DN, password); err != nil {
		return redisstore.User{}, fmt.Errorf("invalid directory credentials")
	}
	first, last := splitName(entry.GetAttributeValue("displayName"))
	if first == "" {
		first = entry.GetAttributeValue("givenName")
	}
	if last == "" {
		last = entry.GetAttributeValue("sn")
	}
	return redisstore.User{Username: "ad:" + strings.ToLower(username), Role: "user", FirstName: first, LastName: last, Email: entry.GetAttributeValue("mail"), AuthProvider: "ad", ExternalID: entry.DN, CreatedAt: time.Now().Unix()}, nil
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if required, err := a.setupRequired(r.Context()); err != nil {
			a.bad(w, r, err)
			return
		} else if required {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		user, ok := a.authenticatedUser(r)
		if !ok {
			http.Redirect(w, r, "/login", 303)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (*redisstore.User, bool) {
	user, ok := currentUser(r)
	if !ok || user.Role != "administrator" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return user, true
}

func (a *App) authenticatedUser(r *http.Request) (*redisstore.User, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return nil, false
	}
	username, err := a.store.GetSession(r.Context(), c.Value)
	if err != nil {
		return nil, false
	}
	user, err := a.store.GetUser(r.Context(), username)
	if err != nil {
		return nil, false
	}
	return &user, true
}

func currentUser(r *http.Request) (*redisstore.User, bool) {
	user, ok := r.Context().Value(userContextKey{}).(*redisstore.User)
	return user, ok
}

func (a *App) admin(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	items, err := a.store.ListAvailableItems(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	uploadRequests, err := a.store.ListAvailableUploadRequests(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	integration, err := a.getIntegrationConfig(r.Context())
	if err != nil {
		a.bad(w, r, err)
		return
	}
	allAuditEvents, err := a.store.ListAuditEvents(r.Context(), 500)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	auditPage := buildAuditPage(r, allAuditEvents)
	analytics := buildAdminAnalytics(items, uploadRequests, allAuditEvents)
	disk := diskInfo(a.cfg.StoragePath)
	message := adminSavedMessage(a.t(r, "settings_saved_"+r.URL.Query().Get("saved")))
	a.render(w, r, 200, "admin.html", Page{Title: "Admin", User: user, Items: items, UploadRequests: uploadRequests, AuditEvents: auditPage.Events, AuditFilterEvent: auditPage.Event, AuditCSVURL: auditCSVURL(r.URL.Query()), AuditFilterResult: auditPage.Result, AuditFilterActor: auditPage.Actor, AuditFilterQuery: auditPage.Query, AuditPage: auditPage.Page, AuditTotal: auditPage.Total, AuditStart: auditPage.Start, AuditEnd: auditPage.End, AuditPrevURL: auditPage.PrevURL, AuditNextURL: auditPage.NextURL, AuditHasPrev: auditPage.HasPrev, AuditHasNext: auditPage.HasNext, HasLinks: len(items)+len(uploadRequests) > 0, Users: users, Integration: integration, Disk: disk, Analytics: analytics, Message: message})
}

type auditPageData struct {
	Events   []redisstore.AuditEvent
	Filtered []redisstore.AuditEvent
	Event    string
	Result   string
	Actor    string
	Query    string
	Page     int
	Total    int
	Start    int
	End      int
	PrevURL  string
	NextURL  string
	HasPrev  bool
	HasNext  bool
}

const auditPageSize = 50

func buildAuditPage(r *http.Request, events []redisstore.AuditEvent) auditPageData {
	filters := auditPageData{Event: strings.TrimSpace(r.URL.Query().Get("audit_event")), Result: strings.TrimSpace(r.URL.Query().Get("audit_result")), Actor: strings.TrimSpace(r.URL.Query().Get("audit_actor")), Query: strings.TrimSpace(r.URL.Query().Get("audit_q")), Page: 1}
	if page, err := strconv.Atoi(r.URL.Query().Get("audit_page")); err == nil && page > 0 {
		filters.Page = page
	}
	filtered := make([]redisstore.AuditEvent, 0, len(events))
	query := strings.ToLower(filters.Query)
	actor := strings.ToLower(filters.Actor)
	for _, event := range events {
		if filters.Event != "" && event.Event != filters.Event {
			continue
		}
		if filters.Result != "" && event.Result != filters.Result {
			continue
		}
		if actor != "" && !strings.Contains(strings.ToLower(event.Actor), actor) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{event.Event, event.Target, event.Result, event.Details, event.IP, event.Actor}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, event)
	}
	filters.Filtered = filtered
	filters.Total = len(filtered)
	pages := (filters.Total + auditPageSize - 1) / auditPageSize
	if pages == 0 {
		pages = 1
	}
	if filters.Page > pages {
		filters.Page = pages
	}
	start := (filters.Page - 1) * auditPageSize
	end := start + auditPageSize
	if end > filters.Total {
		end = filters.Total
	}
	if filters.Total > 0 {
		filters.Start = start + 1
		filters.End = end
		filters.Events = filtered[start:end]
	}
	filters.HasPrev = filters.Page > 1
	filters.HasNext = filters.Page < pages
	if filters.HasPrev {
		filters.PrevURL = auditPageURL(r.URL.Query(), filters.Page-1)
	}
	if filters.HasNext {
		filters.NextURL = auditPageURL(r.URL.Query(), filters.Page+1)
	}
	return filters
}

func auditPageURL(values url.Values, page int) string {
	copy := auditQuery(values)
	copy.Set("audit_page", strconv.Itoa(page))
	return "/admin?" + copy.Encode() + "#audit-logs"
}

func auditCSVURL(values url.Values) string {
	query := auditQuery(values)
	if len(query) == 0 {
		return "/admin/audit.csv"
	}
	return "/admin/audit.csv?" + query.Encode()
}

func auditQuery(values url.Values) url.Values {
	copy := url.Values{}
	for _, key := range []string{"audit_event", "audit_result", "audit_actor", "audit_q"} {
		for _, val := range values[key] {
			copy.Add(key, val)
		}
	}
	return copy
}

func (a *App) adminAuditCSV(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	events, err := a.store.ListAuditEvents(r.Context(), 500)
	if err != nil {
		a.bad(w, r, err)
		return
	}
	filtered := buildAuditPage(r, events).Filtered
	filename := "audit-logs-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"time", "actor", "event", "target", "result", "details", "ip"})
	for _, event := range filtered {
		_ = writer.Write([]string{
			time.Unix(event.CreatedAt, 0).UTC().Format(time.RFC3339),
			csvSafeCell(event.Actor),
			csvSafeCell(event.Event),
			csvSafeCell(event.Target),
			csvSafeCell(event.Result),
			csvSafeCell(event.Details),
			csvSafeCell(event.IP),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		a.log.Error("audit csv export failed", "error", err)
	}
}

func csvSafeCell(value string) string {
	trimmed := strings.TrimLeft(value, "\t\r\n ")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func buildAdminAnalytics(items []redisstore.Item, uploadRequests []redisstore.UploadRequest, auditEvents []redisstore.AuditEvent) AdminAnalytics {
	var stats AdminAnalytics
	stats.ActiveUploadRequests = len(uploadRequests)
	stats.RecentAuditEvents = len(auditEvents)
	for _, item := range items {
		switch item.Type {
		case "text":
			stats.ActiveTextLinks++
		case "file":
			stats.ActiveFileLinks++
		}
		if item.Direction == "receive" {
			stats.ReceivedFilesReady++
		}
		stats.StorageBytesActive += item.FileSize
	}
	for _, event := range auditEvents {
		switch event.Event {
		case "create_text_link", "create_file_link":
			if event.Result == "success" {
				stats.RecentLinksCreated++
			}
		case "reveal_text_link", "download_file_link", "download_uploaded_file":
			if event.Result == "success" {
				stats.RecentLinksConsumed++
			}
		case "create_upload_request":
			if event.Result == "success" {
				stats.RecentUploadRequests++
			}
		case "send_created_link_email":
			if event.Result == "success" {
				stats.RecentEmailDeliveries++
			} else if event.Result == "failure" {
				stats.RecentEmailFailures++
			}
		case "admin_burn_link", "admin_burn_upload_request":
			if event.Result == "success" {
				stats.RecentBurns++
			}
		}
		if event.Result == "failure" {
			stats.RecentFailedAccesses++
		}
	}
	return stats
}

func (a *App) adminBurnLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	id := chi.URLParam(r, "id")
	item, _ := a.store.Get(r.Context(), id)
	if err := a.store.BurnItem(r.Context(), id); err != nil {
		a.bad(w, r, err)
		return
	}
	if item.Type == "file" && item.StorageObjectPath != "" {
		a.files.Delete(item.StorageObjectPath)
	}
	a.audit(r, "admin_burn_link", id, "success", item.Type)
	http.Redirect(w, r, "/admin#existing-links", 303)
}

func (a *App) adminBurnUploadRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := a.store.BurnUploadRequest(r.Context(), id); err != nil {
		a.bad(w, r, err)
		return
	}
	a.audit(r, "admin_burn_upload_request", id, "success", "")
	http.Redirect(w, r, "/admin#existing-links", 303)
}

func (a *App) adminSaveUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	if username == "" {
		http.Redirect(w, r, "/admin#manage-users", 303)
		return
	}
	role := strings.TrimSpace(r.FormValue("role"))
	if role == "" {
		role = "user"
	}
	firstName := strings.TrimSpace(r.FormValue("first_name"))
	lastName := strings.TrimSpace(r.FormValue("last_name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	existing, _ := a.store.GetUser(r.Context(), username)
	if existing.CreatedAt != 0 && existing.PasswordHash == "" {
		http.Redirect(w, r, "/admin#manage-users", 303)
		return
	}
	hash := existing.PasswordHash
	if password != "" {
		var err error
		hash, err = sec.HashPassphrase(password)
		if err != nil {
			a.bad(w, r, err)
			return
		}
	}
	if hash == "" {
		http.Redirect(w, r, "/admin#manage-users", 303)
		return
	}
	createdAt := existing.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	if err := a.store.SaveUser(r.Context(), redisstore.User{Username: username, PasswordHash: hash, Role: role, FirstName: firstName, LastName: lastName, Email: email, CreatedAt: createdAt}); err != nil {
		a.bad(w, r, err)
		return
	}
	if existing.CreatedAt == 0 {
		a.audit(r, "admin_create_user", username, "success", "role="+role)
	} else {
		a.audit(r, "admin_update_user", username, "success", "role="+role)
	}
	http.Redirect(w, r, "/admin#manage-users", 303)
}

func (a *App) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	current, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	username := chi.URLParam(r, "username")
	if username == current.Username {
		http.Redirect(w, r, "/admin#manage-users", 303)
		return
	}
	user, _ := a.store.GetUser(r.Context(), username)
	if user.PasswordHash == "" {
		http.Redirect(w, r, "/admin#manage-users", 303)
		return
	}
	if err := a.store.DeleteUser(r.Context(), username); err != nil {
		a.bad(w, r, err)
		return
	}
	a.audit(r, "admin_delete_user", username, "success", "")
	http.Redirect(w, r, "/admin#manage-users", 303)
}

func (a *App) adminSaveIntegrations(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	cfg, _ := a.getIntegrationConfig(r.Context())
	switch r.FormValue("section") {
	case "microsoft":
		cfg.MicrosoftEnabled = r.FormValue("microsoft_enabled") == "on"
		cfg.MicrosoftTenantID = strings.TrimSpace(r.FormValue("microsoft_tenant_id"))
		cfg.MicrosoftClientID = strings.TrimSpace(r.FormValue("microsoft_client_id"))
		cfg.MicrosoftAudience = strings.TrimSpace(r.FormValue("microsoft_audience"))
		cfg.MicrosoftAuthority = strings.TrimSpace(r.FormValue("microsoft_authority"))
		cfg.EntraAdminGroupName = strings.TrimSpace(r.FormValue("entra_admin_group_name"))
		cfg.EntraAdminGroupID = strings.TrimSpace(r.FormValue("entra_admin_group_id"))
		cfg.EntraUserGroupName = strings.TrimSpace(r.FormValue("entra_user_group_name"))
		cfg.EntraUserGroupID = strings.TrimSpace(r.FormValue("entra_user_group_id"))
	case "ad":
		cfg.ADEnabled = r.FormValue("ad_enabled") == "on"
		cfg.ADHost = strings.TrimSpace(r.FormValue("ad_host"))
		cfg.ADBaseDN = strings.TrimSpace(r.FormValue("ad_base_dn"))
		cfg.ADBindDN = strings.TrimSpace(r.FormValue("ad_bind_dn"))
		if v := r.FormValue("ad_bind_password"); v != "" {
			cfg.ADBindPassword = v
		}
	case "email":
		cfg.SMTPEnabled = r.FormValue("smtp_enabled") == "on"
		cfg.GraphEnabled = r.FormValue("graph_enabled") == "on"
		if cfg.SMTPEnabled && cfg.GraphEnabled {
			cfg.GraphEnabled = false
		}
		cfg.SMTPHost = strings.TrimSpace(r.FormValue("smtp_host"))
		cfg.SMTPPort = strings.TrimSpace(r.FormValue("smtp_port"))
		cfg.SMTPUsername = strings.TrimSpace(r.FormValue("smtp_username"))
		if v := r.FormValue("smtp_password"); v != "" {
			cfg.SMTPPassword = v
		}
		cfg.SMTPFrom = strings.TrimSpace(r.FormValue("smtp_from"))
		cfg.GraphTenantID = strings.TrimSpace(r.FormValue("graph_tenant_id"))
		cfg.GraphClientID = strings.TrimSpace(r.FormValue("graph_client_id"))
		if v := r.FormValue("graph_client_secret"); v != "" {
			cfg.GraphClientSecret = v
		}
		cfg.GraphSender = strings.TrimSpace(r.FormValue("graph_sender"))
	}
	if err := a.saveIntegrationConfig(r.Context(), cfg); err != nil {
		a.bad(w, r, err)
		return
	}
	section := r.FormValue("section")
	a.audit(r, "admin_update_settings", section, "success", "")
	http.Redirect(w, r, "/admin?saved="+url.QueryEscape(section)+"#"+adminSectionHash(section), 303)
}

func (a *App) adminUploadLogo(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		http.Redirect(w, r, "/admin#branding", 303)
		return
	}
	if r.FormValue("remove_logo") == "on" {
		_ = os.Remove(a.logoPath())
		a.audit(r, "admin_remove_logo", "branding", "success", "")
		http.Redirect(w, r, "/admin#branding", 303)
		return
	}
	fh, _, err := r.FormFile("logo")
	if err != nil {
		http.Redirect(w, r, "/admin#branding", 303)
		return
	}
	defer fh.Close()
	data, err := io.ReadAll(io.LimitReader(fh, 1024*1024+1))
	if err != nil || len(data) == 0 || len(data) > 1024*1024 {
		http.Redirect(w, r, "/admin#branding", 303)
		return
	}
	contentType := http.DetectContentType(data[:min(len(data), 512)])
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" && contentType != "image/gif" {
		http.Redirect(w, r, "/admin#branding", 303)
		return
	}
	if !diskCanAccept(a.cfg.StoragePath, int64(len(data))) {
		http.Redirect(w, r, "/admin#branding", 303)
		return
	}
	if err := os.WriteFile(a.logoPath(), data, 0600); err != nil {
		a.bad(w, r, err)
		return
	}
	a.audit(r, "admin_upload_logo", "branding", "success", contentType)
	http.Redirect(w, r, "/admin#branding", 303)
}

func adminSavedMessage(v string) string {
	if strings.HasPrefix(v, "settings_saved_") {
		return ""
	}
	return v
}

func adminSectionHash(section string) string {
	switch section {
	case "microsoft":
		return "microsoft-365"
	case "ad":
		return "local-ad"
	case "email":
		return "email-settings"
	case "branding":
		return "branding"
	case "audit":
		return "audit-logs"
	default:
		return "existing-links"
	}
}

func microsoftAuthority(cfg redisstore.IntegrationConfig) string {
	if strings.TrimSpace(cfg.MicrosoftAuthority) != "" {
		return strings.TrimRight(strings.TrimSpace(cfg.MicrosoftAuthority), "/")
	}
	return "https://login.microsoftonline.com/" + strings.TrimSpace(cfg.MicrosoftTenantID)
}

func microsoftAccessScope(cfg redisstore.IntegrationConfig) string {
	audience := strings.TrimSpace(cfg.MicrosoftAudience)
	audience = strings.TrimPrefix(audience, "api://")
	audience = strings.TrimSuffix(audience, "/access_as_user")
	if audience == "" {
		return ""
	}
	return "api://" + audience + "/access_as_user"
}

func microsoftScopes(cfg redisstore.IntegrationConfig) string {
	scopes := []string{"openid", "profile", "email"}
	if scope := microsoftAccessScope(cfg); scope != "" {
		scopes = append(scopes, scope)
	}
	return strings.Join(scopes, " ")
}

func setAuthCookie(w http.ResponseWriter, name, value string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure})
}
func clearAuthCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure})
}
func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

type microsoftTokenResponse struct {
	IDToken string `json:"id_token"`
}
type microsoftClaims struct {
	Audience          string            `json:"aud"`
	Issuer            string            `json:"iss"`
	Expires           int64             `json:"exp"`
	NotBefore         int64             `json:"nbf"`
	Nonce             string            `json:"nonce"`
	Subject           string            `json:"sub"`
	ObjectID          string            `json:"oid"`
	Name              string            `json:"name"`
	Email             string            `json:"email"`
	UPN               string            `json:"upn"`
	PreferredUsername string            `json:"preferred_username"`
	Groups            []string          `json:"groups"`
	ClaimNames        map[string]string `json:"_claim_names"`
}
type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}
type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
}

func exchangeMicrosoftCode(ctx context.Context, cfg redisstore.IntegrationConfig, baseURL, code, verifier string) (microsoftTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", cfg.MicrosoftClientID)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", strings.TrimRight(baseURL, "/")+"/auth/microsoft/callback")
	form.Set("code_verifier", verifier)
	form.Set("scope", microsoftScopes(cfg))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(microsoftAuthority(cfg), "/")+"/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return microsoftTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return microsoftTokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return microsoftTokenResponse{}, microsoftTokenError(resp)
	}
	var out microsoftTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	if out.IDToken == "" {
		return out, fmt.Errorf("missing id_token")
	}
	return out, nil
}

func microsoftTokenError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.ErrorDescription != "" {
			return fmt.Errorf("%s", payload.ErrorDescription)
		}
		if payload.Error != "" {
			return fmt.Errorf("%s", payload.Error)
		}
	}
	return fmt.Errorf("token endpoint returned %s", resp.Status)
}

func validateMicrosoftAccessToken(ctx context.Context, cfg redisstore.IntegrationConfig, token string) (microsoftClaims, error) {
	claims, err := validateMicrosoftJWT(ctx, cfg, token)
	if err != nil {
		return claims, err
	}
	audience := strings.TrimSpace(cfg.MicrosoftAudience)
	audience = strings.TrimPrefix(audience, "api://")
	audience = strings.TrimSuffix(audience, "/access_as_user")
	if claims.Audience != audience && claims.Audience != "api://"+audience {
		return claims, fmt.Errorf("invalid audience")
	}
	if _, ok := claims.ClaimNames["groups"]; ok {
		return claims, fmt.Errorf("Microsoft token uses group overage claims. Configure the app registration to emit groups for the API token or reduce group memberships before login")
	}
	return claims, nil
}

func validateMicrosoftIDToken(ctx context.Context, cfg redisstore.IntegrationConfig, token, nonce string) (microsoftClaims, error) {
	claims, err := validateMicrosoftJWT(ctx, cfg, token)
	if err != nil {
		return claims, err
	}
	if claims.Audience != cfg.MicrosoftClientID {
		return claims, fmt.Errorf("invalid audience")
	}
	if claims.Nonce != nonce {
		return claims, fmt.Errorf("invalid nonce")
	}
	return claims, nil
}

func validateMicrosoftJWT(ctx context.Context, cfg redisstore.IntegrationConfig, token string) (microsoftClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return microsoftClaims{}, fmt.Errorf("invalid jwt")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return microsoftClaims{}, err
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return microsoftClaims{}, err
	}
	var header struct {
		Kid string `json:"kid"`
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return microsoftClaims{}, err
	}
	if header.Alg != "RS256" {
		return microsoftClaims{}, fmt.Errorf("unsupported alg")
	}
	if strings.TrimSpace(header.Kid) == "" {
		return microsoftClaims{}, fmt.Errorf("missing key id")
	}
	key, err := microsoftSigningKey(ctx, cfg, header.Kid)
	if err != nil {
		return microsoftClaims{}, err
	}
	signed := []byte(parts[0] + "." + parts[1])
	digest := sha256.Sum256(signed)
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return microsoftClaims{}, err
	}
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return microsoftClaims{}, err
	}
	var claims microsoftClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return claims, err
	}
	now := time.Now().Unix()
	if now >= claims.Expires {
		return claims, fmt.Errorf("expired token")
	}
	if claims.NotBefore != 0 && now+300 < claims.NotBefore {
		return claims, fmt.Errorf("token not yet valid")
	}
	expectedIssuer := "https://login.microsoftonline.com/" + strings.TrimSpace(cfg.MicrosoftTenantID) + "/v2.0"
	legacyIssuer := "https://sts.windows.net/" + strings.TrimSpace(cfg.MicrosoftTenantID) + "/"
	configuredIssuer := strings.TrimRight(microsoftAuthority(cfg), "/") + "/v2.0"
	if claims.Issuer != expectedIssuer && claims.Issuer != legacyIssuer && claims.Issuer != configuredIssuer {
		return claims, fmt.Errorf("invalid issuer")
	}
	return claims, nil
}

func microsoftSigningKey(ctx context.Context, cfg redisstore.IntegrationConfig, kid string) (*rsa.PublicKey, error) {
	url := strings.TrimRight(microsoftAuthority(cfg), "/") + "/discovery/v2.0/keys"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("jwks endpoint returned %s", resp.Status)
	}
	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}
	for _, k := range jwks.Keys {
		if k.Kid == kid {
			if k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") {
				return nil, fmt.Errorf("unsupported signing key")
			}
			return jwkToRSA(k)
		}
	}
	return nil, fmt.Errorf("signing key not found")
}

func jwkToRSA(k jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e*256 + int(b)
	}
	if e == 0 {
		parsed, err := strconv.Atoi(string(eBytes))
		if err != nil {
			return nil, err
		}
		e = parsed
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func splitName(name string) (string, string) {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func microsoftRoleFromGroups(cfg redisstore.IntegrationConfig, groups []string, existingRole string) (string, bool) {
	adminConfigured := cfg.EntraAdminGroupID != "" || cfg.EntraAdminGroupName != ""
	userConfigured := cfg.EntraUserGroupID != "" || cfg.EntraUserGroupName != ""
	if adminConfigured && groupMatches(groups, cfg.EntraAdminGroupID, cfg.EntraAdminGroupName) {
		return "administrator", true
	}
	if userConfigured && groupMatches(groups, cfg.EntraUserGroupID, cfg.EntraUserGroupName) {
		return "user", true
	}
	if adminConfigured || userConfigured {
		return "", false
	}
	if existingRole != "" {
		return existingRole, true
	}
	return "user", true
}

func groupMatches(groups []string, id, name string) bool {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	for _, g := range groups {
		if (id != "" && strings.EqualFold(g, id)) || (name != "" && strings.EqualFold(g, name)) {
			return true
		}
	}
	return false
}
