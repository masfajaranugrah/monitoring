package ping

import (
	"context"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// MonitorEngine adalah layanan monitoring background. Ia bekerja secara
// STRICT SEQUENTIAL: satu pinger, satu goroutine loop, satu ping pada satu
// waktu. Semua pelanggan diperiksa satu per satu (tidak pernah paralel),
// dengan pacing agar satu siklus selesai dalam kurun interval (default 5
// menit). Tidak ada burst ping dan tidak membanjiri CPU/network.
type MonitorEngine struct {
	DB          Storer
	Hub         EventPublisher
	pinger      *ICMPPinger

	ensureRoute func(ip, iface string)

	mu      sync.RWMutex
	jobs    []CustomerJob
	cancel  context.CancelFunc
	running bool

	// intervalOverride berisi interval siklus global (detik) dari env
	// PING_INTERVAL_SEC. Nilai 0 berarti memakai interval terbesar
	// per-pelanggan di database.
	intervalOverride int
	// timeoutOverride berisi timeout global (ms) dari env PING_TIMEOUT_MS.
	// Nilai 0 berarti memakai timeout per-pelanggan.
	timeoutOverride int
	// jitterOverride berisi jeda acak (detik) setelah satu siklus selesai.
	// Nilai 0 berarti otomatis = interval/5.
	jitterOverride int
}

// CustomerJob adalah konfigurasi monitoring untuk satu pelanggan plus VPN
// source IP / interface yang dipakai merutekan ping lewat tunnel yang benar.
type CustomerJob struct {
	CustomerID  int64
	IPAddress   string
	VPNID       *int64
	Status      string
	IntervalSec int
	TimeoutMs   int
	Retries     int
	Monitored   bool
	SourceIP    string
	Interface   string
}

// Storer abstracts the persistence needs of the monitor engine.
type Storer interface {
	LoadMonitorData(ctx context.Context) ([]CustomerJob, error)
	// ApplyPingOutcome computes the new status (retry state machine), persists
	// the transition (ping result + status log + alert) and returns the full
	// outcome.
	ApplyPingOutcome(ctx context.Context, o PingAttempt) (*PingOutcome, error)
	UpdateVPNStatus(ctx context.Context, vpnID int64, status string, latencyMs *int) error
}

// PingAttempt adalah satu pengukuran untuk satu pelanggan.
type PingAttempt struct {
	CustomerID int64
	Success    bool
	LatencyMs  *int
	VPNID      *int64
	FailReason string
}

// PingOutcome adalah hasil yang disimpan setelah state machine status berjalan.
type PingOutcome struct {
	CustomerID   int64
	Status       string
	OldStatus    string
	LatencyMs    *int
	Consecutive  int
	UptimePct    float64
	CustomerJSON map[string]interface{}
	Name         string
	Code         string
	IP           string
}

// EventPublisher mengirimkan event realtime ke klien SSE/WebSocket.
type EventPublisher interface {
	BroadcastCustomerUpdate(payload map[string]interface{})
	BroadcastStats(payload interface{})
	BroadcastVPNStatus(payload map[string]interface{})
	PublishAlert(payload interface{})
}

// emptyPassWait digunakan ketika tidak ada pelanggan yang dimonitor.
const emptyPassWait = 5 * time.Second

func NewMonitorEngine(db Storer, hub EventPublisher, workerCount, intervalSec, timeoutMs, jitterSec int, ensureRoute func(ip, iface string)) *MonitorEngine {
	// workerCount sengaja tidak dipakai: engine ini wajib sequential.
	_ = workerCount
	return &MonitorEngine{
		DB:               db,
		Hub:              hub,
		pinger:           NewICMPPinger(),
		ensureRoute:      ensureRoute,
		intervalOverride: intervalSec,
		timeoutOverride:  timeoutMs,
		jitterOverride:   jitterSec,
	}
}

// timeoutFor mengembalikan timeout per ping. Jika env PING_TIMEOUT_MS di-set,
// nilai itu dipakai untuk semua pelanggan (disarankan 1000ms); jika tidak,
// timeout per-pelanggan dari database.
func (m *MonitorEngine) timeoutFor(j CustomerJob) time.Duration {
	if m.timeoutOverride > 0 {
		return time.Duration(m.timeoutOverride) * time.Millisecond
	}
	return time.Duration(j.TimeoutMs) * time.Millisecond
}

// start launches the single sequential monitor loop.
func (m *MonitorEngine) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.running = true

	// Muatan awal.
	if jobs, err := m.DB.LoadMonitorData(ctx); err == nil {
		m.replaceJobs(jobs)
	}

	go m.runLoop(ctx)
	log.Println("[monitor] engine started (sequential, 1 ping at a time)")
}

func (m *MonitorEngine) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
	log.Println("[monitor] engine stopped")
}

func (m *MonitorEngine) IsRunning() bool { return m.running }

// Reload menyegarkan daftar pelanggan dari database.
func (m *MonitorEngine) Reload() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	jobs, err := m.DB.LoadMonitorData(ctx)
	if err != nil {
		log.Printf("[monitor] load monitor data failed: %v", err)
		return
	}
	m.replaceJobs(jobs)
	log.Printf("[monitor] reloaded %d customers", len(jobs))
}

func (m *MonitorEngine) replaceJobs(jobs []CustomerJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = jobs
}

// snapshotMonitored mengambil daftar pelanggan yang aktif dimonitor, diurutkan
// stabil berdasarkan CustomerID agar urutan antrean selalu konsisten.
func (m *MonitorEngine) snapshotMonitored() []CustomerJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]CustomerJob, 0, len(m.jobs))
	for _, j := range m.jobs {
		if j.Monitored {
			out = append(out, j)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CustomerID < out[b].CustomerID })
	return out
}

// runLoop adalah SATU-SATUNYA scheduler: satu loop sequential.
// Tiap iterasi = satu siklus penuh (semua IP dicek sekali), lalu jeda acak
// (jitter), lalu mulai lagi dari IP pertama.
func (m *MonitorEngine) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if done, cycleSec := m.runPass(ctx); done {
			// Jeda jitter acak setelah siklus selesai agar siklus berikutnya
			// tidak selalu mulai pada waktu yang sama.
			if j := m.jitterFor(cycleSec); j > 0 {
				if !sleepCtx(ctx, time.Duration(rand.Intn(j))*time.Second) {
					return
				}
			}
		} else if !sleepCtx(ctx, emptyPassWait) {
			return
		}
	}
}

// runPass berjalan satu siklus: ping semua pelanggan sekali, satu per satu.
// Mengembalikan false jika tidak ada pelanggan yang dimonitor.
func (m *MonitorEngine) runPass(ctx context.Context) (done bool, cycleSec int) {
	jobs := m.snapshotMonitored()
	n := len(jobs)
	if n == 0 {
		return false, 0
	}

	cycleSec = m.cycleSec(jobs)
	if cycleSec < 5 {
		cycleSec = 5
	}
	cycle := time.Duration(cycleSec) * time.Second

	start := time.Now()
	log.Printf("[monitor] pass dimulai: %d IP, target siklus %d detik (± %d ping/detik)",
		n, cycleSec, n/cycleSec)

	for i, j := range jobs {
		select {
		case <-ctx.Done():
			return true, cycleSec
		default:
		}

		m.check(ctx, j)

		// Pacing: pertahankan jarak antar ping = cycle/n. Ping dikerjakan
		// bergantian, satu per satu, sehingga tidak pernah ada burst.
		targetElapsed := time.Duration(float64(cycle) * float64(i+1) / float64(n))
		target := start.Add(targetElapsed)
		if wait := time.Until(target); wait > 0 && !sleepCtx(ctx, wait) {
			return true, cycleSec
		}
	}

	log.Printf("[monitor] siklus selesai: %d IP dalam %s", n, time.Since(start).Round(time.Millisecond))
	return true, cycleSec
}

// cycleSec menentukan durasi satu siklus: pakai override global jika di-set,
// jika tidak gunakan interval terbesar di antara pelanggan (default DB 300s).
func (m *MonitorEngine) cycleSec(jobs []CustomerJob) int {
	if m.intervalOverride > 0 {
		return m.intervalOverride
	}
	maxSec := 300
	for _, j := range jobs {
		if j.IntervalSec > maxSec {
			maxSec = j.IntervalSec
		}
	}
	return maxSec
}

// jitterFor mengembalikan maksimum jeda acak (detik) di antara dua siklus.
// Default = siklus/5; bisa di-set tetap via env PING_JITTER_SEC.
func (m *MonitorEngine) jitterFor(cycleSec int) int {
	if m.jitterOverride > 0 {
		return m.jitterOverride
	}
	if cycleSec/5 < 1 {
		return 1
	}
	return cycleSec / 5
}

// check mengerjakan SATU ping untuk SATU pelanggan, menyimpan hasilnya
// (UP/DOWN), lalu menyiarkan event. Tidak ada ping lain yang berjalan
// bersamaan dalam proses ini — benar-benar satu per satu.
func (m *MonitorEngine) check(ctx context.Context, j CustomerJob) {
	timeout := m.timeoutFor(j)

	tctx, cancel := context.WithTimeout(ctx, time.Second+timeout)
	defer cancel()

	if j.Interface != "" && m.ensureRoute != nil {
		m.ensureRoute(j.IPAddress, j.Interface)
	}

	result := m.pinger.Ping(j.IPAddress, j.SourceIP, timeout)

	var latency *int
	if result.Success {
		ms := int(result.RTT.Milliseconds())
		latency = &ms
	}

	attempt := PingAttempt{
		CustomerID: j.CustomerID,
		Success:    result.Success,
		LatencyMs:  latency,
		VPNID:      j.VPNID,
		FailReason: result.ErrorMsg,
	}

	outcome, err := m.DB.ApplyPingOutcome(tctx, attempt)
	if err != nil {
		log.Printf("[monitor] apply outcome gagal untuk customer %d: %v", j.CustomerID, err)
		return
	}

	if m.Hub != nil && outcome != nil {
		payload := map[string]interface{}{
			"customer_id": outcome.CustomerID,
			"status":      outcome.Status,
			"latency_ms":  outcome.LatencyMs,
			"last_check":  time.Now(),
		}
		m.Hub.BroadcastCustomerUpdate(payload)

		// Hanya kirim alert saat transisi ke OFFLINE (bukan kegagalan berulang).
		if outcome.Status == "OFFLINE" && outcome.OldStatus != "OFFLINE" {
			m.Hub.PublishAlert(map[string]interface{}{
				"customer_id": outcome.CustomerID,
				"code":        outcome.Code,
				"name":        outcome.Name,
				"ip":          outcome.IP,
				"status":      "OFFLINE",
				"alert_type":  "OFFLINE",
				"severity":    "CRITICAL",
				"title":       "ROUTER DOWN",
				"time":        time.Now(),
			})
		}
	}
}

// sleepCtx tidur selama d atau keluar lebih awal saat konteks dibatalkan.
// Mengembalikan false jika konteks dibatalkan sebelum durasi berakhir.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}