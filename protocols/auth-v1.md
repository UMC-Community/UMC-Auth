# UMC Auth API v1

Status: implemented by [UMC-Auth](https://github.com/UMC-Community/UMC-Auth). This document is the
canonical account API contract for the Vue user center, launchers, and administrative clients. The Minecraft-compatible Yggdrasil API stays
under `/api/yggdrasil` and is documented separately.

## Base URL

```text
https://auth.example.com/api/v1
```

The account panel may call `/api/v1` on `account.example.com`; Nginx proxies that path to the same
service so browser cookies remain first-party. Do not send these requests cross-origin.

The auth host is JSON-only. `GET /` returns service metadata:
`{"service":"UMC Auth","api":"...","yggdrasil":"..."}`. Page paths such as `/login` return a JSON
`404`; the browser UI is served only from the account host.

## Conventions

- JSON over HTTPS, UTF-8, `Content-Type: application/json`.
- Field names use `snake_case`.
- Timestamps are Unix seconds (UTC).
- Player IDs are 32 lowercase hexadecimal characters without dashes. Responses include both `id` and
  `uuid` with the same value for compatibility.
- Unknown JSON fields are rejected with `400 invalid_json`.
- Every error uses the same shape:

```json
{
  "error": {
    "code": "invalid_credentials",
    "message": "Invalid login or password"
  }
}
```

## Authentication

### API clients

`POST /api/v1/auth/login`, `/register` (when verification is off), and `/verify-email` return:

```json
{
  "access_token": "opaque-token",
  "refresh_token": "opaque-token",
  "token_type": "Bearer",
  "expires_in": 900,
  "refresh_expires_in": 2592000,
  "user": {},
  "session": {}
}
```

Send the access token as:

```http
Authorization: Bearer opaque-token
```

Access tokens default to 15 minutes (`ACCESS_TOKEN_MINUTES`). Refresh tokens default to 30 days
(`REFRESH_TOKEN_DAYS`) and rotate on every use. Only SHA-256 hashes are stored.

### Web clients

Send `"client": "web"` on login, register, or refresh. The refresh token is set as an HttpOnly cookie
instead of being returned in JSON:

- Name: `umc_auth_refresh`
- Path: `/api/v1/auth`
- `HttpOnly`, `Secure` on HTTPS, `SameSite=Lax`
- Lifetime: `REFRESH_TOKEN_DAYS`

The browser stores the short-lived access token in memory and sends it as a Bearer token. On reload it
calls `POST /api/v1/auth/refresh` with no body; the cookie is used.

### Refresh-token reuse

Refresh tokens rotate. Presenting an already-used refresh token revokes the whole session family and
returns `401 invalid_grant`. A new login is required.

## Endpoints

### Server metadata

`GET /api/v1/meta`

Public configuration used by the account panel before authentication:

```json
{
  "server_name": "UMC Auth",
  "registration_mode": "invite",
  "registration_open": true,
  "first_account": false,
  "bootstrap_required": false,
  "email_verification": "required",
  "captcha_mode": "pow",
  "turnstile_site_key": "",
  "auth_origin": "https://auth.example.com",
  "yggdrasil_url": "https://auth.example.com/api/yggdrasil"
}
```

`registration_open` is false only when registration is closed and an account already exists.
`first_account` is true while the instance has no accounts. `bootstrap_required` is true only when
the first account must supply `BOOTSTRAP_TOKEN`.

### Register

`POST /api/v1/auth/register`

```json
{
  "username": "Player_1",
  "email": "player@example.com",
  "password": "at-least-8-characters",
  "invite_code": "UMC-XXXX-XXXX-XXXX",
  "bootstrap_token": "",
  "turnstile_token": "",
  "pow_token": "",
  "pow_nonce": "",
  "client": "web"
}
```

- Username: 3–16 ASCII letters, digits, or underscores.
- Password: 8–128 characters.
- Email is required and normalized to lowercase.
- `invite_code` is required when `REGISTRATION_MODE=invite`.
- `bootstrap_token` is required for the first account when `BOOTSTRAP_TOKEN` is set.
- CAPTCHA fields are required when the instance enables Turnstile or proof-of-work.
- The first account becomes a superadmin and is verified immediately.

When email verification is required:

```http
HTTP/1.1 202 Accepted
```

```json
{
  "verification_required": true,
  "message": "Check your email for a six-digit verification code",
  "user": { "email_verified": false, "status": "pending" }
}
```

Otherwise the response is `201 Created` with the token response above.

Duplicate usernames and emails return the same `409 registration_conflict` response so the endpoint
does not reveal which field matched.

### Verify email

`POST /api/v1/auth/verify-email`

```json
{"email": "player@example.com", "code": "123456", "client": "web"}
```

Codes are six digits, valid for 10 minutes, and allow five attempts. Success marks the email verified,
activates the account, and returns the token response.

### Resend verification

`POST /api/v1/auth/verify-email/resend`

```json
{"email": "player@example.com"}
```

Always returns `202 Accepted` when the request is valid, whether or not the address exists. A second
request within 60 seconds returns `429 resend_too_soon`.

### Login

`POST /api/v1/auth/login`

```json
{"login": "Player_1", "password": "secret", "client": "web"}
```

`login` accepts a username or an email address. Disabled, locked, and unverified accounts are
rejected after the password is verified.

Ten failed attempts for the same IP and login within 15 minutes return `429 account_locked` until the
window expires.

### Refresh

`POST /api/v1/auth/refresh`

```json
{"refresh_token": "opaque-token", "client": "api"}
```

For web clients, omit the body and rely on the `umc_auth_refresh` cookie. A successful refresh
invalidates the old refresh token and returns a new access token. Web responses set a new cookie and
omit `refresh_token` from JSON.

### Logout

`POST /api/v1/auth/logout`

```json
{"refresh_token": "opaque-token"}
```

The refresh token may also come from the cookie, or the request may carry a Bearer access token.
The session is revoked and the refresh cookie is cleared. The endpoint is idempotent and returns
`204 No Content`.

### Forgot password

`POST /api/v1/auth/password/forgot`

```json
{"email": "player@example.com"}
```

Always returns `202 Accepted` for a valid request. If the address exists, is verified, and is not
disabled, a six-digit reset code is sent. Codes are valid for 10 minutes, allow five attempts, and
are stored as hashes.

### Reset password

`POST /api/v1/auth/password/reset`

```json
{
  "email": "player@example.com",
  "code": "654321",
  "new_password": "new-at-least-8-characters"
}
```

A successful reset revokes every session and Yggdrasil token for the account. Returns
`204 No Content`.

### Current user

`GET /api/v1/users/me`

Requires a Bearer access token.

```json
{
  "user": {
    "id": "32-hex",
    "uuid": "32-hex",
    "username": "Player_1",
    "email": "player@example.com",
    "email_verified": true,
    "role": "user",
    "status": "active",
    "created_at": 1790000000,
    "updated_at": 1790000000
  },
  "session": {
    "id": "session-public-id",
    "client": "web",
    "ip": "203.0.113.10",
    "user_agent": "Mozilla/5.0",
    "created_at": 1790000000,
    "last_used_at": 1790000000,
    "expires_at": 1792592000,
    "current": true
  }
}
```

### Sessions

`GET /api/v1/users/me/sessions`

Returns active sessions for the current account, newest first.

`DELETE /api/v1/users/me/sessions/{id}`

Revokes a session by its public `id`. Revoking the current session clears the refresh cookie. Returns
`204 No Content` or `404 not_found`.

### Profiles

`GET /api/v1/profiles/{uuid}`

`GET /api/v1/profiles/by-username/{name}`

Public profile lookup with the same per-IP limit as Yggdrasil profile lookups.

```json
{"id": "32-hex", "name": "Player_1", "properties": []}
```

When a skin or cape is set, `properties` contains a signed `textures` entry in the same format used
by the Yggdrasil profile endpoints:

```json
{
  "name": "textures",
  "value": "base64-json",
  "signature": "base64-rsa-sha256"
}
```

The decoded `value` contains `timestamp`, `profileId`, `profileName`, and a `textures` object with
`SKIN` and/or `CAPE` URLs plus `metadata.model` (`classic` or `slim`) for skins. The signature is
RSA PKCS#1 v1.5 over SHA-256 of the base64 `value` string, using the service signing key. The
matching public key is available at `/api/yggdrasil` metadata.

### Minecraft profile

`GET /api/v1/users/me/minecraft`

```json
{
  "profile": {
    "uuid": "32-hex",
    "name": "Player_1",
    "skin_url": "https://auth.example.com/textures/skin/32-hex/sha256-hex",
    "cape_url": "",
    "skin_model": "classic",
    "updated_at": 1790000000
  }
}
```

`POST /api/v1/users/me/skin`

`multipart/form-data` with `file` (PNG, at least 64×64 or 64×32 in either order, at most 1 MB) and
optional `model` (`classic` or `slim`, default `classic`). The body limit is 2 MB. Returns the
updated profile.

`DELETE /api/v1/users/me/skin`

Removes the current skin. Returns the updated profile.

`POST /api/v1/users/me/cape`

`multipart/form-data` with `file` (64×32 or 64×64 PNG, at most 1 MB). Returns the updated profile.

`DELETE /api/v1/users/me/cape`

Removes the current cape. Returns the updated profile.

Texture files are public and immutable at
`GET /textures/{skin|cape}/{uuid}/{sha256}`; responses are served with `image/png` and long-lived
cache headers. Uploading a new file changes the URL, so clients never see stale textures.

## Admin API

All admin endpoints require staff access. A `moderator` may list users and read the audit log;
`user` status, role, manual verification, password reset, mail, and statistics endpoints require
`admin` or `superadmin`. Roles are `user`, `moderator`, `admin`, and `superadmin`.

- An administrator cannot change their own role or status.
- Only a `superadmin` can modify another administrator, grant `admin`, or manage `superadmin`.
- Every change is written to the audit log.

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/admin/users?query=&limit=&offset=` | Search username, email, or UUID |
| `GET /api/v1/admin/users/{uuid}` | Inspect one account |
| `POST /api/v1/admin/users/{uuid}/status` | `{"disabled": true}` to ban or unban |
| `POST /api/v1/admin/users/{uuid}/role` | `{"role": "moderator"}` |
| `POST /api/v1/admin/users/{uuid}/verify-email` | Mark the email verified |
| `POST /api/v1/admin/users/{uuid}/reset-password` | Set `new_password`, or generate one and return `generated_password` once |
| `GET /api/v1/admin/audit?query=&limit=` | Recent audit entries, optionally filtered by actor, action, or detail |
| `GET /api/v1/admin/mail?status=&limit=` | Mail outbox metadata (message bodies are not returned) |
| `POST /api/v1/admin/mail/{id}/retry` | Reset a failed message and retry it |
| `GET /api/v1/admin/stats` | User and mail counters |
| `GET /api/v1/admin/invites?limit=` | Invite codes with status, uses, and expiry |
| `POST /api/v1/admin/invites` | `{"count", "uses", "days", "note"}`; returns plaintext `codes` once |
| `POST /api/v1/admin/invites/{id}/revoke` | Revoke an invite code |
| `GET /api/v1/admin/servers?limit=` | Registered game servers |
| `POST /api/v1/admin/servers` | `{"name", "address", "description"}` to register a server |
| `POST /api/v1/admin/servers/{id}/status` | `{"status": "active" \| "disabled"}` |
| `DELETE /api/v1/admin/servers/{id}` | Delete a server; join history is kept |
| `GET /api/v1/admin/servers/joins?server_id=&limit=` | Recent Yggdrasil join records |

Server management requires `admin` or `superadmin`. Join records are written by the Yggdrasil
`join` endpoint and contain the username, server id, client IP, and timestamp.

## Rate limits

| Scope | Default |
|---|---|
| Registration | `REGISTER_LIMIT_PER_HOUR`, `REGISTER_LIMIT_PER_DAY`, global hourly budget |
| Login | `LOGIN_LIMIT_PER_MINUTE` |
| Refresh | `REFRESH_LIMIT_PER_MINUTE` |
| Verification and resend | `VERIFY_LIMIT_PER_MINUTE` |
| Forgot and reset | `FORGOT_LIMIT_PER_MINUTE` |
| Profile lookup | `PROFILE_LOOKUP_LIMIT_PER_MINUTE` |

Limits are in-process per auth container. Nginx also applies edge limits. When a limit is hit, the
response is `429` and may include `Retry-After` in seconds.

## Error codes

| Code | Status | Meaning |
|---|---|---|
| `invalid_json` | 400 | Malformed JSON or unknown fields |
| `invalid_username` | 400 | Username failed validation |
| `invalid_email` | 400 | Email is malformed |
| `weak_password` | 400 | Password failed validation |
| `invalid_code` | 400 | Verification or reset code is wrong or expired |
| `invalid_texture` | 400 | Uploaded skin or cape is not a valid PNG or has the wrong dimensions |
| `invalid_model` | 400 | Skin model is not `classic` or `slim` |
| `invalid_credentials` | 401 | Login or password is wrong |
| `unauthorized` | 401 | Missing or invalid Bearer token |
| `invalid_grant` | 401 | Refresh token is invalid, expired, or reused |
| `forbidden` | 403 | Role or ownership check failed |
| `account_disabled` | 403 | Account is disabled |
| `account_locked` | 429 | Too many failed sign-ins |
| `email_not_verified` | 403 | Email verification is required |
| `registration_conflict` | 409 | Username or email is already registered |
| `server_conflict` | 409 | A game server with that name already exists |
| `registration_closed` | 403 | Registration is disabled |
| `invite_required` / `invalid_invite` | 403 | Invite code missing or invalid |
| `bootstrap_required` | 403 | First-account bootstrap token is wrong |
| `captcha_failed` | 400 | Turnstile or proof-of-work failed |
| `email_unavailable` | 503 | SMTP is not configured |
| `too_many_requests` | 429 | Rate limit reached |
| `not_found` | 404 | Resource does not exist |
| `internal_error` | 500 | Unexpected server error |

## Compatibility

- Yggdrasil endpoints remain available under `/api/yggdrasil` for authlib-injector and Minecraft
  servers.
- The existing registration and admin HTML pages remain available while the Vue panel is built.
- Passwords use Argon2id. Access, refresh, session, invite, and verification secrets are stored only
  as hashes.
- The API does not authenticate Microsoft accounts and does not verify Minecraft ownership.
