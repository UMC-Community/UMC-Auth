ALTER TABLE users ADD COLUMN IF NOT EXISTS username_normalized TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_normalized TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user';
ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at BIGINT NOT NULL DEFAULT 0;

UPDATE users
SET username_normalized = lower(btrim(username))
WHERE username_normalized IS NULL OR username_normalized = '';

UPDATE users
SET email_normalized = lower(btrim(email))
WHERE email_normalized IS NULL;

UPDATE users
SET role = CASE WHEN is_admin THEN 'admin' ELSE 'user' END
WHERE role IS NULL OR role = 'user';

UPDATE users
SET status = CASE
    WHEN disabled THEN 'disabled'
    WHEN NOT email_verified THEN 'pending'
    ELSE 'active'
END
WHERE status IS NULL OR status = 'active';

UPDATE users SET updated_at = created_at WHERE updated_at = 0;

CREATE UNIQUE INDEX IF NOT EXISTS users_username_normalized ON users (username_normalized);
CREATE UNIQUE INDEX IF NOT EXISTS users_email_normalized
    ON users (email_normalized) WHERE email_normalized <> '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_role_check' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users ADD CONSTRAINT users_role_check
            CHECK (role IN ('user', 'moderator', 'admin', 'superadmin'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_status_check' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users ADD CONSTRAINT users_status_check
            CHECK (status IN ('active', 'pending', 'disabled', 'locked'));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS auth_sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    family_id TEXT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token_hash TEXT NOT NULL UNIQUE,
    refresh_token_hash TEXT UNIQUE,
    client TEXT NOT NULL DEFAULT 'api',
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL,
    last_used_at BIGINT NOT NULL,
    access_expires_at BIGINT NOT NULL,
    refresh_expires_at BIGINT,
    revoked_at BIGINT,
    replaced_by BIGINT REFERENCES auth_sessions(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS auth_sessions_user ON auth_sessions(user_id);
CREATE INDEX IF NOT EXISTS auth_sessions_family ON auth_sessions(family_id);
CREATE INDEX IF NOT EXISTS auth_sessions_refresh ON auth_sessions(refresh_token_hash);

CREATE TABLE IF NOT EXISTS email_verification_codes (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    email_normalized TEXT NOT NULL,
    purpose TEXT NOT NULL DEFAULT 'register',
    code_hash TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    send_count INTEGER NOT NULL DEFAULT 1,
    sent_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    used_at BIGINT,
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS email_codes_lookup
    ON email_verification_codes(email_normalized, purpose, created_at DESC);
CREATE INDEX IF NOT EXISTS email_codes_user ON email_verification_codes(user_id);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    expires_at BIGINT NOT NULL,
    used_at BIGINT,
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS password_reset_user ON password_reset_tokens(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS login_attempts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    username_normalized TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    success BOOLEAN NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS login_attempts_ip ON login_attempts(ip, created_at);
CREATE INDEX IF NOT EXISTS login_attempts_user ON login_attempts(username_normalized, created_at);

CREATE TABLE IF NOT EXISTS mail_outbox (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'generic',
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    next_attempt_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    sent_at BIGINT
);
CREATE INDEX IF NOT EXISTS mail_outbox_pending ON mail_outbox(status, next_attempt_at);
