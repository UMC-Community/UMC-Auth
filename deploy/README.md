# UMC Auth deployment environment

This directory is the Docker deployment environment for UMC Auth. Nginx terminates TLS and is the
only public entrypoint; the Go auth service and PostgreSQL stay on internal Docker networks. An
optional Stalwart mail container provides SMTP, IMAP, DKIM, and a management UI without changing the
account core.

The `/api/v1` account API, PostgreSQL migrations, Yggdrasil endpoints, sessions, email codes,
password reset, admin API, self-hosted Stalwart mail profile, Vue 3 account panel, Minecraft skins
and capes, signed texture properties, and server/join administration are implemented. Email change
and OAuth clients are planned. The full architecture, environment variables, data model, mail
requirements, texture storage, and resource budget are in
[English deployment guide](https://umc-community.github.io/Docs/en/UMC-Auth-deployment) and
[简体中文部署指南](https://umc-community.github.io/Docs/zh/UMC-Auth-deployment).

## Status

| Part | Status |
|---|---|
| Nginx entry, HTTPS, security headers, edge rate limits | Environment scaffold |
| Go auth API, PostgreSQL, Yggdrasil endpoints, admin UI | Implemented |
| `/api/v1` account API, migrations, sessions, email codes, password reset, admin API | Implemented |
| `account.example.com` | Vue 3 user center and admin panel |
| Self-hosted mail (Stalwart, optional profile) | Implemented; provisioning and local delivery tested |
| Vue 3 user center | Implemented; build before starting Nginx |

## Layout

```text
deploy/
├── compose.yaml
├── .env.example
├── nginx/
│   ├── nginx.conf
│   └── templates/default.conf.template
├── certs/              # fullchain.pem and privkey.pem (not committed)
├── acme/               # optional ACME HTTP-01 webroot (not committed)
├── mail/               # provisioning output and admin credentials (not committed)
└── scripts/
    ├── setup.sh
    ├── provision-mail.sh
    └── build-web.sh
```

The Vue source lives in [`../web/`](../web/) and builds to `../web/dist`, which Nginx mounts at
`/usr/share/nginx/html/account`.

The `auth-api` build context is the parent directory, which is the UMC-Auth repository root. Keep
the repository checkout on the server, or build and publish the image
separately and change `build:` to `image:`.

## Quick start

```sh
git clone https://github.com/UMC-Community/UMC-Auth.git /opt/UMC-Auth
cd /opt/UMC-Auth/deploy
./scripts/setup.sh
$EDITOR .env
# copy or issue a certificate covering AUTH_DOMAIN and ACCOUNT_DOMAIN
./scripts/build-web.sh
docker compose up -d --build
docker compose ps
```

Required DNS:

| Record | Example |
|---|---|
| `A` / `AAAA` | `auth.example.com` → server address |
| `A` / `AAAA` | `account.example.com` → server address |

Open TCP 80 and 443. Do not open 8080 or 5432.

Place certificates as:

```text
certs/fullchain.pem
certs/privkey.pem
```

A certificate with both names as SANs or a wildcard certificate works. The default layout expects two
names: the auth host is JSON-only for API clients, and the account host serves the Vue panel.
Keeping both entries with the same name produces a duplicate `server_name`.

## Verify

```sh
curl -fsS https://auth.example.com/healthz
curl -fsSI https://auth.example.com/api/yggdrasil
curl -fsSI https://account.example.com/ | head
```

`/healthz` returns `ok`, the metadata endpoint returns JSON, and the account host serves the Vue
panel. Register the first account at `https://account.example.com/register`. With `BOOTSTRAP_TOKEN`
set, the first account becomes a superadmin. Remove the token after setup.

The deployment check runs from the parent directory:

```sh
AUTH_DOMAIN=auth.example.com ACCOUNT_DOMAIN=account.example.com ../scripts/check-deployment.sh
```

It expects the auth host to return JSON for `/` and JSON `404` for page routes, and checks the
account panel CSP and SPA fallback. Nginx adds HSTS.

## Web panel

The Vue 3 source is in [`../web/`](../web/). Build it before starting Nginx:

```sh
./scripts/build-web.sh
# or, with Node 22+ on the host:
cd ../web && npm ci && npm run build
```

The script uses the `node:22-alpine` image and writes `../web/dist` as your host user. Nginx mounts
that directory at `/usr/share/nginx/html/account` and proxies `/api/` to `auth-api:8080`, so the
refresh cookie stays first-party to `account.example.com`.

The panel keeps the short-lived access token in memory and restores the session from the HttpOnly
refresh cookie on page load. It does not use localStorage for tokens.

Local development:

```sh
cd ../web && npm ci && npm run dev
# Vite proxies /api to http://127.0.0.1:8080
```

Run the Go service locally with `PUBLIC_ORIGIN=http://127.0.0.1:8080`,
`ALLOW_INSECURE_HTTP=true`, and `COOKIE_SECURE=false` while developing.

## Reuse an existing host Nginx

If the host already terminates TLS, remove the `nginx` service, publish the API on loopback, and
keep `TRUST_PROXY_HEADERS=true`:

```yaml
  auth-api:
    ports:
      - "127.0.0.1:8080:8080"
```

Then copy the proxy rules from `nginx/templates/default.conf.template` into the host configuration.
Never publish `8080` on a public interface.

## Self-hosted mail (Stalwart)

The `mail` profile runs Stalwart v1 with SMTP on 25, TLS submission on 465, IMAPS on 993,
ManageSieve on 4190, and the management UI on `127.0.0.1:8080`. It is attached to `edge` for outbound
SMTP and to `internal` with a `${MAIL_HOSTNAME}` alias so the auth API can reach it with the same
hostname as the TLS certificate.

Enable it:

```sh
# 1. Set MAIL_* and SMTP_* in .env. Keep SMTP_PASSWORD equal to
#    MAIL_NO_REPLY_PASSWORD, and make sure certs/ covers MAIL_HOSTNAME.
# 2. Provision the domain, DKIM, administrator, no-reply account, and certificate.
./scripts/provision-mail.sh
# 3. Publish the records in mail/dns-records.txt (A, MX, SPF, DKIM, DMARC, PTR).
# 4. Turn on email verification.
$EDITOR .env   # EMAIL_VERIFICATION=required
docker compose --profile mail up -d
docker compose up -d auth-api
```

`provision-mail.sh`:

- requires Python 3 and OpenSSL on the host
- starts Stalwart in bootstrap mode and waits for `/healthz/live`
- completes `Bootstrap` with the configured hostname and domain, with automatic DKIM keys
- stores the generated administrator credentials in `mail/admin-credentials.txt` (mode 600)
- creates or updates `no-reply@${MAIL_DOMAIN}` with `MAIL_NO_REPLY_PASSWORD`
- installs `certs/fullchain.pem` and `certs/privkey.pem` as the default TLS certificate
- recreates the container without the bootstrap administrator
- writes the generated DNS zone to `mail/dns-records.txt`

Stalwart downloads its WebUI bundle from GitHub on first start. If the host cannot reach GitHub, use
the `mail-cli` service or allow outbound HTTPS.

The management UI is intentionally loopback-only. Use an SSH tunnel:

```sh
# Normal administration (container 443 mapped to host 8443):
ssh -L 8443:127.0.0.1:8443 <user>@<server>
# open https://127.0.0.1:8443/admin

# Setup or recovery API (container 8080):
ssh -L 8080:127.0.0.1:8080 <user>@<server>
# open http://127.0.0.1:8080/admin
```

| Port | Purpose | Exposure |
|---|---|---|
| 25 | Inbound SMTP | Public |
| 465 | SMTP submission with implicit TLS | Public |
| 993 | IMAP with implicit TLS | Public |
| 4190 | ManageSieve | Public; block if unused |
| 8080 | Setup/recovery HTTP | Loopback only |
| 8443 | Management UI HTTPS (container 443) | Loopback only |

The auth service uses:

```ini
SMTP_HOST=mail.example.com
SMTP_PORT=465
SMTP_USERNAME=no-reply@example.com
SMTP_PASSWORD=<MAIL_NO_REPLY_PASSWORD>
SMTP_FROM=UMC Auth <no-reply@example.com>
SMTP_TLS=implicit
```

The Docker alias makes `mail.example.com` resolve to the mail container on the internal network, so
certificate verification matches the public certificate.

DNS and deliverability:

- The generated zone includes MX, SPF, DKIM, DMARC, MTA-STS, SRV, and autoconfig records.
- Add `A mail` and the matching `PTR` at the hosting provider.
- Confirm that outbound TCP 25 is not blocked.
- Self-hosted mail does not guarantee inbox delivery to Gmail, Outlook, or QQ Mail. Reputation, PTR,
  SPF, DKIM, and DMARC all matter.
- Local SMTP submission, IMAP delivery, DKIM signing, and the auth API → Stalwart outbox path have
  been tested. External delivery still needs real DNS and PTR records.

## Backup

The account stack stores data in the `auth-db` volume. With the mail profile enabled, Stalwart also
uses `stalwart-data` for mailboxes and `stalwart-etc` for its configuration.

Database:

```sh
docker compose stop auth-api
docker compose exec -T db pg_dump -U umc_auth -d umc_auth -Fc > umc-auth-$(date +%F).dump
docker compose start auth-api
```

Mail volumes, while the mail container is stopped:

```sh
docker compose --profile mail stop mail
docker run --rm -v umc-auth_stalwart-data:/data -v "$PWD":/backup \
  stalwartlabs/stalwart:v1 tar czf /backup/stalwart-data-$(date +%F).tar.gz -C /data .
docker run --rm -v umc-auth_stalwart-etc:/data -v "$PWD":/backup \
  stalwartlabs/stalwart:v1 tar czf /backup/stalwart-etc-$(date +%F).tar.gz -C /data .
docker compose --profile mail start mail
```

The volume names assume the default Compose project name `umc-auth`. If you override the project
name, adjust the `umc-auth_` prefix.

`certs/privkey.pem`, `certs/ca-key.pem` if present, `mail/admin-credentials.txt`, the mail volumes,
and the database dump are sensitive. Do not commit them; `.gitignore` covers `.env`, `certs/*`,
`acme/*`, and `mail/*`.

## Next phases

| Phase | Scope |
|---|---|
| 3 | Self-hosted mail: Stalwart container, SMTP, DNS, delivery tests | Implemented; external DNS/PTR pending |
| 4 | Vue 3 user center and admin panel on `account.example.com` | Implemented |
| 5 | Minecraft profiles, skins/capes, signed textures, server registry, join history, audit search | Implemented |
| 6 | Launcher/server integration and release hardening | Next |

Skins and capes are stored on the `auth-uploads` volume mounted at `/data`; include it in backups
alongside `auth-db`.

The `/api/v1` contract is in [`protocols/auth-v1.md`](../protocols/auth-v1.md) and
[`protocols/auth-v1.yaml`](../protocols/auth-v1.yaml).
