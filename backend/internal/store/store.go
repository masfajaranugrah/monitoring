package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"monitoring/internal/database"
	"monitoring/internal/ping"
)

// Store implements the ping.Storer interface against PostgreSQL.
type Store struct{}

func New() *Store { return &Store{} }

// LoadMonitorData loads customers eligible for monitoring together with the
// VPN source IP so pings can be routed through the correct tunnel.
func (s *Store) LoadMonitorData(ctx context.Context) ([]ping.CustomerJob, error) {
	rows, err := database.Pool.Query(ctx, `
		SELECT c.id, c.ip_address, c.vpn_id, c.status,
		       c.ping_interval, c.timeout_ms, c.retry_count, c.monitoring_enabled,
		       COALESCE(v.local_ip, ''), COALESCE(v.interface_name, '')
		FROM customers c
		LEFT JOIN vpn_connections v ON v.id = c.vpn_id
		WHERE c.monitoring_enabled = true
		  AND (c.vpn_id IS NULL OR v.is_active = true)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]ping.CustomerJob, 0)
	for rows.Next() {
		var j ping.CustomerJob
		var vpnID sql.NullInt64
		if err := rows.Scan(&j.CustomerID, &j.IPAddress, &vpnID, &j.Status,
			&j.IntervalSec, &j.TimeoutMs, &j.Retries, &j.Monitored, &j.SourceIP, &j.Interface); err != nil {
			return nil, err
		}
		if vpnID.Valid {
			id := vpnID.Int64
			j.VPNID = &id
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ApplyPingOutcome runs the status machine:
//
//	success              -> ONLINE, reset consecutive failures
//	fail (<= retries)    -> WARNING
//	fail (> retries)     -> OFFLINE
//
// It persists ping results, status change logs, and alerts, then returns the
// new state so the engine can broadcast it in realtime.
func (s *Store) ApplyPingOutcome(ctx context.Context, a ping.PingAttempt) (*ping.PingOutcome, error) {
	var cu struct {
		ID          int64
		Status      string
		Consecutive int
		Retries     int
		Code        string
		Name        string
		IP          string
		VPNID       *int64
		VPNName     string
	}

	var vpnID sql.NullInt64
	err := database.Pool.QueryRow(ctx, `
		SELECT c.id, c.status, c.consecutive_failures, c.retry_count,
		       c.customer_code, c.customer_name, c.ip_address, c.vpn_id,
		       COALESCE(v.name, '')
		FROM customers c
		LEFT JOIN vpn_connections v ON v.id = c.vpn_id
		WHERE c.id = $1`, a.CustomerID).
		Scan(&cu.ID, &cu.Status, &cu.Consecutive, &cu.Retries,
			&cu.Code, &cu.Name, &cu.IP, &vpnID, &cu.VPNName)
	if err != nil {
		return nil, err
	}
	if vpnID.Valid {
		id := vpnID.Int64
		cu.VPNID = &id
	}

	newStatus := cu.Status
	newConsecutive := cu.Consecutive
	now := time.Now()

	if a.Success {
		newStatus = "ONLINE"
		newConsecutive = 0
	} else {
		newConsecutive++
		if newConsecutive > cu.Retries {
			newStatus = "OFFLINE"
		} else {
			newStatus = "WARNING"
		}
	}

	var lastOnline, lastOffline interface{}
	if newStatus == "ONLINE" {
		lastOnline = now
	} else if newStatus == "OFFLINE" {
		lastOffline = now
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE customers SET
			status = $1,
			latency_ms = $2,
			consecutive_failures = $3,
			last_check = now(),
			last_online = COALESCE($4, last_online),
			last_offline = COALESCE($5, last_offline),
			total_checks = total_checks + 1,
			updated_at = now()
		WHERE id = $6`,
		newStatus, a.LatencyMs, newConsecutive, lastOnline, lastOffline, cu.ID)
	if err != nil {
		return nil, err
	}

	// Persist a probe row for history/latency charting.
	_, err = tx.Exec(ctx, `
		INSERT INTO ping_results (customer_id, vpn_id, status, latency_ms, pinged_at)
		VALUES ($1, $2, $3, $4, now())`,
		cu.ID, cu.VPNID, newStatus, a.LatencyMs)
	if err != nil {
		return nil, err
	}

	// Log state transitions.
	if newStatus != cu.Status {
		_, err = tx.Exec(ctx, `
			INSERT INTO customer_status_logs (customer_id, old_status, new_status, latency_ms, reason)
			VALUES ($1, $2, $3, $4, $5)`,
			cu.ID, cu.Status, newStatus, a.LatencyMs, a.FailReason)
		if err != nil {
			return nil, err
		}
	}

	// Uptime percentage approximation from ping results.
	uptime := s.computeUptime(ctx, cu.ID)

	// Alert on offline transition.
	if newStatus == "OFFLINE" && cu.Status != "OFFLINE" {
		_, err = tx.Exec(ctx, `
			INSERT INTO alerts (customer_id, alert_type, severity, title, message)
			VALUES ($1, 'OFFLINE', 'CRITICAL', $2, $3)`,
			cu.ID,
			fmt.Sprintf("%s OFFLINE", cu.Name),
			fmt.Sprintf("Customer %s (%s) is OFFLINE. %s", cu.Code, cu.IP, a.FailReason))
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	outcome := &ping.PingOutcome{
		CustomerID:  cu.ID,
		Status:      newStatus,
		OldStatus:   cu.Status,
		LatencyMs:   a.LatencyMs,
		Consecutive: newConsecutive,
		UptimePct:   uptime,
		Name:        cu.Name,
		Code:        cu.Code,
		IP:          cu.IP,
	}
	return outcome, nil
}

func (s *Store) computeUptime(ctx context.Context, customerID int64) float64 {
	var total, success int64
	err := database.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'ONLINE')
		FROM ping_results
		WHERE customer_id = $1 AND pinged_at > now() - interval '24 hours'`,
		customerID).Scan(&total, &success)
	if err != nil || total == 0 {
		return 100.0
	}
	return float64(success*1000/total) / 10.0
}

// UpdateVPNStatus records VPN connection state.
func (s *Store) UpdateVPNStatus(ctx context.Context, vpnID int64, status string, latencyMs *int) error {
	_, err := database.Pool.Exec(ctx, `
		UPDATE vpn_connections SET status = $1, latency_ms = $2, updated_at = now()
		WHERE id = $3`, status, latencyMs, vpnID)
	return err
}

// RetentionWorker periodically prunes old ping results so the history table
// does not grow unboundedly. Retention in days via env and resumable loop.
func RetentionWorker(ctx context.Context) {
	days := 7
	if v := os.Getenv("PING_HISTORY_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	log.Printf("[store] ping history retention worker started (%d days)", days)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tag, err := database.Pool.Exec(ctx,
				`DELETE FROM ping_results WHERE pinged_at < now() - $1::interval`,
				fmt.Sprintf("%d days", days))
			if err != nil {
				log.Printf("[store] retention cleanup failed: %v", err)
				continue
			}
			log.Printf("[store] retention cleanup removed %d rows", tag.RowsAffected())
		}
	}
}
