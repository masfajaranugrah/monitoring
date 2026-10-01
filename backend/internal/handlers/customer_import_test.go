package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// TestImportJobSkippedDownload memastikan endpoint unduh mengembalikan file
// Excel berisi baris yang dilewati.
func TestImportJobSkippedDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	job := newImportJob("test-skipped-1")
	finishImportJob(job, &importResult{
		created: 2, updated: 1, skipped: 2, firstSkip: "IP tidak valid (2 baris, contoh baris 4)",
		skipRows: []skippedRow{
			{Row: 4, Code: "C-004", Name: "Budi", IP: "x", Reason: "IP tidak valid"},
			{Row: 7, Name: "Sari", IP: "10.0.0.7", Reason: "koordinat tidak ditemukan: timeout"},
		},
	}, "")
	defer func() {
		importJobsMu.Lock()
		delete(importJobs, job.ID)
		importJobsMu.Unlock()
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/customers/import/"+job.ID+"/skipped", nil)
	c.Params = gin.Params{{Key: "id", Value: job.ID}}

	ImportJobSkipped(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); !bytes.Contains([]byte(cd), []byte("data-terlewat")) {
		t.Fatalf("Content-Disposition = %q", cd)
	}
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("body bukan xlsx valid: %v", err)
	}
	defer f.Close()
	grid, err := f.GetRows("Data Terlewat")
	if err != nil {
		t.Fatalf("baca sheet: %v", err)
	}
	if len(grid) != 3 || grid[1][1] != "Budi" || grid[2][1] != "Sari" {
		t.Fatalf("isi file salah: %v", grid)
	}
}

// TestImportJobSkippedErrors memastikan kondisi tidak ditemukan dan job tanpa
// baris terlewat ditolak dengan pesan yang jelas.
func TestImportJobSkippedErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	run := func(id string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/customers/import/"+id+"/skipped", nil)
		c.Params = gin.Params{{Key: "id", Value: id}}
		ImportJobSkipped(c)
		return w
	}

	if w := run("tidak-ada-job"); w.Code != http.StatusNotFound {
		t.Fatalf("job tak dikenal harus 404, dapat %d", w.Code)
	}

	job := newImportJob("test-skipped-2")
	finishImportJob(job, &importResult{created: 1}, "")
	defer func() {
		importJobsMu.Lock()
		delete(importJobs, job.ID)
		importJobsMu.Unlock()
	}()
	if w := run(job.ID); w.Code != http.StatusNotFound {
		t.Fatalf("job tanpa baris terlewat harus 404, dapat %d", w.Code)
	}
}

// TestImportJobStatusOmitsSkipRows memastikan respons status tetap ringan.
func TestImportJobStatusOmitsSkipRows(t *testing.T) {
	gin.SetMode(gin.TestMode)

	job := newImportJob("test-skipped-3")
	finishImportJob(job, &importResult{
		created: 1, skipped: 1, firstSkip: "IP tidak valid (1 baris, contoh baris 4)",
		skipRows: []skippedRow{{Row: 4, Name: "Budi", IP: "x", Reason: "IP tidak valid"}},
	}, "")
	defer func() {
		importJobsMu.Lock()
		delete(importJobs, job.ID)
		importJobsMu.Unlock()
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/customers/import/"+job.ID, nil)
	c.Params = gin.Params{{Key: "id", Value: job.ID}}

	ImportJobStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if bytes.Contains([]byte(body), []byte("Budi")) {
		t.Fatalf("isi baris terlewat tidak boleh ikut di respons status: %s", body)
	}
	if !bytes.Contains([]byte(body), []byte(`"skipped":1`)) {
		t.Fatalf("jumlah terlewat harus tetap terlihat: %s", body)
	}
}
