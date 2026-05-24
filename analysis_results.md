# Codebase Audit & Improvement Analysis: Ephemeral Link

This report details an audit of the **Ephemeral Link** codebase. It identifies missing features, security vulnerabilities, reliability concerns, architectural issues, and suggests actionable improvements.

---

## 1. Key Integration Gaps

### ⚠️ Unimplemented Microsoft Graph `sendMail` API
* **Finding**: The admin dashboard settings form (`admin.html`) collects, and the authentication system (`auth.go`) saves, configurations for Microsoft Graph mail delivery (`graph_tenant_id`, `graph_client_id`, `graph_client_secret`, and `graph_sender`). However, the actual email sending function (`sendEmail` in `internal/web/email.go`) only implements SMTP delivery:
  ```go
  if !cfg.SMTPEnabled {
      return errors.New("SMTP delivery is not enabled")
  }
  return sendSMTP(cfg, to, subject, body)
  ```
  If Graph delivery is enabled, the code will completely fail to send notifications.
* **Proposed Improvement**: Implement the OAuth 2.0 client credentials flow to get a token from `https://login.microsoftonline.com/<tenant-id>/oauth2/v2.0/token` with the scope `https://graph.microsoft.com/.default`, and post to the Microsoft Graph `sendMail` endpoint at `https://graph.microsoft.com/v1.0/users/<sender>/sendMail`.

### ⚠️ Stubbed External Authentication Providers
* **Finding**: The login screen (`login.html`) displays buttons for **Microsoft Entra ID** and **Local Active Directory** sign-in, but the corresponding endpoints in `internal/web/auth.go` are stubs returning `501 Not Implemented` errors:
  ```go
  func (a *App) microsoftLogin(w http.ResponseWriter, r *http.Request) {
      a.render(w, r, 501, "login.html", Page{Title: "Sign in", Error: "..."})
  }
  ```
* **Proposed Improvement**: Complete these integrations. For Microsoft Entra ID, configure a redirect-based OAuth flow using PKCE. For Active Directory, use a Go LDAP package (e.g., `go-ldap/ldap/v3`) to connect to the configured LDAP server, bind with credentials, and search/authenticate users.

---

## 2. Security & Resource Management Concerns

### 🛑 Rate Limiter Memory Leak
* **Finding**: The rate limiter (`internal/ratelimit/ratelimit.go`) uses an in-memory map to store request counts per IP address:
  ```go
  type Limiter struct {
      mu        sync.Mutex
      perMinute int
      buckets   map[string]bucket
  }
  ```
  Every unique IP that makes a request adds a new key to the `buckets` map. However, there is no expiration or cleanup mechanism. On a public-facing instance, this map will grow indefinitely as new IPs connect, eventually leading to out-of-memory crashes.
* **Proposed Improvement**:
  1. Introduce a periodic cleanup goroutine or check inside the middleware to evict buckets whose reset time has passed.
  2. Alternatively, migrate the rate limiter to use Redis keys with a standard TTL (e.g., `el:rate:<ip>`), which is native, distributed, and cleans up automatically.

### 🛑 Large File Memory Allocation during Download
* **Finding**: The file download handler (`serveClaimedFile` in `internal/web/server.go`) loads the entire encrypted file payload from disk into memory using `os.ReadFile`:
  ```go
  enc, err := a.files.Read(claimed.StorageObjectPath)
  ...
  plain, err := sec.Decrypt(sec.EncryptedPayload{Nonce: claimed.PayloadNonce, Ciphertext: string(enc)}, key)
  ...
  w.Write(plain)
  ```
  If `MAX_FILE_SIZE` is configured to be high (e.g., 50MB–100MB+), concurrent downloads will cause massive memory spikes, making the server susceptible to Denial of Service (DoS) attacks.
* **Proposed Improvement**: Refactor storage and encryption to support chunked streaming. Instead of reading the whole payload into memory, stream the file in 64KB blocks, decrypting each block individually using an AEAD construction (like AES-GCM or XChaCha20-Poly1305 in chunked/streaming mode) and piping it directly to the response writer.

### ⚠️ Local Storage File Cleanup Limitations
* **Finding**: The cleanup task in `internal/cleanup/cleanup.go` runs every 30 minutes and scans the disk directory, deleting files modified longer than `maxTTL + time.Hour` ago. However, if a link is claimed, the Redis metadata is marked `consumed` (or deleted) and the backend attempts to delete the local file. If that deletion fails (due to locking, permissions, or system restart), the orphaned bin file stays on disk until the long `maxTTL` period passes.
* **Proposed Improvement**: The cleanup script should reconcile files on disk against active items in Redis. Any file in the storage directory that has no matching active key in Redis should be deleted immediately.

---

## 3. Code Health & Aesthetics

### 🧹 Dead/Unused CSS Assets
* **Finding**: The repository includes `web/static/style.css` labeled `/* ─── Ephemeral Link — Dark Vault Aesthetic ─────────────────────────────── */`. However, all templates reference `/static/app.css` (which contains the "Ephemeral Link layout"). `style.css` is completely unused.
* **Proposed Improvement**:
  1. Either integrate the dark vault style as a first-class feature/theme toggle.
  2. Or delete the unused CSS file to clean up the workspace.

### 🧪 Test Coverage Deficiencies
* **Finding**: Only the `internal/crypto` package has any tests. Critical areas like the Chi web router and handlers (`internal/web`), translation loading (`internal/i18n`), local file storage operations (`internal/storage`), and the atomic claim Redis Lua script (`internal/redisstore`) have zero test coverage.
* **Proposed Improvement**:
  - Add integration tests for HTTP handlers using Go's `net/http/httptest`.
  - Add tests for Redis Lua claims script using a mock/test Redis/Valkey container or miniredis.

---

## 4. Prioritized Action Plan

| Priority | Area | Task | Impact |
|---|---|---|---|
| **High** | Resource Safety | Fix the memory leak in the in-memory rate limiter. | Prevents gradual memory exhaustion / crashes. |
| **High** | Functionality | Implement the Microsoft Graph `sendMail` API handler. | Fixes broken email notification delivery. |
| **Medium** | Security / Performance | Refactor file decryption to stream rather than buffer in memory. | Drastically reduces memory overhead for file sharing. |
| **Medium** | Authentication | Implement Microsoft Entra ID and Active Directory login providers. | Enables secure enterprise sign-in. |
| **Low** | Code Quality | Clean up unused `style.css` or integrate it as a dark/light theme switch. | Cleans codebase & adds dark mode. |
| **Low** | Test Coverage | Add basic handler unit/integration tests. | Prevents regressions. |
