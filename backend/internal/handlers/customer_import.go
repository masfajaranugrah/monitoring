package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"monitoring/internal/database"
)

// Impor pelanggan dijalankan secara asinkron (latar belakang) supaya request
// HTTP langsung membalas dengan ID pekerjaan. Ini penting saat server berada
// di belakang Cloudflare/nginx yang membatasi waktu tunggu origin — file dengan
// banyak link Google Maps pendek bisa butuh hitungan menit untuk diproses.

const (
	// maxImportFileSize membatasi besar file .xlsx yang diunggah.
	maxImportFileSize = 25 << 20
	// importJobTimeout membatasi total waktu satu pekerjaan impor.
	importJobTimeout = 10 * time.Minute
	// coordResolveWorkers jumlah goroutine pengekstrak / resolusi koordinat.
	coordResolveWorkers = 8
	// importJobsMaxRetain berapa banyak pekerjaan lama yang ditahan di memori.
	importJobsMaxRetain = 100
)

// ImportJob adalah status satu pekerjaan impor file Excel pelanggan.
type ImportJob struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // running | done | error
	Imported int    `json:"imported"`
	Updated  int    `json:"updated"`
	Skipped  int    `json:"skipped"`
	Detail   string `json:"detail"`
	Error    string `json:"error"`
}

var (
	importJobsMu sync.Mutex
	importJobs   = map[string]*ImportJob{}
)

func randomJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newImportJob(id string) *ImportJob {
	job := &ImportJob{ID: id, Status: "running"}
	importJobsMu.Lock()
	if len(importJobs) >= importJobsMaxRetain {
		for k, old := range importJobs {
			if old.Status != "running" {
				delete(importJobs, k)
			}
			if len(importJobs) < importJobsMaxRetain {
				break
			}
		}
	}
	importJobs[id] = job
	importJobsMu.Unlock()
	return job
}

// parseImportRows membuka file .xlsx, memetakan header baris pertama, dan
// mengembalikan pemetaan kolom + seluruh baris data.
func parseImportRows(data []byte) (map[string]int, [][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, errors.New("file excel tidak valid: " + err.Error())
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, errors.New("file excel kosong")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, errors.New("gagal membaca sheet: " + err.Error())
	}
	if len(rows) < 2 {
		return nil, nil, errors.New("tidak ada data baris untuk diimpor")
	}
	return mapExcelHeaders(rows[0]), rows, nil
}

// ImportCustomers menerima file Excel (.xlsx), memvalidasi secara cepat, lalu
// mengantrekan proses impor ke latar belakang. Balasan HTTP berbentuk 202
// dengan job_id; status dipantau lewat GET /customers/import/:id.
func ImportCustomers(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file excel (.xlsx) wajib diunggah (field: file)"})
		return
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".xlsx") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "hanya file .xlsx yang didukung"})
		return
	}
	fh, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gagal membaca file: " + err.Error()})
		return
	}
	defer fh.Close()

	data, err := io.ReadAll(io.LimitReader(fh, maxImportFileSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gagal membaca file: " + err.Error()})
		return
	}
	if len(data) > maxImportFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ukuran file terlalu besar (maks 25 MB)"})
		return
	}

	// Validasi cepat supaya file rusak ditolak segera (tanpa menunggu job).
	if _, _, err := parseImportRows(data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	job := newImportJob(randomJobID())
	go runCustomerImportJob(job, data)

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "status": "running"})
}

// ImportJobStatus mengembalikan status pekerjaan impor.
func ImportJobStatus(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	importJobsMu.Lock()
	job, ok := importJobs[id]
	importJobsMu.Unlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "pekerjaan impor tidak ditemukan"})
		return
	}
	c.JSON(http.StatusOK, job)
}

// parsedRow adalah satu baris data pelanggan hasil bacaan Excel.
type parsedRow struct {
	idx        int
	name       string
	ip         string
	maps       string
	code       string
	vpn        string
	lat        float64
	lng        float64
	hasCoord   bool
	coordError string // alasan koordinat tidak ditemukan (untuk pesan hasil)
}

// runCustomerImportJob memproses file impor di goroutine latar belakang:
//
//  1. koordinat semua baris diekstrak secara paralel (memangkas waktu tunggu),
//  2. hasilnya disimpan ke database dalam satu transaksi.
func runCustomerImportJob(job *ImportJob, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), importJobTimeout)
	defer cancel()

	colIdx, rows, err := parseImportRows(data)
	if err != nil {
		finishImportJob(job, nil, err.Error())
		return
	}

	parsed := buildParsedRows(colIdx, rows)

	// Ekstrak koordinat dari link Google Maps secara paralel, dengan cache
	// per-file supaya link yang sama tidak di-resolve berulang.
	resolveCoordsParallel(ctx, parsed)

	created, updated, skipped, firstSkip, err := persistImportRows(ctx, parsed)
	if err != nil {
		finishImportJob(job, nil, err.Error())
		return
	}

	if created+updated > 0 || firstSkip == "" {
		nudgeMonitor()
	}

	res := &importResult{created: created, updated: updated, skipped: skipped, firstSkip: firstSkip}
	finishImportJob(job, res, "")
}

// buildParsedRows menerjemahkan baris Excel menjadi daftar parsedRow, memuat
// koordinat eksplisit dari kolom latitude/longitude bila tersedia.
func buildParsedRows(colIdx map[string]int, rows [][]string) []*parsedRow {
	var out []*parsedRow
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		cell := func(key string) string {
			idx, ok := colIdx[key]
			if !ok || idx >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[idx])
		}
		name := cell(colName)
		ip := normalizeIP(cell(colIP))
		maps := cell(colMaps)
		if name == "" && ip == "" && maps == "" {
			continue
		}
		pr := &parsedRow{idx: i, name: name, ip: ip, maps: maps, code: cell(colCustomerCode), vpn: cell(colVPN)}
		// Dukungan kolom latitude/longitude terpisah (opsional).
		if latS := cell(colLatitude); latS != "" {
			if lngS := cell(colLongitude); lngS != "" {
				if a, e1 := strconv.ParseFloat(latS, 64); e1 == nil {
					if b, e2 := strconv.ParseFloat(lngS, 64); e2 == nil && validateCoord(a, b) {
						pr.lat, pr.lng, pr.hasCoord = a, b, true
					}
				}
			}
		}
		out = append(out, pr)
	}
	return out
}

// resolveCoordsParallel mengekstrak koordinat semua baris secara paralel.
// Hasil tiap link di-cache dalam satu file impor; untuk link yang sama yang
// diproses bersamaan dipakai mekanisme single-flight sehingga tiap link hanya
// di-resolve satu kali (termasuk yang gagal, supaya tidak diulang-ulang).
func resolveCoordsParallel(ctx context.Context, rows []*parsedRow) {
	if len(rows) == 0 {
		return
	}
	var cacheMu sync.Mutex
	cache := map[string][2]float64{}
	var failMu sync.Mutex
	failed := map[string]string{}
	var busyMu sync.Mutex
	busy := map[string]struct{}{}

	work := make(chan *parsedRow)
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for pr := range work {
			if pr.hasCoord {
				continue
			}
			if pr.maps == "" {
				pr.coordError = "kolom Link Google Maps kosong"
				continue
			}
			key := pr.maps
			for {
				cacheMu.Lock()
				v, ok := cache[key]
				cacheMu.Unlock()
				if ok {
					pr.lat, pr.lng, pr.hasCoord = v[0], v[1], true
					break
				}
				failMu.Lock()
				reason, bad := failed[key]
				failMu.Unlock()
				if bad {
					pr.coordError = reason
					break
				}
				busyMu.Lock()
				if _, inFlight := busy[key]; inFlight {
					busyMu.Unlock()
					select {
					case <-ctx.Done():
						pr.coordError = "resolusi link dihentikan karena batas waktu impor"
						return
					case <-time.After(40 * time.Millisecond):
					}
					continue
				}
				busy[key] = struct{}{}
				busyMu.Unlock()

				a, b, ok2, reason := resolveMapsLinkCoordWithRetry(ctx, key)
				cacheMu.Lock()
				if ok2 {
					cache[key] = [2]float64{a, b}
				}
				cacheMu.Unlock()
				failMu.Lock()
				if !ok2 {
					failed[key] = reason
				}
				failMu.Unlock()
				busyMu.Lock()
				delete(busy, key)
				busyMu.Unlock()

				if ok2 {
					pr.lat, pr.lng, pr.hasCoord = a, b, true
				} else {
					pr.coordError = reason
					log.Printf("[customer] impor: link %q gagal dibaca koordinat: %s", key, reason)
				}
				break
			}
		}
	}
	for w := 0; w < coordResolveWorkers; w++ {
		wg.Add(1)
		go worker()
	}
	for _, pr := range rows {
		select {
		case work <- pr:
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return
		}
	}
	close(work)
	wg.Wait()
}

// resolveMapsLinkCoordWithRetry mencoba resolve link beberapa kali. Kegagalan
// jaringan sementara (timeout, reset koneksi, DNS) sangat umum saat membuka
// banyak link Google Maps berurutan, sehingga satu percobaan terlalu rapuh.
func resolveMapsLinkCoordWithRetry(ctx context.Context, maps string) (float64, float64, bool, string) {
	const attempts = 3
	var (
		lat, lng float64
		reason   string
	)
	for i := 0; i < attempts; i++ {
		if i > 0 {
			if !sleepCtx(ctx, time.Duration(i)*time.Second) {
				break
			}
		}
		var ok bool
		lat, lng, ok, reason = resolveMapsLinkCoordReason(ctx, maps)
		if ok {
			return lat, lng, true, ""
		}
		// Kegagalan format/geocoding tidak akan berubah dengan mengulang.
		if !isTransientCoordFailure(reason) {
			break
		}
	}
	return 0, 0, false, reason
}

// isTransientCoordFailure menandai kegagalan yang layak dicoba ulang.
func isTransientCoordFailure(reason string) bool {
	return strings.Contains(reason, "tidak bisa dibuka") ||
		strings.Contains(reason, "tanpa koordinat") ||
		strings.Contains(reason, "timeout") ||
		strings.Contains(reason, "koneksi")
}

type importResult struct {
	created   int
	updated   int
	skipped   int
	firstSkip string
}

// persistImportRows menyimpan seluruh baris ke database dalam satu transaksi.
// Setiap baris disimpan dalam SAVEPOINT-nya sendiri sehingga satu baris yang
// gagal tidak menggagalkan baris lain.
func persistImportRows(ctx context.Context, rows []*parsedRow) (created, updated, skipped int, firstSkip string, err error) {
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, "", errors.New("gagal mulai transaksi")
	}
	defer tx.Rollback(ctx)

	// groups mengumpulkan semua alasan dilewati (bukan hanya yang pertama),
	// supaya pengguna bisa memperbaiki file Excel-nya sekaligus.
	var order []string
	groups := map[string]*skipReason{}
	skipWith := func(pr *parsedRow, key, msg string) {
		skipped++
		g, ok := groups[key]
		if !ok {
			g = &skipReason{label: msg}
			groups[key] = g
			order = append(order, key)
		}
		g.count++
		if g.sample == "" {
			g.sample = fmt.Sprintf("baris %d", pr.idx+1)
		}
	}

	for _, pr := range rows {
		if pr.name == "" {
			skipWith(pr, "nama", "nama pelanggan kosong")
			continue
		}
		if !validateIP(pr.ip) {
			skipWith(pr, "ip", fmt.Sprintf("IP tidak valid (%q) — gunakan titik, contoh 10.111.210.57", pr.ip))
			continue
		}
		if !pr.hasCoord {
			why := pr.coordError
			if why == "" {
				why = "koordinat tidak ditemukan"
			}
			skipWith(pr, "coord:"+why, "koordinat tidak ditemukan: "+why)
			continue
		}

		code := pr.code
		if code == "" {
			code = generateCustomerCode(pr.ip)
		}

		vpnID, vpnErr := resolveVPNID(ctx, pr.vpn)
		if vpnErr != nil {
			skipWith(pr, "vpn", "VPN tidak ditemukan: "+pr.vpn)
			continue
		}

		if _, execErr := tx.Exec(ctx, "SAVEPOINT import_customer"); execErr != nil {
			return 0, 0, 0, "", errors.New("gagal mulai impor: " + execErr.Error())
		}

		// ID/kode yang sudah dipakai pelanggan → update data lama, bukan duplikat.
		var exists bool
		if qErr := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM customers WHERE customer_code = $1)`, code).Scan(&exists); qErr != nil {
			_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT import_customer")
			_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT import_customer")
			skipWith(pr, "db", "cek kode pelanggan gagal: "+qErr.Error())
			log.Printf("[customer] import cek kode baris %d: %v", pr.idx+1, qErr)
			continue
		}
		if exists {
			_, execErr := tx.Exec(ctx,
				`UPDATE customers SET customer_name=$1, ip_address=$2, latitude=$3, longitude=$4,
				   location=ST_SetSRID(ST_MakePoint($4, $3), 4326), vpn_id=$5, updated_at=now()
				 WHERE customer_code=$6`,
				pr.name, pr.ip, pr.lat, pr.lng, vpnID, code)
			if execErr != nil {
				_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT import_customer")
				_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT import_customer")
				skipWith(pr, "db", "gagal memperbarui: "+execErr.Error())
				log.Printf("[customer] import update gagal baris %d: %v", pr.idx+1, execErr)
				continue
			}
			_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT import_customer")
			updated++
			continue
		}

		_, execErr := tx.Exec(ctx,
			`INSERT INTO customers
			 (customer_code, customer_name, ip_address, latitude, longitude, location,
			  vpn_id, icon, description, monitoring_enabled, ping_interval, timeout_ms, retry_count)
			 VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($5, $4), 4326),
			         $6, 'customer', '', true, 300, 1000, 2)`,
			code, pr.name, pr.ip, pr.lat, pr.lng, vpnID)
		if execErr != nil {
			_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT import_customer")
			_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT import_customer")
			skipWith(pr, "db", "gagal menyimpan: "+execErr.Error())
			log.Printf("[customer] import lewati baris %d (%s): %v", pr.idx+1, pr.name, execErr)
			continue
		}
		_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT import_customer")
		created++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, "", errors.New("gagal menyimpan impor: " + err.Error())
	}
	if skipped > 0 {
		log.Printf("[customer] impor: %d baris dilewati — %s", skipped, summarizeSkipReasons(order, groups))
	}
	return created, updated, skipped, summarizeSkipReasons(order, groups), nil
}

// skipReason mengelompokkan baris yang dilewati karena alasan yang sama.
type skipReason struct {
	label  string
	count  int
	sample string
}

// summarizeSkipReasons merangkum alasan baris yang dilewati beserta jumlahnya,
// diurutkan dari yang paling sering terjadi.
func summarizeSkipReasons(order []string, groups map[string]*skipReason) string {
	if len(order) == 0 {
		return ""
	}
	list := make([]skipReason, 0, len(order))
	for _, key := range order {
		list = append(list, *groups[key])
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].count > list[j].count })

	const maxReasons = 4
	var b strings.Builder
	for i, e := range list {
		if i >= maxReasons {
			fmt.Fprintf(&b, "; %d alasan lain", len(list)-maxReasons)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s (%d baris, contoh %s)", e.label, e.count, e.sample)
	}
	return b.String()
}

// finishImportJob menulis hasil pekerjaan ke penyimpanan status.
func finishImportJob(job *ImportJob, res *importResult, importErr string) {
	importJobsMu.Lock()
	defer importJobsMu.Unlock()

	job.Status = "done"
	if res == nil {
		job.Status = "error"
		job.Error = importErr
		return
	}
	job.Imported = res.created
	job.Updated = res.updated
	job.Skipped = res.skipped
	job.Detail = res.firstSkip
	// Tanpa satu pun baris berhasil → laporkan sebagai kegagalan (alur lama).
	if res.created+res.updated == 0 && res.firstSkip != "" {
		job.Error = "tidak ada pelanggan yang berhasil diimpor · " + res.firstSkip
	}
}
