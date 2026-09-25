# Production configuration guide

This guide covers the minimum production settings for running Ephemeral Link safely with Docker Compose, Redis/Valkey, and local encrypted file storage. For an operator-focused step-by-step list, use the [production deployment checklist](production-deployment-checklist.md).

## Production checklist

Before exposing the app publicly:

- Set a public HTTPS `APP_BASE_URL`, for example `https://links.example.com`.
- Set `SECURE_COOKIES=true` behind HTTPS.
- Generate a unique `ENCRYPTION_MASTER_KEY` and keep it stable while links may still be active.
- Complete the first-run `/setup` flow with a strong administrator password before exposing the app broadly.
- Persist both Redis/Valkey data and `STORAGE_PATH` using Docker volumes or equivalent storage.
- Keep `STORAGE_PATH` outside the public web root.
- Ensure `ENCRYPTION_MASTER_KEY` is set before running Compose; the included Compose file intentionally fails fast if it is missing.
- Keep `MAX_FILE_SIZE` conservative unless memory and disk limits are sized for larger uploads/downloads.
- Configure SMTP or Microsoft Graph only if upload-request emails/notifications are needed.
- Put the container behind a reverse proxy that terminates TLS and forwards the original client IP.
- Back up configuration secrets securely; do not back up ephemeral uploaded files as durable records.

## Docker Compose defaults

The included `docker-compose.yml` runs two services:

- `app`: the Go web server, listening on port `8080` in the container.
- `redis`: Valkey 8 with append-only persistence enabled.

The `app` service runs as the non-root user baked into the image, uses a read-only root filesystem, drops Linux capabilities, enables `no-new-privileges`, mounts only `/app/data/storage` as persistent writable storage, and uses a small `/tmp` tmpfs for multipart parsing scratch space. Keep uploaded files and branding assets under `STORAGE_PATH`; do not make the application source tree writable in production.

It also creates two named volumes:

```yaml
volumes:
  app_storage:
  redis_data:
```

`app_storage` is mounted at:

```text
/app/data/storage
```

inside the app container. This is where encrypted uploaded files are stored by default. On the host, Docker manages this named volume. To inspect its host mountpoint:

```sh
docker volume inspect <project>_app_storage
```

The exact volume name is often prefixed by the Compose project name. Use:

```sh
docker volume ls
```

Redis/Valkey data is stored in the `redis_data` named volume.

## Required secret: `ENCRYPTION_MASTER_KEY`

`ENCRYPTION_MASTER_KEY` is required. It must decode to exactly 32 bytes and may be prefixed with `base64:`.

Generate a key:

```sh
printf 'ENCRYPTION_MASTER_KEY=base64:%s\n' "$(openssl rand -base64 32)"
```

Set this in your production environment or `.env` file before running Docker Compose. The included Compose file uses required-variable interpolation so startup fails if the key is missing.

Important:

- Do not add a default/fallback master key to Compose, deployment manifests, or source control.
- Do not rotate this key while active links still need to be decrypted.
- If the key is lost or changed, existing encrypted text and file payloads cannot be decrypted.
- Do not paste this key into logs, tickets, screenshots, or source control.

## Core environment variables

| Variable | Production guidance |
| --- | --- |
| `APP_BASE_URL` | Set to the public HTTPS origin, for example `https://links.example.com`. Generated links use this value. |
| `REDIS_URL` | Usually `redis://redis:6379/0` in Compose. Use a private network only; do not expose Redis publicly. |
| `STORAGE_PATH` | Defaults to `/app/data/storage` in Compose. Must be outside `web/static` and backed by a persistent volume. |
| `MAX_TEXT_SECRET_SIZE` | Keep small for secrets; default is `65536` bytes. |
| `MAX_FILE_SIZE` | Default is `10485760` bytes (10 MiB). Increase only after sizing memory, disk, reverse proxy limits, and upload timeouts. |
| `DEFAULT_TTL_SECONDS` | Default link lifetime when users do not choose one. |
| `MAX_TTL_SECONDS` | Maximum allowed link lifetime. Shorter is safer for ephemeral sharing. |
| `RATE_LIMIT_PER_MINUTE` | Per-IP request limit. Tune for expected traffic and reverse proxy behavior. |
| `ALLOWED_LANGUAGES` | Comma-separated UI languages, currently `en,de` by default. |
| `DEFAULT_LANGUAGE` | Keep `en` unless intentionally changed. |
| `SECURE_COOKIES` | Set to `true` when served via HTTPS. |
| `TRUSTED_PROXIES` | Comma-separated proxy IPs/CIDRs allowed to supply `X-Forwarded-For` / `X-Real-IP`. Leave empty when no trusted reverse proxy is in front of the app. |

## Custom domains

Set `CUSTOM_DOMAINS` to a comma-separated exact hostname allowlist, for example `links.example.com,secure.example.net`. When a request arrives with an allowlisted `Host`, generated links use that host; arbitrary or untrusted `Host` headers always fall back to `APP_BASE_URL`. Configure TLS for every custom domain and keep the application behind the trusted reverse proxy.

## Reverse proxy and TLS

Run Ephemeral Link behind a TLS reverse proxy such as Caddy, Nginx, Traefik, or a cloud load balancer.

Recommended proxy behavior:

- Terminate HTTPS at the proxy.
- Forward traffic to the app on the private Docker/network port.
- Set `X-Forwarded-For` so rate limiting sees the original client IP.
- Add only the reverse proxy's private IP/CIDR to `TRUSTED_PROXIES`; never trust arbitrary client-supplied forwarding headers.
- Enforce a request body size at or slightly above `MAX_FILE_SIZE` plus overhead.
- Set reasonable read/write timeouts for file uploads and downloads.

Example production values:

```env
APP_BASE_URL=https://links.example.com
SECURE_COOKIES=true
MAX_FILE_SIZE=10485760
RATE_LIMIT_PER_MINUTE=60
```

## Redis/Valkey persistence

The application stores metadata, TTLs, users, sessions, audit logs, upload requests, and single-use claim state in Redis/Valkey.

Production recommendations:

- Keep Valkey on a private network.
- Persist Redis/Valkey data using the `redis_data` volume or managed Redis persistence.
- Monitor memory usage and eviction policy. Do not use an eviction policy that can remove active item metadata unexpectedly.
- Back up Redis only if your operational model requires restoring active links after failure. Remember this is an ephemeral app, not durable storage.

## File storage

The default `local` backend stores encrypted payloads under `STORAGE_PATH`. For S3-compatible storage, set `STORAGE_BACKEND=s3`, configure the endpoint, credentials, bucket, and prefix, and keep `S3_SECURE=true` unless the endpoint is an explicitly isolated development service. Uploads use streaming multipart S3 writes; downloads use streaming object readers. Cleanup removes expired objects and objects not referenced by active Redis metadata.

S3 migration is additive: existing local object references remain readable only while the local backend is configured. Migrate active objects and their Redis `storage_object_path` values together during a maintenance window; never delete the local backend until active links have been verified.

Uploaded files are encrypted before being written to `STORAGE_PATH` as `.bin` files. Original filenames are stored as sanitized metadata and are never used as filesystem paths.

Production recommendations:

- Mount `STORAGE_PATH` as a persistent Docker volume.
- Keep it outside the public web root.
- Do not serve the storage directory directly with Nginx/Caddy.
- Monitor disk usage. Uploads are rejected when projected disk usage approaches the configured 98% guard.
- Treat stored files as temporary encrypted payloads, not backups.
- The cleanup worker removes old files and reconciles orphaned `.bin` files against active Redis metadata.

To inspect files inside the container:

```sh
docker compose exec app ls -la /app/data/storage
```

The files are encrypted and named by item ID, for example:

```text
/app/data/storage/<item-id>.bin
```

## Admin account and users

On first startup, if no local administrator exists, the app redirects unauthenticated users to `/setup`. Use this single-use setup flow to create the initial administrator account. Setup is disabled automatically once an administrator exists.

Before production use:

1. Open `/setup` through the public HTTPS URL or a trusted private network.
2. Create the first administrator with a strong password of at least 12 characters.
3. Store the password securely.
4. Create additional administrator/user accounts as needed.
5. Remove or lock down any unused accounts.
6. Ensure admin users have email addresses if they need upload-request notifications.

## Workspace membership and migration

Every Redis record now carries a workspace ID. Existing records normalize to `DEFAULT_WORKSPACE_ID` (normally `default`). Administrators can create one-time, expiring invitations with `POST /admin/workspace-invitations`; the invited authenticated user accepts with `POST /auth/workspace-invitations/accept`. Invitations are email-bound when an email is supplied and assign the configured workspace role.

Use the workspace migration tool after reviewing the target workspace:

```sh
go run ./cmd/migrate-workspaces -from default -to team-a
```

Administrative listings, burns, audit exports, and API-key audit reporting enforce the current workspace. Anonymous opaque links remain accessible by possession of the link, intentionally preserving the single-use sharing model.

## Local Active Directory authentication

The optional Local AD integration authenticates users against the configured LDAP endpoint and creates a local session without storing the directory password. Configure it from Admin → Microsoft Local AD Integration:

- `AD server host`: use an `ldaps://` URL where possible, for example `ldaps://dc01.example.internal:636`.
- `Base DN`: the directory subtree used to search users.
- `Bind DN` and `Bind password`: a least-privilege service account used only to search the directory. The bind password is encrypted with `ENCRYPTION_MASTER_KEY` before it is stored in Redis.
- Enable the integration only after testing the endpoint and firewall path from the app container.

User authentication searches by `sAMAccountName`, then binds as the matched user with the submitted password. Passwords and directory tokens are never logged or stored. If a plain `ldap://` URL is used, traffic is not encrypted; prefer LDAPS and restrict the directory network path.

## Email delivery

Email is used for direct text/file link delivery, upload-request links, and upload notifications. Configure one delivery method.

### SMTP

Use SMTP when you have a mail relay or transactional mail provider.

Required settings in Admin → Email settings:

- SMTP enabled
- SMTP host
- SMTP port
- SMTP username/password if required
- From address

Use a least-privilege mailbox or API credential dedicated to Ephemeral Link notifications.

### Microsoft Graph sendMail

Use Graph sendMail for Microsoft 365 app-only mail delivery.

Production guidance:

- Create a dedicated Entra app registration for mail delivery.
- Grant only Microsoft Graph application permission `Mail.Send`.
- Grant admin consent.
- Restrict the app to a dedicated sender mailbox with an Exchange Online application access policy where possible.
- Store the Graph client secret securely and rotate it before expiry.

The app uses client credentials against:

```text
https://login.microsoftonline.com/<tenant-id>/oauth2/v2.0/token
```

with scope:

```text
https://graph.microsoft.com/.default
```

and sends mail via:

```text
POST https://graph.microsoft.com/v1.0/users/<sender>/sendMail
```

Never log SMTP passwords, Graph client secrets, access tokens, email bodies, generated links, or recipient-sensitive data. If a link has a passphrase, share the passphrase through a separate channel from the emailed link.

## Microsoft Entra ID sign-in

Microsoft sign-in uses the browser MSAL authorization-code-with-PKCE flow. The browser gets an access token for a backend API app registration, and the Go backend validates the token against Entra metadata/JWKS before creating an HTTP-only app session.

Recommended setup:

1. Create a backend API app registration.
2. Expose an `access_as_user` delegated scope.
3. Create a frontend SPA app registration.
4. Add a SPA redirect URI matching `<APP_BASE_URL>/login`.
5. Grant the frontend app permission to the backend API scope.
6. Grant admin consent if required.
7. Configure group claims if using Entra group-to-role mapping.
8. Enter the tenant ID, frontend client ID, backend API audience/client ID, authority URL, and allowed group mappings in Admin → Microsoft 365 Integration.

Do not create or store a client secret for the browser login app.

## Local Active Directory

The Local AD settings are currently configuration-only until LDAP validation is fully implemented. Do not rely on the Local AD button for production authentication unless the LDAP integration has been completed and tested in your deployment.

## Logging and audit logs

Audit logs intentionally store metadata only:

- actor
- event
- target reference
- result
- source IP
- short operational details

Never log:

- plaintext secrets
- file contents
- passphrases
- generated links
- raw encryption keys
- SMTP passwords
- Graph client secrets or tokens

## Backups and recovery

Back up these production secrets/configuration values:

- `ENCRYPTION_MASTER_KEY`
- `.env` or secret manager entries
- reverse proxy configuration
- mail/Graph app registration details, excluding expired rotated secrets

Think carefully before backing up Redis or file storage. Restoring old Redis/storage snapshots can resurrect active links or preserve encrypted payloads longer than intended. If you do back up these volumes, align retention with your security policy.

## Updating the deployment

Before updating:

1. Review release notes or Git commits.
2. Back up configuration secrets.
3. Ensure `ENCRYPTION_MASTER_KEY` is unchanged.
4. Run:

```sh
docker compose config
docker compose build
docker compose up -d
```

After updating:

```sh
docker compose ps
docker compose logs app
```

Confirm `/healthz` and `/readyz` are healthy through your reverse proxy.
