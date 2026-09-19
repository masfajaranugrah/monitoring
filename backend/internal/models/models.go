package models

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusOnline  Status = "ONLINE"
	StatusOffline Status = "OFFLINE"
	StatusWarning Status = "WARNING"
)

type VpnStatus string

const (
	VpnConnected    VpnStatus = "CONNECTED"
	VpnDisconnected VpnStatus = "DISCONNECTED"
	VpnError        VpnStatus = "ERROR"
	VpnDisabled     VpnStatus = "DISABLED"
)

type VpnType string

const (
	VpnL2TP      VpnType = "L2TP"
	VpnSSTP      VpnType = "SSTP"
	VpnPPTP      VpnType = "PPTP"
	VpnOpenVPN   VpnType = "OPENVPN"
	VpnWireGuard VpnType = "WIREGUARD"
)

type Role string

const (
	RoleAdmin     Role = "ADMIN"
	RoleOperator  Role = "OPERATOR"
)

type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	FullName     string     `json:"full_name"`
	Role         Role       `json:"role"`
	IsActive     bool       `json:"is_active"`
	LastLogin    *time.Time `json:"last_login,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type VPNConnection struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	VPNType          VpnType    `json:"vpn_type"`
	ServerAddress    string     `json:"server_address"`
	Username         string     `json:"username"`
	PasswordEncrypted string    `json:"-"`
	LocalIP          string     `json:"local_ip,omitempty"`
	InterfaceName    string     `json:"interface_name"`
	IsActive         bool       `json:"is_active"`
	Status           VpnStatus  `json:"status"`
	LatencyMs        *int       `json:"latency_ms,omitempty"`
	LastConnectedAt  *time.Time `json:"last_connected_at,omitempty"`
	CustomerCount    int        `json:"customer_count"`
	Description      string     `json:"description,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type Customer struct {
	ID                  int64      `json:"id"`
	CustomerCode        string     `json:"customer_code"`
	CustomerName        string     `json:"customer_name"`
	IPAddress           string     `json:"ip_address"`
	Latitude            float64    `json:"latitude"`
	Longitude           float64    `json:"longitude"`
	VpnID               *int64     `json:"vpn_id,omitempty"`
	VPNName             string     `json:"vpn_name,omitempty"`
	Icon                string     `json:"icon,omitempty"`
	Description         string     `json:"description,omitempty"`
	MonitoringEnabled   bool       `json:"monitoring_enabled"`
	PingInterval        int        `json:"ping_interval"`
	TimeoutMs           int        `json:"timeout_ms"`
	RetryCount          int        `json:"retry_count"`
	Status              Status     `json:"status"`
	LatencyMs           *int       `json:"latency_ms,omitempty"`
	LastCheck           *time.Time `json:"last_check,omitempty"`
	LastOnline          *time.Time `json:"last_online,omitempty"`
	LastOffline         *time.Time `json:"last_offline,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	TotalChecks         int64      `json:"total_checks"`
	UptimePercentage    float64    `json:"uptime_percentage"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type PingResult struct {
	ID        int64     `json:"id"`
	CustomerID int64    `json:"customer_id"`
	VpnID     *int64    `json:"vpn_id,omitempty"`
	Status    Status    `json:"status"`
	LatencyMs *int      `json:"latency_ms,omitempty"`
	PingedAt  time.Time `json:"pinged_at"`
}

type CustomerStatusLog struct {
	ID         int64     `json:"id"`
	CustomerID int64     `json:"customer_id"`
	OldStatus  *Status   `json:"old_status,omitempty"`
	NewStatus  Status    `json:"new_status"`
	LatencyMs  *int      `json:"latency_ms,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Alert struct {
	ID         int64     `json:"id"`
	CustomerID *int64    `json:"customer_id,omitempty"`
	AlertType  string    `json:"alert_type"`
	Severity   string    `json:"severity"`
	Title      string    `json:"title"`
	Message    string    `json:"message"`
	IsRead     bool      `json:"is_read"`
	CreatedAt  time.Time `json:"created_at"`
}

type DashboardStats struct {
	TotalCustomers int64 `json:"total_customers"`
	OnlineCount    int64 `json:"online_count"`
	OfflineCount   int64 `json:"offline_count"`
	WarningCount   int64 `json:"warning_count"`
	ActiveVPNs     int   `json:"active_vpns"`
	TotalVPNs      int   `json:"total_vpns"`
	AvgLatency     *int  `json:"avg_latency,omitempty"`
}

type CustomerCreateInput struct {
	CustomerCode      string   `json:"customer_code"`
	CustomerName      string   `json:"customer_name" binding:"required"`
	IPAddress         string   `json:"ip_address" binding:"required"`
	Latitude          float64  `json:"latitude" binding:"required"`
	Longitude         float64  `json:"longitude" binding:"required"`
	VpnID             *int64   `json:"vpn_id"`
	Icon              string   `json:"icon"`
	Description       string   `json:"description"`
	MonitoringEnabled bool     `json:"monitoring_enabled"`
	PingInterval      int      `json:"ping_interval"`
	TimeoutMs         int      `json:"timeout_ms"`
	RetryCount        int      `json:"retry_count"`
}

type VPNCreateInput struct {
	Name           string  `json:"name" binding:"required"`
	VPNType        VpnType `json:"vpn_type" binding:"required"`
	ServerAddress  string  `json:"server_address" binding:"required"`
	Username       string  `json:"username" binding:"required"`
	Password       string  `json:"password" binding:"required"`
	LocalIP        string  `json:"local_ip"`
	InterfaceName  string  `json:"interface_name"`
	IsActive       bool    `json:"is_active"`
	Description    string  `json:"description"`
}

type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UserCreateInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"full_name"`
	Role     Role   `json:"role"`
	IsActive bool   `json:"is_active"`
}

type PingEvent struct {
	Type      string      `json:"type"`
	Customer  *Customer   `json:"customer,omitempty"`
	Stats     interface{} `json:"stats,omitempty"`
	VPNStatus *VPNConnection `json:"vpn_status,omitempty"`
	Alert     *Alert      `json:"alert,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

type MapFeature struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	FeatureType string          `json:"feature_type"`
	Icon        string          `json:"icon"`
	Color       string          `json:"color"`
	Description string          `json:"description"`
	Geometry    json.RawMessage `json:"geometry"`
	Properties  json.RawMessage `json:"properties"`
	Source      string          `json:"source"`
	CreatedBy   *int64          `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}