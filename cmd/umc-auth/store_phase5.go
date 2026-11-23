package main

import (
	"database/sql"
	"fmt"
	"strings"
)

type MinecraftProfile struct {
	ID         int64
	UserID     int64
	PlayerName string
	PlayerUUID string
	SkinURL    string
	CapeURL    string
	SkinModel  string
	SkinHash   string
	CapeHash   string
	UpdatedAt  int64
}

type ServerInstance struct {
	ID          int64
	Name        string
	Address     string
	Description string
	Status      string
	CreatedBy   sql.NullInt64
	CreatedAt   int64
	UpdatedAt   int64
}

type ServerJoinRecord struct {
	ID        int64
	UserID    sql.NullInt64
	ServerID  string
	Username  string
	IP        string
	CreatedAt int64
}

func (s *Store) EnsureMinecraftProfile(
	userID int64,
	playerName, playerUUID string,
	now int64,
) error {
	_, err := s.db.Exec(
		`INSERT INTO minecraft_profiles
		 (user_id, player_name, player_uuid, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE
		 SET player_name = EXCLUDED.player_name,
		     player_uuid = EXCLUDED.player_uuid,
		     updated_at = EXCLUDED.updated_at`,
		userID, playerName, normalizeUUID(playerUUID), now,
	)
	return err
}

func (s *Store) MinecraftProfileByUserID(userID int64) (*MinecraftProfile, error) {
	return scanMinecraftProfile(s.db.QueryRow(
		`SELECT id, user_id, player_name, player_uuid, skin_url, cape_url,
		        skin_model, skin_hash, cape_hash, updated_at
		 FROM minecraft_profiles WHERE user_id = $1`,
		userID,
	))
}

func (s *Store) MinecraftProfileByUUID(uuid string) (*MinecraftProfile, error) {
	return scanMinecraftProfile(s.db.QueryRow(
		`SELECT id, user_id, player_name, player_uuid, skin_url, cape_url,
		        skin_model, skin_hash, cape_hash, updated_at
		 FROM minecraft_profiles WHERE player_uuid = $1`,
		normalizeUUID(uuid),
	))
}

func (s *Store) UpdateMinecraftSkin(
	userID int64,
	skinURL, skinHash, model string,
	now int64,
) error {
	_, err := s.db.Exec(
		`UPDATE minecraft_profiles
		 SET skin_url = $1, skin_hash = $2, skin_model = $3, updated_at = $4
		 WHERE user_id = $5`,
		skinURL, skinHash, model, now, userID,
	)
	return err
}

func (s *Store) ClearMinecraftSkin(userID int64, now int64) error {
	_, err := s.db.Exec(
		`UPDATE minecraft_profiles
		 SET skin_url = '', skin_hash = '', skin_model = 'classic', updated_at = $1
		 WHERE user_id = $2`,
		now, userID,
	)
	return err
}

func (s *Store) UpdateMinecraftCape(userID int64, capeURL, capeHash string, now int64) error {
	_, err := s.db.Exec(
		`UPDATE minecraft_profiles
		 SET cape_url = $1, cape_hash = $2, updated_at = $3
		 WHERE user_id = $4`,
		capeURL, capeHash, now, userID,
	)
	return err
}

func (s *Store) ClearMinecraftCape(userID int64, now int64) error {
	_, err := s.db.Exec(
		`UPDATE minecraft_profiles
		 SET cape_url = '', cape_hash = '', updated_at = $1
		 WHERE user_id = $2`,
		now, userID,
	)
	return err
}

func scanMinecraftProfile(row rowScanner) (*MinecraftProfile, error) {
	var profile MinecraftProfile
	if err := row.Scan(
		&profile.ID, &profile.UserID, &profile.PlayerName, &profile.PlayerUUID,
		&profile.SkinURL, &profile.CapeURL, &profile.SkinModel, &profile.SkinHash,
		&profile.CapeHash, &profile.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *Store) CreateServerInstance(
	name, address, description string,
	createdBy, now int64,
) (*ServerInstance, error) {
	var instance ServerInstance
	err := s.db.QueryRow(
		`INSERT INTO server_instances
		 (name, address, description, status, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, 'active', $4, $5, $5)
		 RETURNING id, name, address, description, status, created_by, created_at, updated_at`,
		strings.TrimSpace(name), strings.TrimSpace(address), strings.TrimSpace(description),
		createdBy, now,
	).Scan(
		&instance.ID, &instance.Name, &instance.Address, &instance.Description,
		&instance.Status, &instance.CreatedBy, &instance.CreatedAt, &instance.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

func (s *Store) ListServerInstances(limit int) ([]ServerInstance, error) {
	rows, err := s.db.Query(
		`SELECT id, name, address, description, status, created_by, created_at, updated_at
		 FROM server_instances ORDER BY created_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var instances []ServerInstance
	for rows.Next() {
		var instance ServerInstance
		if err := rows.Scan(
			&instance.ID, &instance.Name, &instance.Address, &instance.Description,
			&instance.Status, &instance.CreatedBy, &instance.CreatedAt, &instance.UpdatedAt,
		); err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (s *Store) ServerInstanceByID(id int64) (*ServerInstance, error) {
	var instance ServerInstance
	err := s.db.QueryRow(
		`SELECT id, name, address, description, status, created_by, created_at, updated_at
		 FROM server_instances WHERE id = $1`,
		id,
	).Scan(
		&instance.ID, &instance.Name, &instance.Address, &instance.Description,
		&instance.Status, &instance.CreatedBy, &instance.CreatedAt, &instance.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

func (s *Store) SetServerInstanceStatus(id int64, status string, now int64) error {
	if status != "active" && status != "disabled" {
		return fmt.Errorf("invalid server status %q", status)
	}
	_, err := s.db.Exec(
		"UPDATE server_instances SET status = $1, updated_at = $2 WHERE id = $3",
		status, now, id,
	)
	return err
}

func (s *Store) DeleteServerInstance(id int64) error {
	result, err := s.db.Exec("DELETE FROM server_instances WHERE id = $1", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RecordServerJoin(
	userID int64,
	serverID, username, ip string,
	now int64,
) error {
	var userValue any
	if userID > 0 {
		userValue = userID
	}
	_, err := s.db.Exec(
		`INSERT INTO server_join_records (user_id, server_id, username, ip, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userValue, serverID, username, ip, now,
	)
	return err
}

func (s *Store) ListServerJoinRecords(limit int, serverID string) ([]ServerJoinRecord, error) {
	query := `SELECT id, user_id, server_id, username, ip, created_at FROM server_join_records`
	args := []any{}
	if serverID != "" {
		query += " WHERE server_id = $1"
		args = append(args, serverID)
	}
	query += " ORDER BY created_at DESC LIMIT $" + fmt.Sprint(len(args)+1)
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ServerJoinRecord
	for rows.Next() {
		var record ServerJoinRecord
		if err := rows.Scan(
			&record.ID, &record.UserID, &record.ServerID, &record.Username,
			&record.IP, &record.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) SearchAudit(query string, limit int) ([]AuditEntry, error) {
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := s.db.Query(
		`SELECT a.id, COALESCE(u.username, 'system'), a.action, a.detail, a.created_at
		 FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id
		 WHERE $1 = '%%' OR a.action ILIKE $1 OR a.detail ILIKE $1
		    OR COALESCE(u.username, '') ILIKE $1
		 ORDER BY a.created_at DESC LIMIT $2`,
		pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Actor, &entry.Action, &entry.Detail, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) DeleteMinecraftProfile(userID int64) error {
	_, err := s.db.Exec("DELETE FROM minecraft_profiles WHERE user_id = $1", userID)
	return err
}
