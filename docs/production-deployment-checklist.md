# Production deployment checklist

Use this checklist before exposing Ephemeral Link to users. It is intentionally operational and should be completed alongside the broader [production configuration guide](production-configuration.md).

## 1. Prepare secrets and environment

- [ ] Generate a unique 32-byte master key:

  ```sh
  printf 'ENCRYPTION_MASTER_KEY=base64:%s\n' "$(openssl rand -base64 32)"
  ```

- [ ] Store `ENCRYPTION_MASTER_KEY` in a secret manager or protected `.env` file.
- [ ] Confirm the Compose fallback example key is not used in production.
- [ ] Set the public URL:

  ```env
  APP_BASE_URL=https://links.example.com
  ```

- [ ] Enable secure cookies for HTTPS deployments:

  ```env
  SECURE_COOKIES=true
  ```

- [ ] Choose conservative upload limits, for example:

  ```env
  MAX_FILE_SIZE=10485760
  MAX_TEXT_SECRET_SIZE=65536
  ```

- [ ] Choose TTL limits appropriate for ephemeral sharing:

  ```env
  DEFAULT_TTL_SECONDS=3600
  MAX_TTL_SECONDS=2592000
  ```

## 2. Prepare persistent storage

- [ ] Confirm Redis/Valkey persistence is enabled and stored on a persistent volume.
- [ ] If using `STORAGE_BACKEND=local`, confirm `STORAGE_PATH` is mounted as persistent storage and is outside the public web root.
- [ ] If using `STORAGE_BACKEND=s3`, configure the bucket, endpoint, TLS, and least-privilege credentials before startup; do not expose S3 credentials to clients.
- [ ] For Docker Compose, verify named volumes exist or will be created:

  ```sh
  docker volume ls
  ```

- [ ] Confirm `app_storage` maps to `/app/data/storage` inside the app container when using local storage.
- [ ] Confirm storage is not served directly by the reverse proxy.
- [ ] Confirm disk monitoring/alerting exists for the host or volume backing `STORAGE_PATH`, or object-store monitoring exists for S3.

## 3. Configure reverse proxy and TLS

- [ ] Terminate HTTPS at Caddy, Nginx, Traefik, a cloud load balancer, or equivalent.
- [ ] Forward requests to the app service on the private network.
- [ ] Forward the original client IP with `X-Forwarded-For` so rate limiting works correctly.
- [ ] Set reverse proxy upload/body limits at or slightly above `MAX_FILE_SIZE` plus overhead.
- [ ] Set reasonable read/write timeouts for uploads and downloads.
- [ ] Verify `/healthz` and `/readyz` are reachable through the proxy.

## 4. Validate Docker Compose configuration

Run:

```sh
docker compose config
```

Check that:

- [ ] `APP_BASE_URL` is the public HTTPS URL.
- [ ] `SECURE_COOKIES` is `true`.
- [ ] `ENCRYPTION_MASTER_KEY` is set from a real secret.
- [ ] `REDIS_URL` points to the private Redis/Valkey service or managed Redis endpoint.
- [ ] `STORAGE_BACKEND` is explicitly set to `local` or `s3`.
- [ ] If using local storage, `STORAGE_PATH` points to the mounted storage location.
- [ ] `app_storage` and `redis_data` volumes are present or replaced by intentional production mounts.
- [ ] If using S3, `S3_ENDPOINT`, `S3_BUCKET`, credentials, and `S3_SECURE=true` are configured.

## 5. Build and start

Run:

```sh
docker compose build
docker compose up -d
```

Then verify:

```sh
docker compose ps
docker compose logs app
docker compose logs redis
```

- [ ] App container is healthy/running.
- [ ] Redis/Valkey container is healthy/running.
- [ ] App logs do not contain secrets, generated links, passphrases, file contents, or raw tokens.
- [ ] `/readyz` returns success.

## 6. Complete first-run admin setup

- [ ] Open `/setup` through HTTPS or a trusted private network.
- [ ] Create the first administrator account.
- [ ] Use a password with at least 12 characters.
- [ ] Store the administrator credentials securely.
- [ ] Confirm `/setup` redirects to `/login` after the administrator exists.
- [ ] Sign in and create additional users/admins if needed.

## 7. Configure optional integrations

### Email delivery

If direct link emails, upload request emails, or notifications are needed:

- [ ] Configure either SMTP or Microsoft Graph sendMail, not both unless you intentionally want Graph to take precedence.
- [ ] Use a dedicated sender mailbox or mail credential.
- [ ] Verify a test notification is delivered.
- [ ] Confirm mail credentials, Graph client secrets, access tokens, and generated links are not logged.
- [ ] Confirm users understand passphrases should be shared separately from emailed links.

### Microsoft Entra ID sign-in

If Microsoft login is enabled:

- [ ] Configure the backend API app registration and `access_as_user` scope.
- [ ] Configure the browser SPA app registration with redirect URI `<APP_BASE_URL>/login`.
- [ ] Grant API permissions and admin consent as needed.
- [ ] Configure group role mappings if access should be restricted.
- [ ] Test sign-in with a user and an administrator.

### Local Active Directory

- [ ] Test the configured LDAP/LDAPS endpoint and user bind from the app container before enabling local AD login. Prefer LDAPS in production.

## 8. Perform smoke tests

- [ ] Sign in as an administrator.
- [ ] Create a text link and reveal it once.
- [ ] Confirm a second reveal shows the gone page.
- [ ] Create a file link and download it once.
- [ ] Confirm a second download shows the gone page.
- [ ] Create a `Get file` upload request link.
- [ ] Upload a file through the request link.
- [ ] Download the received file from `My Links` once.
- [ ] Confirm audit logs show metadata-only events.
- [ ] Confirm active links appear correctly in Admin → Existing links.

## 9. Operational monitoring

- [ ] Monitor Redis/Valkey memory and persistence health.
- [ ] Monitor disk usage for the storage volume.
- [ ] Monitor app restarts and error logs.
- [ ] Monitor reverse proxy 4xx/5xx rates and upload/download timeouts.
- [ ] Periodically verify cleanup removes expired/orphaned encrypted files or unreferenced S3 objects.
- [ ] Rotate SMTP/Graph credentials according to your organization’s policy.

## 10. Backup and recovery policy

- [ ] Back up configuration secrets and deployment manifests.
- [ ] Back up `ENCRYPTION_MASTER_KEY` securely.
- [ ] Decide whether Redis/storage backups are appropriate for your threat model.
- [ ] If backing up Redis/storage, ensure retention does not preserve ephemeral payloads longer than policy allows.
- [ ] Document recovery steps and test them in a non-production environment.

## 11. Update procedure

Before updating:

- [ ] Review changes and migration notes.
- [ ] Confirm `ENCRYPTION_MASTER_KEY` will remain unchanged.
- [ ] Back up configuration secrets.
- [ ] Run `docker compose config`.

Deploy:

```sh
docker compose build
docker compose up -d
```

After updating:

- [ ] Check `docker compose ps`.
- [ ] Check app logs.
- [ ] Verify `/healthz` and `/readyz`.
- [ ] Run smoke tests for text, file, and get-file links.
