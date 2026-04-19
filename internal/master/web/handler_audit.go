package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"craftstack/internal/master/store"
)

// audit creates an audit log entry, extracting user info from the echo context.
func (s *Server) audit(c echo.Context, action, targetType, targetID, targetName, fieldName, oldVal, newVal, detail string) {
	userID, _ := c.Get("user_id").(int)
	username, _ := c.Get("username").(string)
	if username == "" {
		username = "system"
	}
	if err := s.db.CreateAuditLog(&store.AuditLog{
		UserID:     &userID,
		Username:   username,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		TargetName: targetName,
		FieldName:  fieldName,
		OldValue:   oldVal,
		NewValue:   newVal,
		Detail:     detail,
	}); err != nil {
		s.log.Warn("audit log create failed", "error", err)
	}
}

// auditFieldChange logs a single field change if the value actually changed.
func (s *Server) auditFieldChange(c echo.Context, targetType, targetID, targetName, field, oldVal, newVal string) {
	if oldVal != newVal {
		s.audit(c, "update", targetType, targetID, targetName, field, oldVal, newVal,
			fmt.Sprintf("%s change: %s → %s", field, oldVal, newVal))
	}
}

// handleAuditPage renders the audit log page.
func (s *Server) handleAuditPage(c echo.Context) error {
	page := 1
	limit := 50
	if p, err := strconv.Atoi(c.QueryParam("page")); err == nil && p > 0 {
		page = p
	}
	offset := (page - 1) * limit

	logs, err := s.db.ListAuditLogs(limit, offset)
	if err != nil {
		s.log.Error("audit log query failed", "error", err)
	}
	total, _ := s.db.CountAuditLogs()

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	data := map[string]interface{}{
		"Title":      "audit log",
		"AuditLogs":  logs,
		"Page":       page,
		"TotalPages": totalPages,
		"Total":      total,
		"Limit":      limit,
	}
	return renderPage(c, "audit", data)
}

// apiListAuditLogs returns audit logs as JSON with pagination.
func (s *Server) apiListAuditLogs(c echo.Context) error {
	page := 1
	limit := 50
	if p, err := strconv.Atoi(c.QueryParam("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.QueryParam("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	offset := (page - 1) * limit

	logs, err := s.db.ListAuditLogs(limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	total, _ := s.db.CountAuditLogs()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"logs":  logs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// apiRollbackAuditLog rolls back a configuration change recorded in an audit log entry.
func (s *Server) apiRollbackAuditLog(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "invalid ID"})
	}

	entry, err := s.db.GetAuditLog(id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"status": "error", "message": "audit log not found"})
	}

	if entry.RolledBack {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "already rollback item"})
	}

	if entry.FieldName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "field change record no rollbackcannot"})
	}

	// Currently only instance rollback is supported
	if entry.TargetType != "instance" {
		return c.JSON(http.StatusBadRequest, map[string]string{"status": "error", "message": "current instance configuration changeonly rollbackcan"})
	}

	inst, err := s.db.GetInstance(entry.TargetID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"status": "error", "message": "instance not found"})
	}

	oldValue := entry.OldValue // the value to restore

	// Apply the old value to the corresponding field
	switch entry.FieldName {
	case "memory_min":
		inst.MemoryMin = oldValue
	case "memory_max":
		inst.MemoryMax = oldValue
	case "docker_memory":
		inst.DockerMemory = oldValue
	case "docker_cpus":
		inst.DockerCPUs = oldValue
	case "auto_start":
		inst.AutoStart = oldValue == "true"
	case "auto_restart":
		inst.AutoRestart = oldValue == "true"
	case "stop_command":
		inst.StopCommand = oldValue
	case "jvm_args":
		inst.JVMArgs = oldValue
	case "service_version":
		inst.ServiceVersion = oldValue
	case "java_version":
		inst.JavaVersion = oldValue
	case "custom_dockerfile":
		inst.CustomDockerfile = oldValue
	case "custom_compose":
		inst.CustomCompose = oldValue
	// MySQL
	case "mysql_root_password":
		inst.MySQLRootPassword = oldValue
	case "mysql_extra_args":
		inst.MySQLExtraArgs = oldValue
	// PostgreSQL
	case "pg_password":
		inst.PGPassword = oldValue
	case "pg_extra_args":
		inst.PGExtraArgs = oldValue
	// MongoDB
	case "mongo_admin_user":
		inst.MongoAdminUser = oldValue
	case "mongo_admin_password":
		inst.MongoAdminPassword = oldValue
	case "mongo_extra_args":
		inst.MongoExtraArgs = oldValue
	// Redis
	case "redis_password":
		inst.RedisPassword = oldValue
	case "redis_extra_args":
		inst.RedisExtraArgs = oldValue
	// Kafka
	case "kafka_broker_id":
		if v, err := strconv.Atoi(oldValue); err == nil {
			inst.KafkaBrokerID = v
		}
	case "kafka_extra_args":
		inst.KafkaExtraArgs = oldValue
	// Backup schedule
	case "backup_enabled":
		inst.BackupEnabled = oldValue == "true"
	case "backup_cron":
		inst.BackupCron = oldValue
	case "backup_max_count":
		if v, err := strconv.Atoi(oldValue); err == nil {
			inst.BackupMaxCount = v
		}
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("rollback support not field: %s", entry.FieldName),
		})
	}

	if err := s.db.UpdateInstance(inst); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("instance update failed: %v", err),
		})
	}

	// Mark the original entry as rolled back
	if err := s.db.MarkAuditLogRolledBack(id); err != nil {
		s.log.Warn("rollback marking failed", "error", err)
	}

	// Create a new audit log for the rollback action
	s.audit(c, "update", "instance", entry.TargetID, entry.TargetName, entry.FieldName, entry.NewValue, entry.OldValue,
		fmt.Sprintf("rollback: %s %s → %s ( #%d)", entry.FieldName, entry.NewValue, entry.OldValue, entry.ID))

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("%s field '%s'() as rollback", entry.FieldName, entry.OldValue),
	})
}
