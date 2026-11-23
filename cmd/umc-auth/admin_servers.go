package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func apiServerJSON(server *ServerInstance) map[string]any {
	if server == nil {
		return nil
	}
	var createdBy any
	if server.CreatedBy.Valid {
		createdBy = server.CreatedBy.Int64
	}
	return map[string]any{
		"id":          server.ID,
		"name":        server.Name,
		"address":     server.Address,
		"description": server.Description,
		"status":      server.Status,
		"created_by":  createdBy,
		"created_at":  server.CreatedAt,
		"updated_at":  server.UpdatedAt,
	}
}

func apiJoinJSON(record *ServerJoinRecord) map[string]any {
	var userID any
	if record.UserID.Valid {
		userID = record.UserID.Int64
	}
	return map[string]any{
		"id":         record.ID,
		"user_id":    userID,
		"server_id":  record.ServerID,
		"username":   record.Username,
		"ip":         record.IP,
		"created_at": record.CreatedAt,
	}
}

func (a *App) apiAdminServers(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIAdmin(w, r); !ok {
		return
	}
	limit := queryInt(r, "limit", 100, 1, 500)
	servers, err := a.store.ListServerInstances(limit)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load servers")
		return
	}
	result := make([]map[string]any, 0, len(servers))
	for index := range servers {
		result = append(result, apiServerJSON(&servers[index]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": result})
}

type apiCreateServerRequest struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Description string `json:"description"`
}

func (a *App) apiAdminCreateServer(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	var request apiCreateServerRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || len(name) > 64 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Server name must be 1-64 characters")
		return
	}
	if len(request.Address) > 255 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Server address is too long")
		return
	}
	if len(request.Description) > 500 {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Server description is too long")
		return
	}
	now := time.Now().Unix()
	server, err := a.store.CreateServerInstance(
		name, request.Address, request.Description, actor.ID, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			apiV1Error(w, http.StatusConflict, "server_conflict", "A server with that name already exists")
			return
		}
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not create the server")
		return
	}
	_ = a.store.AddAudit(actor.ID, "server.create", server.Name, now)
	writeJSON(w, http.StatusCreated, map[string]any{"server": apiServerJSON(server)})
}

type apiServerStatusRequest struct {
	Status string `json:"status"`
}

func (a *App) apiAdminServerStatus(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Invalid server id")
		return
	}
	var request apiServerStatusRequest
	if !decodeV1JSON(w, r, &request) {
		return
	}
	status := strings.ToLower(strings.TrimSpace(request.Status))
	if status != "active" && status != "disabled" {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Status must be active or disabled")
		return
	}
	now := time.Now().Unix()
	if err := a.store.SetServerInstanceStatus(id, status, now); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not update the server")
		return
	}
	server, err := a.store.ServerInstanceByID(id)
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "Server not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the server")
		return
	}
	action := "server.disable"
	if status == "active" {
		action = "server.enable"
	}
	_ = a.store.AddAudit(actor.ID, action, server.Name, now)
	writeJSON(w, http.StatusOK, map[string]any{"server": apiServerJSON(server)})
}

func (a *App) apiAdminDeleteServer(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireAPIAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		apiV1Error(w, http.StatusBadRequest, "invalid_request", "Invalid server id")
		return
	}
	server, err := a.store.ServerInstanceByID(id)
	if errors.Is(err, sql.ErrNoRows) {
		apiV1Error(w, http.StatusNotFound, "not_found", "Server not found")
		return
	}
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load the server")
		return
	}
	if err := a.store.DeleteServerInstance(id); err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not delete the server")
		return
	}
	_ = a.store.AddAudit(actor.ID, "server.delete", server.Name, time.Now().Unix())
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) apiAdminServerJoins(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireAPIAdmin(w, r); !ok {
		return
	}
	limit := queryInt(r, "limit", 100, 1, 500)
	serverID := strings.TrimSpace(r.URL.Query().Get("server_id"))
	records, err := a.store.ListServerJoinRecords(limit, serverID)
	if err != nil {
		apiV1Error(w, http.StatusInternalServerError, "internal_error", "Could not load join records")
		return
	}
	result := make([]map[string]any, 0, len(records))
	for index := range records {
		result = append(result, apiJoinJSON(&records[index]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"joins": result})
}
