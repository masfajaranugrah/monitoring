package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"monitoring/internal/crypto"
	"monitoring/internal/database"
	"monitoring/internal/models"
	"monitoring/internal/vpn"
)

type VPNHandler struct {
	Manager *vpn.Manager
}

func NewVPNHandler(mgr *vpn.Manager) *VPNHandler {
	return &VPNHandler{Manager: mgr}
}

func (h *VPNHandler) List(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	rows, err := database.Pool.Query(ctx, `
		SELECT v.id, v.name, v.vpn_type, v.server_address, v.username,
		       COALESCE(v.local_ip, ''), COALESCE(v.interface_name, ''),
		       v.is_active, v.status, v.latency_ms, v.last_connected_at,
		       (SELECT COUNT(*) FROM customers cu WHERE cu.vpn_id = v.id),
		       COALESCE(v.description, ''), v.created_at, v.updated_at
		FROM vpn_connections v
		ORDER BY v.name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load VPNs"})
		return
	}
	defer rows.Close()

	vpns := make([]models.VPNConnection, 0)
	for rows.Next() {
		var v models.VPNConnection
		if err := rows.Scan(&v.ID, &v.Name, &v.VPNType, &v.ServerAddress,
			&v.Username, &v.LocalIP, &v.InterfaceName, &v.IsActive,
			&v.Status, &v.LatencyMs, &v.LastConnectedAt, &v.CustomerCount,
			&v.Description, &v.CreatedAt, &v.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to scan VPNs"})
			return
		}
		vpns = append(vpns, v)
	}

	c.JSON(http.StatusOK, gin.H{"data": vpns})
}

func (h *VPNHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var v models.VPNConnection
	err = database.Pool.QueryRow(ctx, `
		SELECT v.id, v.name, v.vpn_type, v.server_address, v.username,
		       COALESCE(v.local_ip, ''), COALESCE(v.interface_name, ''),
		       v.is_active, v.status, v.latency_ms, v.last_connected_at,
		       (SELECT COUNT(*) FROM customers cu WHERE cu.vpn_id = v.id),
		       COALESCE(v.description, ''), v.created_at, v.updated_at
		FROM vpn_connections v WHERE v.id = $1`, id).
		Scan(&v.ID, &v.Name, &v.VPNType, &v.ServerAddress, &v.Username,
			&v.LocalIP, &v.InterfaceName, &v.IsActive, &v.Status,
			&v.LatencyMs, &v.LastConnectedAt, &v.CustomerCount,
			&v.Description, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "VPN not found"})
		return
	}

	c.JSON(http.StatusOK, v)
}

func (h *VPNHandler) Create(c *gin.Context) {
	var input models.VPNCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	encrypted, err := crypto.Encrypt(input.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt VPN password"})
		return
	}

	if input.InterfaceName == "" {
		if input.VPNType == models.VpnL2TP {
			input.InterfaceName = "l2tp"
		} else if input.VPNType == models.VpnSSTP {
			input.InterfaceName = "sstp"
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	var id int64
	err = database.Pool.QueryRow(ctx, `
		INSERT INTO vpn_connections
		  (name, vpn_type, server_address, username, password_encrypted,
		   local_ip, interface_name, is_active, description)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		input.Name, input.VPNType, input.ServerAddress, input.Username,
		encrypted, input.LocalIP, input.InterfaceName, input.IsActive, input.Description).
		Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create VPN: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "message": "VPN created successfully"})
}

func (h *VPNHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN ID"})
		return
	}

	var input models.VPNCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	encrypted, err := crypto.Encrypt(input.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt VPN password"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	_, err = database.Pool.Exec(ctx, `
		UPDATE vpn_connections SET
		  name=$1, vpn_type=$2, server_address=$3, username=$4,
		  password_encrypted=$5, local_ip=$6, interface_name=$7,
		  is_active=$8, description=$9, updated_at=now()
		WHERE id=$10`,
		input.Name, input.VPNType, input.ServerAddress, input.Username,
		encrypted, input.LocalIP, input.InterfaceName, input.IsActive,
		input.Description, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update VPN"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "VPN updated successfully"})
}

func (h *VPNHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	_, err = database.Pool.Exec(ctx, `DELETE FROM vpn_connections WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete VPN (may still have customers)"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "VPN deleted"})
}

func (h *VPNHandler) SetActive(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN ID"})
		return
	}

	var input struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	_, err = database.Pool.Exec(ctx,
		`UPDATE vpn_connections SET is_active = $1, updated_at = now() WHERE id = $2`,
		input.IsActive, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update VPN"})
		return
	}

	if !input.IsActive {
		// Bring the tunnel down on deactivation.
		if v, err := h.loadVPN(ctx, id); err == nil {
			h.Manager.Disconnect(&v)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "VPN status updated"})
}

// TestConnection pings the VPN server through the tunnel and refreshes state.
func (h *VPNHandler) TestConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	v, err := h.loadVPN(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "VPN not found"})
		return
	}

	status, ip := h.Manager.Status(&v)

	if status != models.VpnConnected && v.IsActive {
		_ = h.Manager.Connect(&v, passwordFor(&v))
		time.Sleep(1500 * time.Millisecond)
		status, ip = h.Manager.Status(&v)
	}

	ok, latency, msg := h.Manager.Test(&v, 3*time.Second)

	// Determinasi status akhir berdasarkan interface.
	if status == models.VpnConnected && ip != "" {
		status = models.VpnConnected
	} else {
		status = models.VpnDisconnected
	}

	if v.IsActive {
		_, _ = database.Pool.Exec(ctx, `
			UPDATE vpn_connections SET status = $1, latency_ms = $2,
			  local_ip = $3, last_connected_at = CASE WHEN $1 = 'CONNECTED' THEN now() ELSE last_connected_at END,
			  updated_at = now()
			WHERE id = $4`,
			status, latency, ip, id)
	}

	c.JSON(http.StatusOK, gin.H{
		"connected": ok,
		"status":    status,
		"latency_ms": latency,
		"message":   msg,
		"local_ip":  ip,
	})
}

// ConnectVPN initiates a VPN tunnel connection from the web UI.
func (h *VPNHandler) ConnectVPN(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	v, err := h.loadVPN(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "VPN not found"})
		return
	}
	if !v.IsActive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VPN is disabled. Enable it first."})
		return
	}

	// Check if already connected.
	currentStatus, currentIP := h.Manager.Status(&v)
	if currentStatus == models.VpnConnected {
		c.JSON(http.StatusOK, gin.H{
			"connected": true,
			"status":    currentStatus,
			"local_ip":  currentIP,
			"message":   "VPN already connected",
		})
		return
	}

	// Decrypt password and connect.
	pw := passwordFor(&v)
	if err := h.Manager.Connect(&v, pw); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Connect failed: " + err.Error()})
		return
	}

	// Wait for tunnel to come up (max 8 seconds).
	var newIP string
	var newStatus models.VpnStatus
	for i := 0; i < 8; i++ {
		time.Sleep(1 * time.Second)
		newStatus, newIP = h.Manager.Status(&v)
		if newStatus == models.VpnConnected {
			break
		}
	}

	// Persist.
	lat := (*int)(nil)
	if newStatus == models.VpnConnected && newIP != "" {
		if ok, ms, _ := h.Manager.Test(&v, 3*time.Second); ok {
			v := int(ms)
			lat = &v
		}
		_, _ = database.Pool.Exec(ctx,
			`UPDATE vpn_connections SET status='CONNECTED', latency_ms=$1, local_ip=$2,
			  last_connected_at=now(), updated_at=now() WHERE id=$3`,
			lat, newIP, id)
	} else {
		_, _ = database.Pool.Exec(ctx,
			`UPDATE vpn_connections SET status='DISCONNECTED', latency_ms=NULL, local_ip='',
			  updated_at=now() WHERE id=$1`, id)
	}

	connected := newStatus == models.VpnConnected
	msg := "VPN connected successfully"
	if !connected {
		msg = "VPN connection failed — tunnel did not come up. Check server/credentials."
	}

	c.JSON(http.StatusOK, gin.H{
		"connected": connected,
		"status":    newStatus,
		"local_ip":  newIP,
		"latency_ms": lat,
		"message":   msg,
	})
}

// DisconnectVPN brings down the VPN tunnel from the web UI.
func (h *VPNHandler) DisconnectVPN(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VPN ID"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	v, err := h.loadVPN(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "VPN not found"})
		return
	}

	h.Manager.Disconnect(&v)
	time.Sleep(1 * time.Second)

	status, _ := h.Manager.Status(&v)
	_, _ = database.Pool.Exec(ctx,
		`UPDATE vpn_connections SET status=$1, latency_ms=NULL, local_ip='',
		  updated_at=now() WHERE id=$2`,
		status, id)

	c.JSON(http.StatusOK, gin.H{
		"status":  status,
		"message": "VPN disconnected",
	})
}

// AutoConnectActiveVPNs tries to bring up all active VPN tunnels at server start.
// Runs in a goroutine so it never blocks server startup.
func (h *VPNHandler) AutoConnectActiveVPNs(ctx context.Context) {
	time.Sleep(3 * time.Second)

	rows, err := database.Pool.Query(ctx,
		`SELECT id, name, vpn_type, server_address, username, password_encrypted,
		        local_ip, interface_name, is_active FROM vpn_connections WHERE is_active = true`)
	if err != nil {
		log.Printf("[vpn] auto-connect: failed to load active VPNs: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var v models.VPNConnection
		if err := rows.Scan(&v.ID, &v.Name, &v.VPNType, &v.ServerAddress, &v.Username,
			&v.PasswordEncrypted, &v.LocalIP, &v.InterfaceName, &v.IsActive); err != nil {
			continue
		}

		status, currentIP := h.Manager.Status(&v)
		if status == models.VpnConnected && currentIP != "" {
			log.Printf("[vpn] auto-connect: %s already connected (%s)", v.Name, currentIP)
			continue
		}

		log.Printf("[vpn] auto-connect: connecting %s (%s)", v.Name, v.VPNType)
		if err := h.Manager.Connect(&v, passwordFor(&v)); err != nil {
			log.Printf("[vpn] auto-connect: %s connect error: %v", v.Name, err)
			continue
		}

		// Wait up to 10s for the tunnel to come up.
		for i := 0; i < 10; i++ {
			time.Sleep(1 * time.Second)
			s, ip := h.Manager.Status(&v)
			if s == models.VpnConnected && ip != "" {
				_, _ = database.Pool.Exec(ctx,
					`UPDATE vpn_connections SET status='CONNECTED', local_ip=$1,
					  last_connected_at=now(), updated_at=now() WHERE id=$2`,
					ip, v.ID)
				log.Printf("[vpn] auto-connect: %s connected (%s)", v.Name, ip)
				break
			}
		}
	}
}

func (h *VPNHandler) loadVPN(ctx context.Context, id int64) (models.VPNConnection, error) {
	var v models.VPNConnection
	err := database.Pool.QueryRow(ctx, `
		SELECT id, name, vpn_type, server_address, username, password_encrypted,
		       local_ip, interface_name, is_active
		FROM vpn_connections WHERE id = $1`, id).
		Scan(&v.ID, &v.Name, &v.VPNType, &v.ServerAddress, &v.Username,
			&v.PasswordEncrypted, &v.LocalIP, &v.InterfaceName, &v.IsActive)
	return v, err
}

func passwordFor(v *models.VPNConnection) string {
	pw, err := crypto.Decrypt(v.PasswordEncrypted)
	if err != nil {
		return ""
	}
	return pw
}

// RefreshStatus re-syncs all VPN connection states from the tunnel interfaces.
func (h *VPNHandler) RefreshStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	rows, err := database.Pool.Query(ctx,
		`SELECT id, name, vpn_type, server_address, username, password_encrypted,
		        local_ip, interface_name, is_active FROM vpn_connections`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load VPNs"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var v models.VPNConnection
		if err := rows.Scan(&v.ID, &v.Name, &v.VPNType, &v.ServerAddress, &v.Username,
			&v.PasswordEncrypted, &v.LocalIP, &v.InterfaceName, &v.IsActive); err != nil {
			continue
		}
		status, ip := h.Manager.Status(&v)
		latency := (*int)(nil)
		if v.IsActive {
			if ok, ms, _ := h.Manager.Test(&v, 2*time.Second); ok && status == models.VpnConnected {
				lat := int(ms)
				latency = &lat
			}
		}
		_, _ = database.Pool.Exec(ctx, `
			UPDATE vpn_connections SET status = $1, latency_ms = $2, local_ip = $3,
			  updated_at = now() WHERE id = $4`,
			status, latency, ip, v.ID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "VPN statuses refreshed"})
}