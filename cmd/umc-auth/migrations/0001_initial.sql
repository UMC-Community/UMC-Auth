CREATE TABLE IF NOT EXISTS users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    email_verified BOOLEAN NOT NULL DEFAULT TRUE,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at BIGINT NOT NULL,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS disabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower ON users (LOWER(username));
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower ON users (LOWER(email)) WHERE email <> '';

CREATE TABLE IF NOT EXISTS access_tokens (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    client_token TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS access_tokens_user ON access_tokens(user_id);

CREATE TABLE IF NOT EXISTS web_sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    csrf_token TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS web_sessions_user ON web_sessions(user_id);

CREATE TABLE IF NOT EXISTS joins (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    server_id TEXT NOT NULL,
    ip TEXT,
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS joins_lookup ON joins(server_id, username, created_at);

CREATE TABLE IF NOT EXISTS invites (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code_hash TEXT NOT NULL UNIQUE,
    code_prefix TEXT NOT NULL,
    max_uses INTEGER NOT NULL,
    used_count INTEGER NOT NULL DEFAULT 0,
    expires_at BIGINT NOT NULL,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL,
    revoked_at BIGINT
);
CREATE INDEX IF NOT EXISTS invites_created ON invites(created_at);

CREATE TABLE IF NOT EXISTS email_verifications (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at BIGINT NOT NULL,
    used_at BIGINT,
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS email_verifications_user ON email_verifications(user_id);

CREATE TABLE IF NOT EXISTS pow_challenges (
    challenge TEXT PRIMARY KEY,
    expires_at BIGINT NOT NULL,
    used_at BIGINT
);

CREATE TABLE IF NOT EXISTS audit_log (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS audit_log_created ON audit_log(created_at);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
