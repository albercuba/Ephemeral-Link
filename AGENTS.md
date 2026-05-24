# AGENTS.md

## Project overview

Ephemeral Link is a lightweight burn-after-reading secret and file sharing app.

Stack:

- Backend: Go
- Router: Chi
- UI: server-rendered HTML templates with small JavaScript helpers
- Styling: Tailwind-inspired static CSS for the MVP; keep production free of a Node.js runtime
- State: Redis/Valkey for metadata, TTLs, and atomic single-use claims
- File storage: local encrypted filesystem storage under `STORAGE_PATH`, outside the public web root
- Deployment: Docker and Docker Compose

## Security rules

- Never store plaintext secrets, file contents, passphrases, generated links, or raw encryption keys in logs.
- Keep single-use access atomic. Use Redis Lua or another atomic Redis operation for claim semantics.
- Verify passphrases before claiming an item so incorrect passphrases do not consume links.
- After a valid file download request is claimed, treat it as consumed even if the transfer is interrupted.
- Use cryptographically secure randomness for IDs, keys, nonces, and CSRF tokens.
- Keep AES-256-GCM encryption with per-item data keys wrapped by `ENCRYPTION_MASTER_KEY` unless replacing it with an equally strong reviewed design.
- Keep files outside the public web root and never expose original filesystem paths.
- Sanitize filenames and serve downloads with safe `Content-Disposition` headers.
- Keep request size limits, CSRF protection, rate limiting, and secure HTTP headers enabled.
- Do not weaken Redis TTL expiry or cleanup behavior.

## Code structure

- `cmd/server`: application entrypoint.
- `internal/config`: environment configuration.
- `internal/crypto`: token generation, encryption, key wrapping, passphrase hashing.
- `internal/redisstore`: Redis persistence and single-use claim logic.
- `internal/storage`: local encrypted file storage helpers.
- `internal/web`: HTTP routes, templates, middleware, and request handling.
- `internal/i18n`: translation loading and lookup.
- `internal/ratelimit`: lightweight per-IP rate limiting.
- `internal/cleanup`: background cleanup.
- `web/templates`: server-rendered templates.
- `web/static`: static CSS and JavaScript.
- `locales`: translation JSON files.

## Internationalization

- Every visible UI string should come from `locales/en.json` and `locales/de.json`.
- When adding UI copy, update all supported languages.
- Keep `DEFAULT_LANGUAGE=en` unless explicitly asked to change it.

## Proactive improvement suggestions

When making code changes, also look for nearby security, reliability, resource-management, and functionality gaps that are relevant to the touched area. Do not expand the implementation scope without being asked, but do mention concise follow-up recommendations in the final summary when they are clearly useful.

Prioritize calling out:

- Security issues that could weaken single-use access, encryption, CSRF protection, rate limiting, logging hygiene, or file isolation.
- Resource-safety issues such as unbounded memory growth, whole-file buffering, disk cleanup gaps, or missing TTL cleanup.
- UI/configuration mismatches where the app exposes settings or buttons for integrations that are not implemented.
- Missing tests for code paths changed during the task, especially web handlers, templates, Redis claim semantics, cleanup, and storage behavior.
- Dead or unused assets discovered while editing related templates/static files.

When suggesting improvements:

- Keep them separate from the completed work unless the user explicitly asks to implement them.
- Explain the practical impact in one sentence.
- Prefer actionable commit-style follow-up titles, for example `fix: evict expired rate limit buckets`.
- Avoid overwhelming the user with unrelated audit items; surface only the highest-value recommendations tied to the current change.

## Validation

Run the most relevant checks before finishing changes:

```sh
go test ./...
docker compose config
docker compose build
```

If Go or Docker is unavailable in the environment, state that clearly in the summary.

## Future extensions

The code should remain easy to extend with S3-compatible storage, user accounts, custom domains, email delivery, REST API keys, team workspaces, audit logs, and an admin dashboard without weakening the anonymous single-use sharing model.
