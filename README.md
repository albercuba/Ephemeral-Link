# Ephemeral Link

Ephemeral Link is a lightweight, Go-based “burn after reading” web app for sharing text secrets and files through single-use links. It is an independent implementation built with a small server-rendered stack: Go, Chi, Redis/Valkey, local encrypted file storage, HTML templates, and minimal JavaScript.

## Features

- Login-protected link creation with a single-use first-run setup flow for the initial local administrator
- Admin dashboard for active links, local users, audit logs, and Microsoft Entra ID / local AD integration settings
- Single-use text secret links
- Single-use file download links
- Optional passphrase protection
- Optional direct link delivery by email through SMTP or Microsoft Graph
- AES-256-GCM encryption before storage
- Per-item random data keys wrapped by `ENCRYPTION_MASTER_KEY`
- Argon2id passphrase hashing
- Redis TTL expiry and atomic claim script for single-use access
- Local encrypted file storage outside the web root
- English and German UI translations
- Browser language detection with English fallback
- Language cookie and language switcher
- Rate limiting, CSRF checks, request size limits, and secure HTTP headers
- Docker Compose deployment with Valkey

## License

Ephemeral Link is licensed under the MIT License. See [LICENSE](LICENSE).

## Security model

Each secret/file gets a cryptographically random opaque ID and a random 256-bit data encryption key. The payload is encrypted with AES-256-GCM. The data key is encrypted with the configured master key and stored as metadata. Plaintext secrets, passphrases, and file contents are not stored.

Single-use access uses an atomic Redis Lua script. The first valid reveal/download request changes the item from `available` to `consumed`; later requests receive the gone page. For files, the link is consumed before the response is sent. If a download is interrupted, the item remains consumed for security.

Passphrases are hashed with Argon2id. The passphrase is verified before the single-use claim, so incorrect passphrases do not consume the item.

## Authentication

On first run, if no local administrator exists, the app redirects unauthenticated users to `/setup` to create the initial administrator account. The setup flow is disabled automatically as soon as an administrator exists. Local users are stored in the app's Redis-backed internal store with Argon2id password hashes. Link creation requires a session cookie, and admin screens require a signed-in user with the `administrator` role. Single-use reveal/download links remain accessible to recipients without an account.

The login screen includes buttons for Microsoft Entra ID and local Active Directory sign-in. Microsoft Entra ID sign-in mirrors the M365 Toolbox model: the browser uses MSAL's SPA authorization-code-with-PKCE flow to get an access token for a backend API app registration, then the Go backend validates that access token against Entra JWKS metadata before creating an HTTP-only app session. Configure the frontend app registration with a SPA redirect URI of `<APP_BASE_URL>/login`, for example `https://links.example.com/login`, grant it the backend API app's delegated `access_as_user` permission, and enter the tenant ID, frontend client ID, backend API audience/client ID, and optional authority URL in Admin → Microsoft 365 Integration. Do not create or store a client secret for Microsoft login. The local Active Directory sign-in button is still configuration-only until LDAP validation is wired.

## Audit logs

Administrators can review recent security and administrative events from Admin → Audit logs. Audit entries include the timestamp, actor, event type, target reference, result, source IP, and short operational details. The audit trail is intentionally metadata-only: plaintext secrets, passphrases, generated single-use links, encryption keys, Microsoft tokens, SMTP credentials, Graph secrets, and file contents are never written to audit entries. Audit entries are stored in Redis/Valkey and the app keeps the latest 500 entries for up to 90 days.

## Email settings and Microsoft Graph sendMail

Administrators can configure SMTP settings or Microsoft Graph `sendMail` with app-only authentication from Admin → Email settings. Direct link emails use a styled HTML message with a plain-text fallback and follow the sender's current UI language. Secrets such as SMTP passwords and Graph client secrets should be handled like deployment secrets and must not be logged or shared.

To configure Microsoft Graph `sendMail` with app-only authentication:

1. In Microsoft Entra admin center, open App registrations → New registration.
2. Name the app registration clearly, for example `Ephemeral Link Mailer`. Use `Accounts in this organizational directory only` unless you intentionally need multi-tenant mail delivery.
3. Do not add a redirect URI for this mailer app. The Graph `sendMail` integration uses app-only client credentials, not a browser sign-in callback.
4. After creating the app, copy the Directory/tenant ID into Admin → Email settings → Microsoft Graph sendMail → `Tenant ID`.
5. Copy the Application/client ID into the `Client ID` field.
6. Open Certificates & secrets → Client secrets → New client secret. Choose a short, managed expiry, create the secret, and copy the secret `Value` immediately. Paste the `Value`, not the Secret ID, into the `Client secret` field.
7. Open API permissions → Add a permission → Microsoft Graph → Application permissions. Add only `Mail.Send` unless there is a documented reason for broader access.
8. Click Grant admin consent for the tenant and confirm Microsoft Graph `Mail.Send` shows as granted for your organization.
9. Choose the sender mailbox that Graph should send as, for example `secure-links@example.com`. Enter that mailbox address in `Sender mailbox`. The mailbox must exist in Exchange Online and be allowed for app-only sending.
10. For least privilege, create an Exchange Online application access policy that restricts this app registration to only the approved sender mailbox or a dedicated mail-enabled security group.
11. In Ephemeral Link, enable Graph sendMail, save the Tenant ID, Client ID, Client secret, and Sender mailbox, then create a test upload request or notification to verify delivery.
12. The server-side flow uses the OAuth client credentials grant against `https://login.microsoftonline.com/<tenant-id>/oauth2/v2.0/token` with scope `https://graph.microsoft.com/.default`, then calls `POST https://graph.microsoft.com/v1.0/users/<sender>/sendMail`.

Review the app registration regularly, rotate credentials before expiry, and do not grant broader Graph permissions than `Mail.Send` for notification delivery. Never paste Graph secrets into logs, tickets, screenshots, browser local storage, or source control.

## Limitations

This MVP uses local filesystem storage and reads encrypted files into memory for decrypt/download. Keep `MAX_FILE_SIZE` conservative. The storage interface is intentionally simple so S3-compatible storage can be added later. This app is for ephemeral sharing, not durable storage or backups.

## Quick start with Docker Compose

```sh
cp .env.example .env
# Edit .env and replace ENCRYPTION_MASTER_KEY with: base64:$(openssl rand -base64 32)
docker compose up --build
```

Open `http://localhost:8080`.

### Create `ENCRYPTION_MASTER_KEY`

`ENCRYPTION_MASTER_KEY` must be a base64-encoded 32-byte key. The app uses it to wrap per-item encryption keys, so keep it secret and stable for as long as active links may need to be decrypted.

Generate one with OpenSSL:

```sh
openssl rand -base64 32
```

Then add it to your `.env` file with the `base64:` prefix:

```env
ENCRYPTION_MASTER_KEY=base64:<paste-generated-value-here>
```

Or generate and print the full environment line in one command:

```sh
printf 'ENCRYPTION_MASTER_KEY=base64:%s\n' "$(openssl rand -base64 32)"
```

Example output:

```env
ENCRYPTION_MASTER_KEY=base64:REPLACE_WITH_GENERATED_32_BYTE_BASE64_VALUE
```

Do not reuse the example value above. If you change `ENCRYPTION_MASTER_KEY`, existing encrypted secrets and files can no longer be decrypted.

`ENCRYPTION_MASTER_KEY` is required in all environments. The app will refuse to start without a strong unique key.

## Local development

Requirements:

- Go 1.23+
- Redis or Valkey

```sh
cp .env.example .env
# Start Redis separately, then use a local URL:
export REDIS_URL=redis://localhost:6379/0
export STORAGE_PATH=./data/storage
export ENCRYPTION_MASTER_KEY=base64:$(openssl rand -base64 32)
go run ./cmd/server
```

Run tests:

```sh
go test ./...
```

## Environment variables

| Variable | Description |
| --- | --- |
| `APP_BASE_URL` | Public URL used when generating single-use links. |
| `REDIS_URL` | Redis/Valkey connection URL. |
| `STORAGE_PATH` | Directory for encrypted file payloads. Must not be public web root. |
| `MAX_TEXT_SECRET_SIZE` | Maximum text secret size in bytes. |
| `MAX_FILE_SIZE` | Maximum file upload size in bytes. |
| `DEFAULT_TTL_SECONDS` | Default expiry for new items. |
| `MAX_TTL_SECONDS` | Upper bound for allowed expiry. Defaults to 30 days. |
| `ENCRYPTION_MASTER_KEY` | Required base64-encoded 32-byte master key, optionally prefixed with `base64:`. |
| `RATE_LIMIT_PER_MINUTE` | Per-IP request limit. |
| `TRUSTED_PROXIES` | Optional comma-separated trusted reverse proxy IPs/CIDRs. `X-Forwarded-For` and `X-Real-IP` are ignored unless the direct peer is trusted. |
| `ALLOWED_LANGUAGES` | Comma-separated language list, default `en,de`. |
| `DEFAULT_LANGUAGE` | Default UI language, default `en`. |
| `SECURE_COOKIES` | Set `true` behind HTTPS in production. |

## Roadmap

See [docs/roadmap.md](docs/roadmap.md) for the v1.0.0 and v2.0.0 goals.

## Production deployment notes

Use the [production deployment checklist](docs/production-deployment-checklist.md) before exposing Ephemeral Link to users. See the [production configuration guide](docs/production-configuration.md) for details on Docker volumes, Redis/Valkey persistence, reverse proxy/TLS settings, file storage, email delivery, Microsoft Entra ID sign-in, backups, and operational security.

Minimum production requirements:

- Put the app behind Caddy, Nginx, or another HTTPS reverse proxy.
- Set `APP_BASE_URL` to the public HTTPS URL.
- Set `SECURE_COOKIES=true` when serving over HTTPS.
- Set `TRUSTED_PROXIES` to your reverse proxy IP/CIDR if rate limits and audit IPs should use forwarded client addresses.
- Generate a strong master key and keep it secret. Losing it makes stored encrypted payloads unrecoverable.
- Complete the first-run `/setup` flow with a strong administrator password before exposing the app broadly.
- Do not log generated links, passphrases, or payloads.
- Persist Redis/Valkey data if you want TTL metadata to survive restarts.
- Persist `STORAGE_PATH` only for the life of active links; do not treat it as backup storage.

## Visual assets

The UI uses the Google Font `Share Tech Mono`, which is distributed under the SIL Open Font License. Icons are loaded from Font Awesome Free. Font Awesome Free uses mixed licensing: icons are CC BY 4.0, fonts are SIL OFL 1.1, and code is MIT. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Language behavior

On the first visit, Ephemeral Link checks the browser `Accept-Language` header and uses the first supported language. With the default languages, German browsers receive the German UI and unsupported browser languages fall back to English. If a user changes the language in the UI, the app stores that choice in the `lang` cookie and uses it on later requests.

## Adding another language

1. Add `locales/<lang>.json` with the same keys as `locales/en.json`.
2. Add the language code to `ALLOWED_LANGUAGES`, for example `en,de,fr`.
3. Restart the app.

Every visible UI string should come from the translation files.

## File storage notes

Uploaded files are encrypted before being written under `STORAGE_PATH`. The original filesystem path is never exposed. Recipients receive a sanitized filename through `Content-Disposition`. A background cleanup goroutine deletes old orphaned files based on `MAX_TTL_SECONDS`.

## Future extension points

The code is structured for later additions such as custom domains, S3-compatible storage, REST API keys, multi-team workspace isolation, and expanded administrative reporting.
