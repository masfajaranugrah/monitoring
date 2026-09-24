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
	idx      int
	name     string
	ip       string
	maps     string
	code     string
	vpn      string
	lat      float64
	lng      float64
	hasCoord bool
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
		ip := cell(colIP)
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
	failed := map[string]struct{}{}
	var busyMu sync.Mutex
	busy := map[string]struct{}{}

	work := make(chan *parsedRow)
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for pr := range work {
			if pr.hasCoord || pr.maps == "" {
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
				_, bad := failed[key]
				failMu.Unlock()
				if bad {
					break
				}
				busyMu.Lock()
				if _, inFlight := busy[key]; inFlight {
					busyMu.Unlock()
					select {
					case <-ctx.Done():
						return
					case <-time.After(40 * time.Millisecond):
					}
					continue
				}
				busy[key] = struct{}{}
				busyMu.Unlock()

				a, b, ok2 := resolveMapsLinkCoord(ctx, key)
				cacheMu.Lock()
				if ok2 {
					cache[key] = [2]float64{a, b}
				}
				cacheMu.Unlock()
				failMu.Lock()
				if !ok2 {
					failed[key] = struct{}{}
				}
				failMu.Unlock()
				busyMu.Lock()
				delete(busy, key)
				busyMu.Unlock()

				if ok2 {
					pr.lat, pr.lng, pr.hasCoord = a, b, true
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

	rowErr := func(pr *parsedRow, msg string) string {
		return fmt.Sprintf("baris %d: %s", pr.idx+1, msg)
	}

	for _, pr := range rows {
		if pr.name == "" {
			skipped++
			if firstSkip == "" {
				firstSkip = rowErr(pr, "nama kosong")
			}
			continue
		}
		if !validateIP(pr.ip) {
			skipped++
			if firstSkip == "" {
				firstSkip = rowErr(pr, "ip tidak valid ("+pr.ip+")")
			}
			continue
		}
		if !pr.hasCoord {
			skipped++
			if firstSkip == "" {
				firstSkip = rowErr(pr, "koordinat tidak ditemukan di link Google Maps ("+pr.maps+"). "+
					"Format link yang didukung: maps.google.com/?q=LAT,LNG, google.com/maps/@LAT,LNG,"+
					" /place/LAT,LNG/data=..., link pendek maps.app.goo.gl / goo.gl/maps, atau alamat / plus code.")
			}
			continue
		}

		code := pr.code
		if code == "" {
			code = generateCustomerCode(pr.ip)
		}

		vpnID, vpnErr := resolveVPNID(ctx, pr.vpn)
		if vpnErr != nil {
			skipped++
			if firstSkip == "" {
				firstSkip = rowErr(pr, "vpn tidak ditemukan: "+pr.vpn)
			}
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
			skipped++
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
				skipped++
				if firstSkip == "" {
					firstSkip = rowErr(pr, execErr.Error())
				}
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
			skipped++
			if firstSkip == "" {
				firstSkip = rowErr(pr, execErr.Error())
			}
			log.Printf("[customer] import lewati baris %d (%s): %v", pr.idx+1, pr.name, execErr)
			continue
		}
		_, _ = tx.Exec(ctx, "RELEASE SAVEPOINT import_customer")
		created++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, "", errors.New("gagal menyimpan impor: " + err.Error())
	}
	return created, updated, skipped, firstSkip, nil
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
