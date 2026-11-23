CREATE TABLE IF NOT EXISTS minecraft_profiles (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    player_name TEXT NOT NULL,
    player_uuid TEXT NOT NULL UNIQUE,
    skin_url TEXT NOT NULL DEFAULT '',
    cape_url TEXT NOT NULL DEFAULT '',
    skin_model TEXT NOT NULL DEFAULT 'classic',
    skin_hash TEXT NOT NULL DEFAULT '',
    cape_hash TEXT NOT NULL DEFAULT '',
    updated_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS minecraft_profiles_name
    ON minecraft_profiles (LOWER(player_name));

INSERT INTO minecraft_profiles (user_id, player_name, player_uuid, updated_at)
SELECT id, username, uuid, COALESCE(NULLIF(updated_at, 0), created_at)
FROM users
ON CONFLICT (user_id) DO NOTHING;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'minecraft_profiles_model_check'
          AND conrelid = 'minecraft_profiles'::regclass
    ) THEN
        ALTER TABLE minecraft_profiles ADD CONSTRAINT minecraft_profiles_model_check
            CHECK (skin_model IN ('classic', 'slim'));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS server_instances (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS server_instances_name
    ON server_instances (LOWER(name));

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'server_instances_status_check'
          AND conrelid = 'server_instances'::regclass
    ) THEN
        ALTER TABLE server_instances ADD CONSTRAINT server_instances_status_check
            CHECK (status IN ('active', 'disabled'));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS server_join_records (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    server_id TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS server_join_records_server
    ON server_join_records (server_id, created_at DESC);
CREATE INDEX IF NOT EXISTS server_join_records_user
    ON server_join_records (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS server_join_records_created
    ON server_join_records (created_at DESC);
