UPDATE users
SET username_normalized = lower(btrim(username))
WHERE username_normalized IS NULL;

UPDATE users
SET email_normalized = lower(btrim(email))
WHERE email_normalized IS NULL;

ALTER TABLE users ALTER COLUMN username_normalized SET NOT NULL;
ALTER TABLE users ALTER COLUMN email_normalized SET DEFAULT '';
ALTER TABLE users ALTER COLUMN email_normalized SET NOT NULL;
