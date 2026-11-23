# UMC Auth

Optional self-hosted account and authentication service for Minecraft: Java Edition communities. It
provides a registration website and a Yggdrasil-compatible API for servers that use
[authlib-injector](https://github.com/yushijinhun/authlib-injector) or a launcher with external
authentication support.

This service is independent from the UMC launcher engine. A community can run it next to UMC or use
it with another launcher. It is written in Go, stores accounts in PostgreSQL, and is deployed behind
Nginx in a split Docker environment.

## Architecture

```text
auth.example.com    → Nginx → auth-api:8080 → PostgreSQL
account.example.com → Nginx → Vue panel + /api/ proxy → auth-api:8080
mail.example.com    → Stalwart (optional profile) → SMTP / IMAP / DKIM
```

Only Nginx publishes ports 80 and 443 for the account stack. The auth API and PostgreSQL stay on
internal Docker networks. The optional mail profile publishes 25, 465, 993, and 4190, and keeps its
management UI on loopback. The deployment environment lives in [`deploy/`](./deploy/). Its README
and the documentation pages cover certificates, environment variables, backup, and mail.

## What it provides

- Account registration with username and password
- A stable 32-character Minecraft UUID per account
- Argon2id password hashing
- A versioned `/api/v1` account API with short-lived access tokens and rotating refresh tokens
- Six-digit email verification and password reset codes with a retrying mail outbox
- An optional self-hosted Stalwart mail profile with SMTP, IMAPS, DKIM, and a management UI
- `user`, `moderator`, `admin`, and `superadmin` roles
- Opaque access tokens stored as SHA-256 hashes
- Yggdrasil `authenticate`, `refresh`, `validate`, `invalidate`, `signout`, and session `join` /
  `hasJoined` endpoints
- `GET /api/profiles/minecraft` and profile lookup endpoints
- Minecraft skins and capes with signed `textures` profile properties
- A game server registry and Yggdrasil join history for administrators
- Short-lived access tokens with HttpOnly, SameSite=Lax refresh cookies
- HTTPS through Nginx with operator-provided certificates
- PostgreSQL storage with a persistent Docker volume
- A persistent RSA signing key for profile-property signatures

## Requirements

- A Linux server with Docker and Docker Compose
- Two DNS names pointing at the server: auth and account
- A TLS certificate covering both names, or a wildcard certificate
- TCP ports 80 and 443 reachable
- For self-hosted mail: a mail hostname, TCP ports 25/465/993, and DNS/PTR control

For local development outside Docker you need Go 1.24+ and a PostgreSQL 17 database.

## Deploy

```sh
cd deploy
./scripts/setup.sh
# Edit AUTH_DOMAIN, ACCOUNT_DOMAIN, POSTGRES_PASSWORD, and BOOTSTRAP_TOKEN in .env
# Place fullchain.pem and privkey.pem in certs/
./scripts/build-web.sh
docker compose up -d --build
```

Open:

```text
https://account.example.com/
```

Register the first account at `/register`. If `BOOTSTRAP_TOKEN` is set, enter it on the registration
form. The first account is marked as a superadmin and bypasses the invite, CAPTCHA, and email
requirements so the instance can be bootstrapped. Remove `BOOTSTRAP_TOKEN` after setup.

The `account` host serves the Vue user center and admin panel. Build it with
`deploy/scripts/build-web.sh` before starting the stack. See [`deploy/README.md`](./deploy/README.md)
for certificates, host Nginx reuse, and verification.

To add self-hosted mail:

```sh
# Set MAIL_* and SMTP_* in .env, and ensure certs/ covers MAIL_HOSTNAME.
cd deploy
./scripts/provision-mail.sh
# Publish mail/dns-records.txt, then set EMAIL_VERIFICATION=required.
docker compose --profile mail up -d
docker compose up -d auth-api
```

The Stalwart management UI stays on loopback; use an SSH tunnel to
`https://127.0.0.1:8443/admin` (or `http://127.0.0.1:8080/admin` for setup/recovery). See
[`deploy/README.md`](./deploy/README.md#self-hosted-mail-stalwart).

The Yggdrasil API root is:

```text
https://auth.example.com/api/yggdrasil
```

Give that URL to authlib-injector or to a launcher that supports external authentication. For
example:

```text
-javaagent:authlib-injector.jar=https://auth.example.com/api/yggdrasil
```

For a Minecraft server, start it with the same agent and keep `online-mode=true`:

```sh
java -javaagent:authlib-injector.jar=https://auth.example.com/api/yggdrasil \
  -jar server.jar nogui
```

Players register on the website, then sign in through their launcher. The server verifies each
join through `hasJoined` and receives the account UUID.

## User center and admin UI

The first registered account is a superadmin. The Vue panel at `https://account.example.com` provides
the account dashboard, sessions, skin/cape editing, and an admin area for users, roles,
verification, password resets, invite codes, audit, game servers, join history, and the mail outbox.
Staff see read-only user and audit pages; administrators get the mutating actions.

The auth host is JSON-only: `/` returns service metadata, and page paths return JSON `404`. The
login and registration UI lives on `https://account.example.com/`. Invite codes are shown once;
only a SHA-256 hash is stored.

## Configuration

Deployment settings live in `deploy/.env`. The Compose file maps them to the application
environment; do not set `PUBLIC_ORIGIN`, `DATABASE_URL`, `COOKIE_SECURE`, or `TRUST_PROXY_HEADERS`
independently.

| Variable | Default | Meaning |
|---|---|---|
| `AUTH_DOMAIN` | required | Public auth hostname used by Nginx and `PUBLIC_ORIGIN` |
| `ACCOUNT_DOMAIN` | required | Vue user center and admin panel hostname |
| `POSTGRES_PASSWORD` | required | PostgreSQL password, also passed to the auth service |
| `SERVER_NAME` | `UMC Auth` | Name shown in the UI and Yggdrasil metadata |
| `BOOTSTRAP_TOKEN` | unset | If set, the first account must provide this token |
| `REGISTRATION_MODE` | `invite` | `invite`, `open`, or `closed` |
| `EMAIL_VERIFICATION` | `off` | `off` or `required`; required accounts must verify an email |
| `CAPTCHA_MODE` | `none` | `none`, `pow`, or `turnstile` |
| `POW_DIFFICULTY` | `16` | Leading zero bits for proof-of-work (12–24) |
| `REGISTER_LIMIT_PER_HOUR` | `3` | Registrations per IPv4 address or IPv6 /64 per hour |
| `REGISTER_LIMIT_PER_DAY` | `10` | Registrations per IPv4 address or IPv6 /64 per day |
| `REGISTER_LIMIT_GLOBAL_PER_HOUR` | `100` | Global registration budget per hour |
| `LOGIN_LIMIT_PER_MINUTE` | `10` | Sign-in and Yggdrasil authentication attempts per address per minute |
| `POW_CHALLENGE_LIMIT_PER_MINUTE` | `30` | Proof-of-work challenges per IPv4 address or IPv6 /64 per minute |
| `PROFILE_LOOKUP_LIMIT_PER_MINUTE` | `60` | Bulk profile lookups per IPv4 address or IPv6 /64 per minute |
| `TURNSTILE_SITE_KEY` / `TURNSTILE_SECRET_KEY` | unset | Required for `CAPTCHA_MODE=turnstile`; browsers must reach Cloudflare |
| `SMTP_HOST` / `SMTP_PORT` | unset / `587` | SMTP server for verification email |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | unset | SMTP authentication when required |
| `SMTP_FROM` | unset | Sender address, for example `UMC Auth <no-reply@example.com>` |
| `SMTP_TLS` | `starttls` | `starttls`, `implicit`, or `none`; Stalwart uses `implicit` on 465 |
| `MAIL_DOMAIN` | unset | Email domain for the Stalwart profile, for example `example.com` |
| `MAIL_HOSTNAME` | unset | Mail server hostname and Docker alias, for example `mail.example.com` |
| `MAIL_ADMIN_PASSWORD` | unset | Bootstrap password for `provision-mail.sh` |
| `MAIL_NO_REPLY_ADDRESS` | unset | Sender account, for example `no-reply@example.com` |
| `MAIL_NO_REPLY_PASSWORD` | unset | SMTP password for the no-reply account; keep equal to `SMTP_PASSWORD` |
| `SESSION_DAYS` | `30` | Web session lifetime |
| `TOKEN_DAYS` | `30` | Yggdrasil access-token lifetime |
| `ACCESS_TOKEN_MINUTES` | `15` | `/api/v1` access-token lifetime |
| `REFRESH_TOKEN_DAYS` | `30` | `/api/v1` refresh-token lifetime |
| `FORGOT_LIMIT_PER_MINUTE` | `5` | Password reset requests per address |
| `VERIFY_LIMIT_PER_MINUTE` | `10` | Email verification and resend requests |
| `REFRESH_LIMIT_PER_MINUTE` | `60` | Access-token refresh requests |
| `UPLOAD_DIR` | `/data/uploads` in Compose | Skin and cape storage; the `auth-uploads` volume is mounted at `/data` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

The service also accepts `BIND_ADDR` and `DATABASE_URL` directly when run outside Compose.

## Security notes

- **CAPTCHA choices:** `pow` is self-hosted and works without third-party connectivity.
  `turnstile` is usually invisible but depends on the browser reaching Cloudflare, which may be
  unreliable on some networks. Turnstile tokens are browser-only; API clients should use an invite
  code or `pow`.
- **HTTPS is required.** Nginx terminates TLS, redirects HTTP, sets HSTS, and keeps the auth
  container port private. Do not publish the `auth-api` port directly. The service refuses to start
  with an `http://` PUBLIC_ORIGIN unless `ALLOW_INSECURE_HTTP=true` is set for local development.
- Passwords are hashed with Argon2id; access tokens and web session tokens are stored only as
  SHA-256 hashes.
- Refresh cookies are HttpOnly and SameSite=Lax; API clients use Bearer access tokens. The legacy
  HTML forms are no longer routed, so the auth host serves no browser UI.
- Responses include `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`,
  `Permissions-Policy`, and a restrictive Content-Security-Policy.
- The auth container runs as UID 10001 with `no-new-privileges` and a read-only root filesystem.
  PostgreSQL and the `auth-uploads` texture volume are the persistent stores.
- Uploads are validated as PNGs with fixed skin/cape dimensions and a 1 MB per-file limit.
- Back up the `auth-db` volume. It contains password hashes, token hashes, and the RSA private key
  used to sign profile `textures` properties; treat it and `auth-uploads` as sensitive.

## Account API v1

The JSON account API for the Vue panel, launchers, and admin clients lives under `/api/v1`:

```text
POST /api/v1/auth/register
POST /api/v1/auth/verify-email
POST /api/v1/auth/verify-email/resend
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
POST /api/v1/auth/password/forgot
POST /api/v1/auth/password/reset
GET  /api/v1/users/me
GET  /api/v1/users/me/sessions
DELETE /api/v1/users/me/sessions/{id}
GET  /api/v1/users/me/minecraft
POST /api/v1/users/me/skin
DELETE /api/v1/users/me/skin
POST /api/v1/users/me/cape
DELETE /api/v1/users/me/cape
GET  /api/v1/profiles/{uuid}
GET  /api/v1/profiles/by-username/{name}
GET  /textures/{skin|cape}/{uuid}/{sha256}
```

Admin endpoints under `/api/v1/admin/` cover user search, status, roles, manual verification,
password reset, audit search, the mail outbox, game servers, join history, and statistics. The
canonical contract and OpenAPI document are in [`protocols/auth-v1.md`](protocols/auth-v1.md)
and [`protocols/auth-v1.yaml`](protocols/auth-v1.yaml).

Access tokens default to 15 minutes. Refresh tokens default to 30 days, rotate on every use, and
revoke the whole session family when reused. Web clients receive the refresh token as an HttpOnly
cookie; other clients receive it in JSON.

## API surface

Metadata:

```text
GET /api/yggdrasil
```

Yggdrasil:

```text
POST /api/yggdrasil/authserver/authenticate
POST /api/yggdrasil/authserver/refresh
POST /api/yggdrasil/authserver/validate
POST /api/yggdrasil/authserver/invalidate
POST /api/yggdrasil/authserver/signout
POST /api/yggdrasil/sessionserver/session/minecraft/join
GET  /api/yggdrasil/sessionserver/session/minecraft/hasJoined
GET  /api/yggdrasil/sessionserver/session/minecraft/profile/{uuid}
GET  /api/yggdrasil/api/user/profile/{uuid}
POST /api/yggdrasil/api/profiles/minecraft
```

Account panel on `account.example.com`:

```text
GET  /register
GET  /login
GET  /dashboard
GET  /verify-email
GET  /admin
GET  /admin/users
GET  /admin/invites
GET  /admin/servers
GET  /admin/audit
GET  /admin/mail
```

The auth host keeps only JSON endpoints (`/`, `/api/v1`, `/api/yggdrasil`), `/textures/`, and
`/healthz`; other paths return JSON `404`.

The API is intended to be reached over HTTPS. Health check:

```text
GET /healthz
```

## Data and backup

Accounts and Minecraft profiles live in the `auth-db` PostgreSQL volume. Uploaded skins and capes
live in the `auth-uploads` volume. From `deploy/`, back up the database with `pg_dump`
while the API is stopped:

```sh
docker compose stop auth-api
docker compose exec -T db pg_dump -U umc_auth -d umc_auth -Fc > umc-auth.dump
docker run --rm -v umc-auth_auth-uploads:/data -v "$PWD":/backup \
  alpine tar czf /backup/auth-uploads.tar.gz -C /data .
docker compose start auth-api
```

Check `docker volume ls` if the Compose project name is not `umc-auth`.

Restore:

```sh
docker compose stop auth-api
docker compose exec -T db pg_restore -U umc_auth -d umc_auth --clean --if-exists < umc-auth.dump
docker compose start auth-api
```

TLS files in `deploy/certs` are also operator state. Back them up, or keep the certificate issuance
process reproducible.

## Deployment check

From the repository root:

```sh
AUTH_DOMAIN=auth.example.com ACCOUNT_DOMAIN=account.example.com ./scripts/check-deployment.sh
```

It verifies HTTP-to-HTTPS redirects, certificate trust and expiry, HSTS, the health endpoint,
JSON metadata, the JSON service document and page-route `404`, that the auth port is not exposed
directly, and, when `ACCOUNT_DOMAIN` is set, the Vue panel headers, login page, API proxy, and SPA
deep-link fallback. Run it from a machine that can resolve the domain.

## Development

Start a development database:

```sh
docker run --rm -d --name umc-auth-db \
  -e POSTGRES_DB=umc_auth \
  -e POSTGRES_USER=umc_auth \
  -e POSTGRES_PASSWORD=devpassword \
  -p 5432:5432 postgres:17-alpine
```

Run the service with Go 1.24+:

```sh
PUBLIC_ORIGIN=http://127.0.0.1:8080 \
ALLOW_INSECURE_HTTP=true \
COOKIE_SECURE=false \
BIND_ADDR=127.0.0.1:8080 \
DATABASE_URL="host=127.0.0.1 port=5432 user=umc_auth dbname=umc_auth sslmode=disable" \
PGPASSWORD=devpassword \
go run ./cmd/umc-auth
```

Tests:

```sh
./scripts/check.sh
```

The PostgreSQL integration test runs when `TEST_DATABASE_URL` is set:

```sh
TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=umc_auth dbname=umc_auth sslmode=disable" \
PGPASSWORD=devpassword go test -run TestStoreIntegration ./...
```

## Current limitations

- Email verification is optional.
- No email change or self-service account deletion.
- The Vue panel covers the user center and admin workflows, including skins, capes, invites,
  servers, join history, audit, and the mail outbox.
- Rate limiting is in-process per auth container. If you run multiple replicas, add a shared limiter
  or edge proxy.
- Self-hosted mail is implemented and locally tested; external delivery still needs real DNS and PTR
  records, and some VPS providers block outbound TCP 25.
- The UMC launcher's Microsoft sign-in route is separate; this service does not implement
  client-side third-party authentication.
