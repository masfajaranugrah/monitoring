package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"fiber-monitor/internal/database"
	"fiber-monitor/internal/models"
)

func GetDashboardStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var stats struct {
		total, online, offlinelist, warning int64
	}
	var activeVPNs, totalVPNs int
	var avgLatency sql.NullInt64

	err := database.Pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM customers),
		  (SELECT COUNT(*) FROM customers WHERE status = 'ONLINE'),
		  (SELECT COUNT(*) FROM customers WHERE status = 'OFFLINE'),
		  (SELECT COUNT(*) FROM customers WHERE status = 'WARNING'),
		  (SELECT COUNT(*) FROM vpn_connections WHERE status IN ('CONNECTED','DISCONNECTED') AND is_active = true),
		  (SELECT COUNT(*) FROM vpn_connections),
		  (SELECT ROUND(AVG(latency_ms)) FROM
		     (SELECT DISTINCT ON (customer_id) latency_ms FROM ping_results
		      WHERE status = 'ONLINE' ORDER BY customer_id, pinged_at DESC) t
		   WHERE latency_ms IS NOT NULL)`).
		Scan(&stats.total, &stats.online, &stats.offlinelist, &stats.warning,
			&activeVPNs, &totalVPNs, &avgLatency)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load dashboard stats"})
		return
	}

	var latency *int
	if avgLatency.Valid {
		v := int(avgLatency.Int64)
		latency = &v
	}

	c.JSON(http.StatusOK, models.DashboardStats{
		TotalCustomers: stats.total,
		OnlineCount:    stats.online,
		OfflineCount:   stats.offlinelist,
		WarningCount:   stats.warning,
		ActiveVPNs:     activeVPNs,
		TotalVPNs:      totalVPNs,
		AvgLatency:     latency,
	})
}

// MapCustomers returns customer markers for the map with optional filters.
func MapCustomers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	status := c.Query("status")
	vpnID := c.Query("vpn_id")

	query := `
		SELECT c.id, c.customer_code, c.customer_name, c.ip_address,
		       c.latitude, c.longitude, c.vpn_id, COALESCE(v.name, ''),
		       c.status, c.latency_ms, c.last_check, c.last_online, c.last_offline,
		       c.uptime_percentage
		FROM customers c
		LEFT JOIN vpn_connections v ON v.id = c.vpn_id
		WHERE 1=1`
	args := []interface{}{}
	argID := 1

	if status != "" && (status == "ONLINE" || status == "OFFLINE" || status == "WARNING") {
		query += fmt.Sprintf(" AND c.status = $%d", argID)
		args = append(args, status)
		argID++
	}
	if vpnID != "" {
		query += fmt.Sprintf(" AND c.vpn_id = $%d", argID)
		args = append(args, vpnID)
	}

	rows, err := database.Pool.Query(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load map data"})
		return
	}
	defer rows.Close()

	customers := make([]models.Customer, 0)
	for rows.Next() {
		var cu models.Customer
		var vpnID sql.NullInt64
		if err := rows.Scan(&cu.ID, &cu.CustomerCode, &cu.CustomerName,
			&cu.IPAddress, &cu.Latitude, &cu.Longitude, &vpnID, &cu.VPNName,
			&cu.Status, &cu.LatencyMs, &cu.LastCheck, &cu.LastOnline,
			&cu.LastOffline, &cu.UptimePercentage); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan map customer"})
			return
		}
		if vpnID.Valid {
			id := vpnID.Int64
			cu.VpnID = &id
		}
		customers = append(customers, cu)
	}

	c.JSON(http.StatusOK, gin.H{"data": customers})
}

func PingHistory(c *gin.Context) {
	customerID, err := parseIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid customer ID"})
		return
	}

	limit := 100
	if v, err := parseIntQuery(c, "limit", 100); err == nil && v > 0 && v <= 1000 {
		limit = v
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	rows, err := database.Pool.Query(ctx, `
		SELECT id, customer_id, vpn_id, status, latency_ms, pinged_at
		FROM ping_results
		WHERE customer_id = $1
		ORDER BY pinged_at DESC
		LIMIT $2`, customerID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load ping history"})
		return
	}
	defer rows.Close()

	history := make([]models.PingResult, 0)
	for rows.Next() {
		var pr models.PingResult
		var vpnID sql.NullInt64
		if err := rows.Scan(&pr.ID, &pr.CustomerID, &vpnID, &pr.Status,
			&pr.LatencyMs, &pr.PingedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan ping result"})
			return
		}
		if vpnID.Valid {
			id := vpnID.Int64
			pr.VpnID = &id
		}
		history = append(history, pr)
	}

	c.JSON(http.StatusOK, gin.H{"data": history})
}

func StatusLogs(c *gin.Context) {
	customerID, err := parseIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid customer ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	rows, err := database.Pool.Query(ctx, `
		SELECT id, customer_id, old_status, new_status, latency_ms, reason, created_at
		FROM customer_status_logs
		WHERE customer_id = $1
		ORDER BY created_at DESC LIMIT 100`, customerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load status logs"})
		return
	}
	defer rows.Close()

	logs := make([]models.CustomerStatusLog, 0)
	for rows.Next() {
		var l models.CustomerStatusLog
		var oldStatus sql.NullString
		var latency sql.NullInt64
		var reason sql.NullString
		if err := rows.Scan(&l.ID, &l.CustomerID, &oldStatus, &l.NewStatus,
			&latency, &reason, &l.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan status log"})
			return
		}
		if oldStatus.Valid {
			s := models.Status(oldStatus.String)
			l.OldStatus = &s
		}
		if latency.Valid {
			v := int(latency.Int64)
			l.LatencyMs = &v
		}
		if reason.Valid {
			l.Reason = reason.String
		}
		logs = append(logs, l)
	}

	c.JSON(http.StatusOK, gin.H{"data": logs})
}

func AlertsList(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	limit := 50
	if v, err := parseIntQuery(c, "limit", 50); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	unreadOnly := c.Query("unread") == "1"

	query := `SELECT a.id, a.customer_id, a.alert_type, a.severity, a.title,
	                 a.message, a.is_read, a.created_at,
	                 COALESCE(cu.customer_name, ''), COALESCE(cu.customer_code, '')
	          FROM alerts a
	          LEFT JOIN customers cu ON cu.id = a.customer_id`
	args := []interface{}{}
	if unreadOnly {
		query += " WHERE a.is_read = false"
	}
	query += " ORDER BY a.created_at DESC LIMIT $1"
	args = append(args, limit)

	rows, err := database.Pool.Query(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load alerts"})
		return
	}
	defer rows.Close()

	alerts := make([]gin.H, 0)
	for rows.Next() {
		var a models.Alert
		var customerID sql.NullInt64
		var custName, custCode sql.NullString
		if err := rows.Scan(&a.ID, &customerID, &a.AlertType, &a.Severity,
			&a.Title, &a.Message, &a.IsRead, &a.CreatedAt, &custName, &custCode); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan alert"})
			return
		}
		row := gin.H{
			"id": a.ID, "alert_type": a.AlertType, "severity": a.Severity,
			"title": a.Title, "message": a.Message, "is_read": a.IsRead,
			"created_at": a.CreatedAt,
			"customer_name": custName.String, "customer_code": custCode.String,
		}
		if customerID.Valid {
			row["customer_id"] = customerID.Int64
		}
		alerts = append(alerts, row)
	}

	c.JSON(http.StatusOK, gin.H{"data": alerts})
}

func AlertMarkRead(c *gin.Context) {
	id, err := parseIntParam(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid alert ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	_, err = database.Pool.Exec(ctx, `UPDATE alerts SET is_read = true WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update alert"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Alert marked as read"})
}

func parseIntParam(c *gin.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

func parseIntQuery(c *gin.Context, name string, def int) (int, error) {
	s := c.Query(name)
	if s == "" {
		return def, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def, err
	}
	return int(v), nil
}