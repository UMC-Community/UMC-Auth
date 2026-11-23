# UMC Auth web panel

Vue 3 + Vue Router + Vite user center and admin panel for UMC Auth. Nginx serves the built files at
`account.example.com` and proxies `/api/` to the Go auth service.

## Build

From `deploy/`:

```sh
./scripts/build-web.sh
```

This uses the `node:22-alpine` image and writes `web/dist` as your host user. With Node 22+ on the
host:

```sh
npm ci
npm run build
```

## Develop

```sh
npm ci
npm run dev
```

Vite proxies `/api` to `http://127.0.0.1:8080`. Run the Go service locally with:

```sh
PUBLIC_ORIGIN=http://127.0.0.1:8080 \
ALLOW_INSECURE_HTTP=true \
COOKIE_SECURE=false \
BIND_ADDR=127.0.0.1:8080 \
DATABASE_URL="host=127.0.0.1 port=5432 user=umc_auth dbname=umc_auth sslmode=disable" \
PGPASSWORD=devpassword \
go run ./cmd/umc-auth
```

## Authentication model

- The access token is kept in memory only.
- The refresh token is an HttpOnly cookie scoped to `/api/v1/auth`.
- A non-sensitive `umc_auth_session_hint` cookie avoids refresh calls on guest pages.
- The API base is same-origin `/api/v1`, so no CORS configuration is required.

## Pages

- Register, email verification, login, forgot/reset password
- Dashboard, sessions, profile with skin and cape upload
- Admin overview, users, invite codes, audit log with search, game servers and join history, mail outbox

The panel follows the system light/dark preference. Login and registration use a standalone,
centered card; the rest of the panel shares the same neutral color tokens.

The interface supports English and Chinese. It detects Chinese browsers on first visit, remembers
the choice in `localStorage`, and exposes an `EN / 中文` switch in the header and on auth pages.

The canonical API contract is [`protocols/auth-v1.md`](../protocols/auth-v1.md).
