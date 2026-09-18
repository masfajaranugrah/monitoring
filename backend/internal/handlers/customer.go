package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"monitoring/internal/database"
	"monitoring/internal/models"
)

func validateIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil
}

func validateCoord(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func generateCustomerCode(ip string) string {
	replacer := strings.NewReplacer(".", "-", ":", "-", " ", "")
	return "CO-" + replacer.Replace(ip)
}

func uniqueCustomerCode(ctx context.Context, base string) string {
	code := base
	for i := 2; i <= 100; i++ {
		var exists bool
		if err := database.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM customers WHERE customer_code = $1)`, code).Scan(&exists); err != nil {
			return code
		}
		if !exists {
			return code
		}
		code = fmt.Sprintf("%s-%d", base, i)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

func ListCustomers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	search := c.Query("search")
	status := c.Query("status")
	vpnID := c.Query("vpn_id")
	sortBy := c.Query("sort_by")
	sortOrder := c.Query("sort_order")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	monitoring := c.Query("monitoring")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}

	where := " WHERE 1=1"
	args := []interface{}{}
	argID := 1

	if search != "" {
		where += fmt.Sprintf(" AND (c.customer_code ILIKE $%d OR c.customer_name ILIKE $%d OR c.ip_address ILIKE $%d)",
			argID, argID+1, argID+2)
		like := "%" + search + "%"
		args = append(args, like, like, like)
		argID += 3
	}
	if status != "" && (status == "ONLINE" || status == "OFFLINE" || status == "WARNING") {
		where += fmt.Sprintf(" AND c.status = $%d", argID)
		args = append(args, status)
		argID++
	}
	if vpnID != "" {
		where += fmt.Sprintf(" AND c.vpn_id = $%d", argID)
		args = append(args, vpnID)
		argID++
	}
	if monitoring == "1" {
		where += " AND c.monitoring_enabled = true"
	} else if monitoring == "0" {
		where += " AND c.monitoring_enabled = false"
	}

	allowedSorts := map[string]string{
		"customer_code": "c.customer_code",
		"customer_name": "c.customer_name",
		"status":        "c.status",
		"latency":       "c.latency_ms",
		"last_check":    "c.last_check",
		"vpn_name":      "v.name",
	}
	sortCol, ok := allowedSorts[sortBy]
	if !ok {
		sortCol = "c.customer_code"
	}
	order := "ASC"
	if sortOrder == "desc" {
		order = "DESC"
	}

	var total int64
	err := database.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM customers c`+where, args...).Scan(&total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count customers"})
		return
	}

	offset := (page - 1) * pageSize
	queryArgs := append(args, offset, pageSize)
	query := fmt.Sprintf(`
		SELECT c.id, c.customer_code, c.customer_name, c.ip_address, c.latitude, c.longitude,
		       c.vpn_id, COALESCE(v.name, ''), COALESCE(c.description, ''),
		       c.monitoring_enabled, c.ping_interval, c.timeout_ms, c.retry_count,
		       c.status, c.latency_ms, c.last_check, c.last_online, c.last_offline,
		       c.consecutive_failures, c.total_checks, c.uptime_percentage,
		       c.created_at, c.updated_at
		FROM customers c
		LEFT JOIN vpn_connections v ON v.id = c.vpn_id
		%s
		ORDER BY %s %s OFFSET $%d LIMIT $%d`,
		where, sortCol, order, argID, argID+1)

	rows, err := database.Pool.Query(ctx, query, queryArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query customers"})
		return
	}
	defer rows.Close()

	customers := make([]models.Customer, 0)
	for rows.Next() {
		var cu models.Customer
		var vpnID sql.NullInt64
		if err := rows.Scan(
			&cu.ID, &cu.CustomerCode, &cu.CustomerName, &cu.IPAddress,
			&cu.Latitude, &cu.Longitude, &vpnID, &cu.VPNName,
			&cu.Description, &cu.MonitoringEnabled, &cu.PingInterval,
			&cu.TimeoutMs, &cu.RetryCount, &cu.Status, &cu.LatencyMs,
			&cu.LastCheck, &cu.LastOnline, &cu.LastOffline,
			&cu.ConsecutiveFailures, &cu.TotalChecks, &cu.UptimePercentage,
			&cu.CreatedAt, &cu.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan customer"})
			return
		}
		if vpnID.Valid {
			id := vpnID.Int64
			cu.VpnID = &id
		}
		customers = append(customers, cu)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      customers,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pages":     (total + int64(pageSize) - 1) / int64(pageSize),
	})
}

func CreateCustomer(c *gin.Context) {
	var input models.CustomerCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if !validateIP(input.IPAddress) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP address"})
		return
	}
	if !validateCoord(input.Latitude, input.Longitude) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid coordinates. Latitude: -90..90, Longitude: -180..180"})
		return
	}
	if input.PingInterval < 5 {
		input.PingInterval = 5
	}
	if input.TimeoutMs < 500 {
		input.TimeoutMs = 2000
	}
	if input.RetryCount < 1 {
		input.RetryCount = 2
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var vpnID *int64
	if input.VpnID != nil {
		var exists bool
		err := database.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM vpn_connections WHERE id = $1)`, *input.VpnID).Scan(&exists)
		if err != nil || !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN connection ID"})
			return
		}
		vpnID = input.VpnID
	}

	var id int64
	if strings.TrimSpace(input.CustomerCode) == "" {
		input.CustomerCode = uniqueCustomerCode(ctx, generateCustomerCode(input.IPAddress))
	}
	err := database.Pool.QueryRow(ctx,
		`INSERT INTO customers
		 (customer_code, customer_name, ip_address, latitude, longitude, location,
		  vpn_id, description, monitoring_enabled, ping_interval, timeout_ms, retry_count)
		 VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($5, $4), 4326),
		         $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		input.CustomerCode, input.CustomerName, input.IPAddress,
		input.Latitude, input.Longitude, vpnID, input.Description,
		input.MonitoringEnabled, input.PingInterval, input.TimeoutMs, input.RetryCount,
	).Scan(&id)
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": "Customer code already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create customer: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "message": "Customer created successfully"})
}

func GetCustomer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid customer ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var cu models.Customer
	var vpnID sql.NullInt64
	err = database.Pool.QueryRow(ctx,
		`SELECT c.id, c.customer_code, c.customer_name, c.ip_address, c.latitude, c.longitude,
		        c.vpn_id, COALESCE(v.name, ''), COALESCE(c.description, ''),
		        c.monitoring_enabled, c.ping_interval, c.timeout_ms, c.retry_count,
		        c.status, c.latency_ms, c.last_check, c.last_online, c.last_offline,
		        c.consecutive_failures, c.total_checks, c.uptime_percentage,
		        c.created_at, c.updated_at
		 FROM customers c
		 LEFT JOIN vpn_connections v ON v.id = c.vpn_id
		 WHERE c.id = $1`, id).
		Scan(&cu.ID, &cu.CustomerCode, &cu.CustomerName, &cu.IPAddress,
			&cu.Latitude, &cu.Longitude, &vpnID, &cu.VPNName,
			&cu.Description, &cu.MonitoringEnabled, &cu.PingInterval,
			&cu.TimeoutMs, &cu.RetryCount, &cu.Status, &cu.LatencyMs,
			&cu.LastCheck, &cu.LastOnline, &cu.LastOffline,
			&cu.ConsecutiveFailures, &cu.TotalChecks, &cu.UptimePercentage,
			&cu.CreatedAt, &cu.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Customer not found"})
		return
	}
	if vpnID.Valid {
		id := vpnID.Int64
		cu.VpnID = &id
	}

	c.JSON(http.StatusOK, cu)
}

func UpdateCustomer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid customer ID"})
		return
	}

	var input models.CustomerCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if !validateIP(input.IPAddress) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP address"})
		return
	}
	if !validateCoord(input.Latitude, input.Longitude) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid coordinates"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	tag, err := database.Pool.Exec(ctx,
		`UPDATE customers SET customer_code=$1, customer_name=$2, ip_address=$3,
		   latitude=$4, longitude=$5, location=ST_SetSRID(ST_MakePoint($5, $4), 4326),
		   vpn_id=$6, description=$7, monitoring_enabled=$8, ping_interval=$9,
		   timeout_ms=$10, retry_count=$11, updated_at=now()
		 WHERE id = $12`,
		input.CustomerCode, input.CustomerName, input.IPAddress,
		input.Latitude, input.Longitude, input.VpnID, input.Description,
		input.MonitoringEnabled, input.PingInterval, input.TimeoutMs,
		input.RetryCount, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update customer"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Customer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Customer updated successfully"})
}

func DeleteCustomer(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid customer ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	tag, err := database.Pool.Exec(ctx,
		`DELETE FROM customers WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete customer"})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Customer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Customer deleted successfully"})
}

func ToggleMonitoring(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid customer ID"})
		return
	}

	var input struct {
		MonitoringEnabled bool `json:"monitoring_enabled"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	_, err = database.Pool.Exec(ctx,
		`UPDATE customers SET monitoring_enabled = $1, updated_at = now() WHERE id = $2`,
		input.MonitoringEnabled, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update customer"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Monitoring setting updated"})
}

func SyncLocation(c *gin.Context) {
	// Backfill location from latitude/longitude for rows that may have NULL geometry.
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	_, err := database.Pool.Exec(ctx,
		`UPDATE customers SET location = ST_SetSRID(ST_MakePoint(longitude, latitude), 4326)
		 WHERE location IS NULL`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sync locations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Locations synced"})
}