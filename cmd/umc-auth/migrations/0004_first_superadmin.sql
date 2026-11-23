UPDATE users
SET role = 'superadmin',
    is_admin = TRUE,
    updated_at = EXTRACT(EPOCH FROM now())::BIGINT
WHERE id = (
    SELECT id FROM users
    WHERE is_admin = TRUE AND role = 'admin'
    ORDER BY id
    LIMIT 1
)
AND NOT EXISTS (SELECT 1 FROM users WHERE role = 'superadmin');
