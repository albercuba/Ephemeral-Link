# Ephemeral Link — Security & Reliability Hardening Roadmap

This roadmap tracks the security and reliability hardening work needed before treating Ephemeral Link as production-ready. Items are grouped by priority and should be implemented with tests where practical.

## 0. Ground rules

- [ ] Keep single-use claims atomic.
- [ ] Verify passphrases before claiming an item.
- [ ] Never log plaintext secrets, files, passphrases, generated links, raw encryption keys, SMTP credentials, Graph client secrets, or Microsoft tokens.
- [ ] Keep audit logs metadata-only.
- [ ] Keep files outside the public web root.
- [ ] Preserve AES-256-GCM encryption with per-item data keys wrapped by `ENCRYPTION_MASTER_KEY` unless replacing it with an equally strong reviewed design.
- [ ] Keep CSRF protection, request limits, secure headers, Redis TTL expiry, and cleanup behavior enabled.

## Phase 1 — High priority security

### 1.1 Remove the public fallback master key

- [x] Require `ENCRYPTION_MASTER_KEY` in all environments.
- [x] Reject missing, non-base64, or incorrectly sized master keys.
- [x] Document that the app will not start without a valid master key.

Status: complete / confirmed in `internal/config/config.go` and `README.md`.

### 1.2 Wipe payload from Redis when an item is claimed

- [x] Keep claim operation atomic with Redis Lua.
- [x] Return the claimed item to the request handler for delivery.
- [x] Remove wrapped key, payload nonce/ciphertext, and storage object path from Redis after a successful claim.
- [x] Add Redis-backed regression tests for the claim script.

Status: implemented in `internal/redisstore/store.go`; atomic claim and payload-wipe coverage is in `internal/redisstore/store_test.go`.

### 1.3 Trusted proxies instead of blindly trusting `X-Forwarded-For`

- [x] Remove blind `X-Forwarded-For` trust from rate limiting.
- [x] Remove blind `middleware.RealIP` usage.
- [x] Add `TRUSTED_PROXIES` configuration.
- [x] Trust `X-Forwarded-For` / `X-Real-IP` only when the direct peer is trusted.
- [x] Document `TRUSTED_PROXIES`.
- [x] Add middleware tests for trusted and untrusted proxy cases.

Status: implemented in `internal/web/server.go`, `internal/ratelimit/ratelimit.go`, `internal/config/config.go`, and `README.md`; trusted and untrusted proxy coverage is in `internal/web/security_test.go`.

### 1.4 Make first-run `/setup` safe

- [x] Use Redis `SETNX` lock for initial admin creation.
- [x] Recheck admin existence inside the locked creation path.
- [x] Add concurrent setup regression test.
- [ ] Consider rate limiting setup attempts.

Status: existing implementation reviewed in `internal/redisstore/store.go` and `internal/web/auth.go`; concurrent setup coverage is in `internal/redisstore/store_test.go`.

### 1.5 Brute-force protection for passphrases and logins

- [x] Add throttling for local login failures.
- [x] Add throttling for link passphrase failures.
- [x] Keep throttling metadata free of plaintext passphrases, secrets, or generated links.
- [x] Add tests for lockout/rate-limit behavior.

Status: implemented with Redis-backed failure counters in `internal/redisstore/store.go`, `internal/web/auth.go`, and `internal/web/server.go`; counter limit, expiry, and reset coverage is in `internal/redisstore/store_test.go`.

### 1.6 Neutralize CSV formula injection in the audit export

- [x] Sanitize dynamic audit CSV cells starting with spreadsheet formula trigger characters.
- [x] Preserve CSV export behavior while preventing formula execution in spreadsheet tools.
- [ ] Add unit tests for CSV cell sanitization.

Status: implemented in `internal/web/auth.go`; tests still needed.

## Phase 2 — Medium priority

### 2.1 Entra identity: key on `oid`, never merge with local accounts

- [x] Store Entra users under stable object ID (`oid`) instead of mutable usernames/emails.
- [x] Prevent accidental merge with local accounts.
- [x] Preserve display names/emails as metadata only.
- [ ] Add migration/compatibility handling for existing Entra users.
- [ ] Add tests for Entra/local account separation.

Status: implemented in `internal/web/auth.go` and `internal/redisstore/store.go`; migration and tests still needed.

### 2.2 Replace hand-rolled JWT validation

- [x] Replace custom JWT/JWKS validation with a maintained library or hardened verifier.
- [x] Validate issuer, audience, expiry, algorithm, key ID, and nonce where applicable.
- [x] Add tests for invalid audience/issuer/expiry/signature.

Status: hardened verifier in `internal/web/auth.go` validates RS256, key ID, RSA signing keys, issuer, audience, expiry, not-before, signature, and nonce where applicable; tests cover valid tokens plus invalid audience, issuer, expiry, signature, and nonce.

### 2.3 Remove `KEYS` from hot paths

- [x] Find any Redis `KEYS` usage.
- [x] Replace with indexes or `SCAN`-based iteration.
- [ ] Add tests for list/search behavior.

Status: implemented with cursor-based `SCAN` in `internal/redisstore/store.go`; tests still needed.

### 2.4 Stop putting the secret link into URLs

- [x] Avoid exposing generated links in query strings where possible.
- [x] Avoid leaking generated links via Referer headers, logs, or browser history.
- [x] Update templates and email flows without logging full links.
- [x] Add receipt-flow tests for `/created`.

Status: implemented with short-lived server-side created-link receipts and an opaque `/created` cookie; receipt creation coverage is in `internal/web/handler_test.go`.

### 2.5 Response headers: no-store and HSTS

- [x] Add `Cache-Control: no-store` for sensitive pages/responses.
- [x] Add HSTS when serving behind HTTPS / secure cookies.
- [ ] Verify headers on reveal, download, login, setup, and admin pages.
- [ ] Add security header tests.

Status: implemented in `internal/web/server.go`; tests still needed.

### 2.6 Self-host fonts, icons and MSAL; tighten the CSP

- [x] Remove production dependencies on external font/icon/script CDNs.
- [x] Self-host required static assets.
- [x] Tighten CSP after external font/icon assets are removed.
- [x] Update third-party notices.

Status: complete; external Google Fonts and Font Awesome stylesheets were removed and replaced with local CSS/system-font fallbacks, and the pinned MSAL 2.38.3 browser bundle is vendored under `web/static/vendor/`.

### 2.7 Encrypt integration secrets at rest

- [x] Encrypt SMTP credentials, Graph client secrets, and other integration secrets before storing them in Redis.
- [x] Reuse the master-key wrapping design or equivalent reviewed design.
- [x] Avoid displaying existing secret values back to the UI.
- [x] Add tests for save/load behavior.
- [x] Add explicit migration coverage for plaintext legacy values.

Status: implemented for SMTP password and Graph client secret in `internal/web/server.go`; helper tests cover encryption round-trip, no double encryption, and plaintext legacy values.

### 2.8 Turn the side-effecting download GET into a POST

- [x] Ensure claim/download side effects happen on POST only.
- [x] Preserve user experience with an interstitial form if needed.
- [x] Keep valid file downloads consumed even if transfer is interrupted.
- [x] Add handler tests.

Status: implemented for public file links and authenticated upload-request downloads; single-use streaming download coverage is in `internal/web/handler_test.go`.

### 2.9 CSRF and request-size hardening

- [x] Review all state-changing routes for CSRF coverage.
- [x] Ensure upload/text/request body limits are enforced consistently.
- [x] Add tests for oversized requests and missing/invalid CSRF tokens.
- [x] Add tests for route-aware POST body limits.

Status: implemented route-aware POST body caps in `internal/web/server.go`; tests cover missing/invalid CSRF tokens, oversized form bodies, and route-specific body limits.

## Phase 3 — Lower priority hardening, hygiene, and tests

### 3.1 Docker / Compose hardening

- [x] Review container user, filesystem permissions, health checks, and restart policy.
- [x] Avoid unnecessary writable paths.
- [x] Document production Compose settings.
- [x] Validate hardened Compose config in Docker.

Status: implemented app service hardening in `docker-compose.yml` with read-only root filesystem, `/tmp` tmpfs, dropped capabilities, no-new-privileges, healthcheck, and required `ENCRYPTION_MASTER_KEY`; `docker compose config` and `docker compose build` validated with a temporary test key.

### 3.2 File handling improvements

- [x] Review whole-file buffering and memory growth risks.
- [x] Improve cleanup of orphaned encrypted files.
- [x] Add tests for filename sanitization and storage cleanup.
- [x] Replace whole-file buffering with streaming encryption/decryption for large files.

Status: implemented chunked AES-256-GCM streaming for new direct file links in `internal/crypto`, `internal/storage`, and `internal/web`; legacy single-payload files remain readable during expiry-based migration. Crypto tests cover round-trip and tamper rejection.

### 3.3 i18n and error messages

- [ ] Ensure every visible UI string comes from `locales/en.json` and `locales/de.json`.
- [x] Keep user-facing email copy localized.
- [x] Avoid leaking sensitive operational details in user-facing errors.

Status: in progress; login and Microsoft sign-in errors now use localized messages, and token validation failures no longer expose verifier details to users. A full template/string audit is still needed.

### 3.4 Code hygiene

- [ ] Split large handlers where it improves readability.
- [ ] Remove dead code and unused assets.
- [ ] Keep security-sensitive helpers small and tested.

Status: not started.

### 3.5 Documentation cleanup

- [ ] Update deployment docs for every behavior/config change.
- [ ] Keep `.env.example`, README, and production docs in sync.
- [ ] Document operational backup/restore expectations.

Status: in progress; README updated for master key and trusted proxy configuration.

### 3.6 Test coverage

- [x] Add config validation tests.
- [x] Add Redis claim semantics tests.
- [x] Add setup race tests.
- [ ] Add passphrase/login brute-force tests.
- [x] Add audit CSV injection tests.
- [x] Add security header tests.
- [x] Add request-size/CSRF tests.

Status: in progress; focused tests cover config validation, Redis claim/setup semantics, audit CSV escaping, security headers, integration secret helpers, CSRF validation, route-aware body limits, receipts, and single-use streaming downloads.

## Final verification checklist

Run after the hardening work is complete:

```sh
gofmt -w ./...
go test ./...
docker compose config
docker compose build
```

Manual security checks:

- [x] App refuses to start without a valid `ENCRYPTION_MASTER_KEY`.
- [ ] Claimed Redis items no longer retain payload/key fields.
- [ ] Untrusted clients cannot spoof IPs with `X-Forwarded-For`.
- [ ] Setup cannot create multiple initial admins under concurrent requests.
- [ ] Login and passphrase brute-force attempts are throttled.
- [ ] Audit CSV export neutralizes formula injection.
- [ ] Sensitive pages and responses are not cached.
- [ ] File download/reveal semantics remain single-use.

## Future platform extensions

These are larger feature goals after the hardening roadmap:

- [ ] S3-compatible encrypted file storage backend.
- [ ] Scoped, revocable REST API keys.
- [ ] User/workspace isolation.
- [ ] Custom domains.
- [ ] Expanded administrative reporting.
