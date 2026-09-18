package ping

import (
	"context"
	"log"
	"sync"
	"time"
)

// MonitorEngine is the background monitoring service. It pings customer IPs
// through their assigned VPN tunnels and updates status in the database in
// parallel using a fixed worker pool. It is driven by a lightweight scheduler
// that dispatches due jobs (respecting each customer's ping interval).
type MonitorEngine struct {
	DB          Storer
	Hub         EventPublisher
	pinger      *ICMPPinger
	workerCount int
	queue       chan workItem

	ensureRoute func(ip, iface string)

	mu          sync.RWMutex
	jobs        []CustomerJob
	nextRun     map[int64]time.Time
	nextSeq     map[int64]int
	cancel      context.CancelFunc
	running     bool
	reloadEvery time.Duration
}

// CustomerJob is the monitoring configuration for one customer plus the VPN
// source IP / interface used to route the ping through the correct tunnel.
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

type workItem struct {
	customerID int64
	ip         string
	vpnID      *int64
	source     string
	iface      string
	timeout    time.Duration
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

// PingAttempt is a single measurement for one customer.
type PingAttempt struct {
	CustomerID int64
	Success    bool
	LatencyMs  *int
	VPNID      *int64
	FailReason string
}

// PingOutcome is the persisted result after the retry/status state machine runs.
type PingOutcome struct {
	CustomerID   int64
	Status       string
	OldStatus    string
	LatencyMs    *int
	Consecutive  int
	UptimePct    float64
	CustomerJSON map[string]interface{}
}

// EventPublisher broadcasts realtime events to SSE/WebSocket clients.
type EventPublisher interface {
	BroadcastCustomerUpdate(payload map[string]interface{})
	BroadcastStats(payload interface{})
	BroadcastVPNStatus(payload map[string]interface{})
}

const (
	defWorkerCount = 30
	reloadInterval = 30 * time.Second
)

func NewMonitorEngine(db Storer, hub EventPublisher, workerCount int, ensureRoute func(ip, iface string)) *MonitorEngine {
	if workerCount <= 0 {
		workerCount = defWorkerCount
	}
	return &MonitorEngine{
		DB:          db,
		Hub:         hub,
		pinger:      NewICMPPinger(),
		workerCount: workerCount,
		queue:       make(chan workItem, workerCount*4),
		ensureRoute: ensureRoute,
		nextRun:     make(map[int64]time.Time),
		nextSeq:     make(map[int64]int),
		reloadEvery: reloadInterval,
	}
}

// Start launches the scheduler and worker pool.
func (m *MonitorEngine) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.running = true

	for i := 0; i < m.workerCount; i++ {
		go m.worker(ctx)
	}
	go m.scheduler(ctx)

	// Initial load.
	if jobs, err := m.DB.LoadMonitorData(ctx); err == nil {
		m.replaceJobs(jobs)
	}
	log.Printf("[monitor] engine started with %d workers", m.workerCount)
}

func (m *MonitorEngine) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
	log.Println("[monitor] engine stopped")
}

func (m *MonitorEngine) IsRunning() bool { return m.running }

// Reload refreshes the in-memory job list from the database.
func (m *MonitorEngine) Reload() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	jobs, err := m.DB.LoadMonitorData(ctx)
	if err != nil {
		log.Printf("[monitor] load monitor data failed: %v", err)
		return
	}
	m.replaceJobs(jobs)
	log.Printf("[monitor] reloaded %d monitored customers", len(jobs))
}

func (m *MonitorEngine) replaceJobs(jobs []CustomerJob) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs = jobs
	now := time.Now()
	for _, j := range jobs {
		if !j.Monitored {
			delete(m.nextRun, j.CustomerID)
			continue
		}
		next, ok := m.nextRun[j.CustomerID]
		if !ok || next.IsZero() || !next.After(now.Add(time.Duration(j.IntervalSec)*time.Second)) {
			m.nextRun[j.CustomerID] = now
		}
	}
}

func (m *MonitorEngine) scheduler(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	reload := time.NewTicker(m.reloadEvery)
	defer reload.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-reload.C:
			m.Reload()
		case <-ticker.C:
			m.dispatchDue()
		}
	}
}

func (m *MonitorEngine) dispatchDue() {
	now := time.Now()
	m.mu.RLock()
	var due []CustomerJob
	for i := range m.jobs {
		j := &m.jobs[i]
		if !j.Monitored {
			continue
		}
		next, ok := m.nextRun[j.CustomerID]
		if !ok {
			next = now
		}
		if !now.Before(next) {
			due = append(due, *j)
			m.nextSeq[j.CustomerID] = m.nextSeq[j.CustomerID] + 1
			m.nextRun[j.CustomerID] = now.Add(time.Duration(j.IntervalSec) * time.Second)
		}
	}
	m.mu.RUnlock()

	for _, j := range due {
		m.enqueue(j, now)
	}
}

func (m *MonitorEngine) enqueue(j CustomerJob, now time.Time) {
	item := workItem{
		customerID: j.CustomerID,
		ip:         j.IPAddress,
		vpnID:      j.VPNID,
		source:     j.SourceIP,
		iface:      j.Interface,
		timeout:    time.Duration(j.TimeoutMs) * time.Millisecond,
	}
	select {
	case m.queue <- item:
	default:
		log.Printf("[monitor] queue full, scheduling retry for customer %d", j.CustomerID)
		m.mu.Lock()
		m.nextRun[j.CustomerID] = now.Add(2 * time.Second)
		m.mu.Unlock()
	}
}

func (m *MonitorEngine) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-m.queue:
			m.process(ctx, job)
		}
	}
}

func (m *MonitorEngine) process(ctx context.Context, job workItem) {
	tctx, cancel := context.WithTimeout(ctx, 3*time.Second+job.timeout)
	defer cancel()

	if job.iface != "" && m.ensureRoute != nil {
		m.ensureRoute(job.ip, job.iface)
	}

	result := m.pinger.Ping(job.ip, job.source, job.timeout)

	var latency *int
	if result.Success {
		ms := int(result.RTT.Milliseconds())
		latency = &ms
	}

	attempt := PingAttempt{
		CustomerID: job.customerID,
		Success:    result.Success,
		LatencyMs:  latency,
		VPNID:      job.vpnID,
		FailReason: result.ErrorMsg,
	}

	outcome, err := m.DB.ApplyPingOutcome(tctx, attempt)
	if err != nil {
		log.Printf("[monitor] apply outcome failed for customer %d: %v", job.customerID, err)
		return
	}

	if m.Hub != nil && outcome != nil {
		payload := map[string]interface{}{
			"customer_id": outcome.CustomerID,
			"status":      outcome.Status,
			"latency_ms":  outcome.LatencyMs,
		}
		m.Hub.BroadcastCustomerUpdate(payload)
	}
}
