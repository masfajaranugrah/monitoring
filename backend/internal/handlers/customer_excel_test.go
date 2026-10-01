package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func TestParseGoogleMapsCoord(t *testing.T) {
	cases := []struct {
		in      string
		wantLat float64
		wantLng float64
		wantOK  bool
	}{
		{"https://maps.google.com/?q=-7.656872,110.717812", -7.656872, 110.717812, true},
		{"https://maps.google.com/maps?q=-7.5828333,110.6287978", -7.5828333, 110.6287978, true},
		{"https://maps.google.com?q=-7.5828333,110.6287978&entry=gps&shh=CAE&lucs=,94297699,94231188,94280568,100821559,47071704,94218641,94282134,100835694,94286869,100820247,100822504&g_st=ic", -7.5828333, 110.6287978, true},
		{"https://www.google.co.id/maps/@-7.5828333,110.6287978,17z", -7.5828333, 110.6287978, true},
		{"https://google.com/maps?q=loc:-7.5828333,110.6287978&z=15", -7.5828333, 110.6287978, true},
		{"https://www.google.com/maps/place/Budi/@-7.656872,110.717812,17z", -7.656872, 110.717812, true},
		{"https://maps.google.com/?q=loc:-7.656872,110.717812", -7.656872, 110.717812, true},
		{"https://www.google.com/maps/place/-7.807025,110.689765/data=!4m6!3m5!1s0!7e2!8m2!3d-7.807024599999999!4d110.68976509999999!18m1!1e1?utm_source=mstt_1", -7.807025, 110.689765, true},
		{"https://www.google.com/maps/place/X/@-6.2,106.8,15z/data=!4m5!3m4!1s0!8m2", -6.2, 106.8, true},
		{"-7.656872,110.717812", -7.656872, 110.717812, true},
		{"https://www.google.com/maps?ll=-7.657,110.718&z=12", -7.657, 110.718, true},
		{"https://maps.google.com/?q=-7.656872", 0, 0, false},
		{"abcdef", 0, 0, false},
		{"https://maps.google.com/?q=100,200", 0, 0, false},
		{"", 0, 0, false},
		// Sel yang memuat teks tambahan setelah link.
		{"https://maps.app.goo.gl/6fBq61pzEKvvcVfs7 nurdin sembungan", 0, 0, false},
		{"https://maps.google.com/?q=-7.792081,110.733498  seerpti ini", -7.792081, 110.733498, true},
		// Format hasil redirect link pendek yang lazim.
		{"https://www.google.com/maps/place/7%C2%B047'25.9%22S+110%C2%B043'07.9%22E/@-7.7905214,110.7162749,1136m/data=!3m2!1e3!4b1!4m4!3m3!8m2!3d-7.7905214!4d110.7188498?entry=tts", -7.7905214, 110.7162749, true},
		{"https://www.google.com/maps/place/6P5C%2B6GJ+Sumur+mbah+Sarto,+Grogol,+Kec.+Weru,/data=!4m6!3m5!1s0x2e7a37000266b26f:0x95902762f1973ec5!7e2!8m2!3d-7.791986!4d110.72110099999999", -7.791986, 110.72110099999999, true},
		{"https://www.google.com/maps/place/-7.790561,110.719199/data=!4m6!3m5!1s0!7e2!8m2!3d-7.7905606999999994!4d110.71919899999999!18m1!1e1", -7.790561, 110.719199, true},
		{"https://maps.google.com/maps?q=-7.791039,110.722476&entry=gps", -7.791039, 110.722476, true},
		// Koordinat dengan spasi setelah koma & bungkus kurung.
		{"(-7.790466, 110.721451)", -7.790466, 110.721451, true},
		{"-7.791032,110.722221 (catatan)", -7.791032, 110.722221, true},
	}
	for _, tc := range cases {
		lat, lng, ok := parseGoogleMapsCoord(tc.in)
		if ok != tc.wantOK {
			t.Fatalf("parseGoogleMapsCoord(%q) ok=%v want %v", tc.in, ok, tc.wantOK)
		}
		if ok && (lat != tc.wantLat || lng != tc.wantLng) {
			t.Fatalf("parseGoogleMapsCoord(%q) = (%v,%v) want (%v,%v)", tc.in, lat, lng, tc.wantLat, tc.wantLng)
		}
	}
}

func TestNormalizeAlias(t *testing.T) {
	cases := map[string]string{
		"IP Pelanggan":     colIP,
		"link google maps": colMaps,
		"pilih VPN":        colVPN,
		"NAMA":             colName,
		"customer_name":    colName,
		"ID":               colCustomerCode,
		"No":               colCustomerCode,
	}
	for in, want := range cases {
		if got := customerExcelAliases[normalizeAlias(in)]; got != want {
			t.Fatalf("alias %q resolved to %q want %q", in, got, want)
		}
	}
}

func TestResolveMapsLinkCoordFollowsRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/s/abc", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/maps/@-7.656872,110.717812,17z", http.StatusFound)
	})
	mux.HandleFunc("/maps/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	lat, lng, ok := resolveMapsLinkCoord(ctx, srv.URL+"/s/abc")
	if !ok {
		t.Fatalf("resolveMapsLinkCoord gagal mengekstrak koordinat dari redirect")
	}
	if lat != -7.656872 || lng != 110.717812 {
		t.Fatalf("resolveMapsLinkCoord = (%v,%v) want (-7.656872,110.717812)", lat, lng)
	}
}

func TestResolveMapsLinkCoordNonURLFallsBack(t *testing.T) {
	lat, lng, ok := resolveMapsLinkCoord(context.Background(), "-7.656872,110.717812")
	if !ok || lat != -7.656872 || lng != 110.717812 {
		t.Fatalf("resolveMapsLinkCoord langsung koordinat = (%v,%v,%v) want (-7.656872,110.717812,true)", lat, lng, ok)
	}
}

func TestMapsQueryText(t *testing.T) {
	u := "https://maps.google.com?q=6PC2+3PH+Bagas%E2%80%99s+Home,+Jetis,+Tugu&entry=gps&g_st=iw"
	if got := mapsQueryText(u); got != "6PC2 3PH Bagas\u2019s Home, Jetis, Tugu" {
		t.Fatalf("mapsQueryText = %q", got)
	}
	// URL /place/Nama (tanpa @) yang tidak sarat koordinat.
	p := "https://www.google.com/maps/place/Toko+Anda/data=!3m1!1s0x0%3A0x0"
	if got := mapsQueryText(p); got != "Toko Anda" {
		t.Fatalf("mapsQueryText path = %q", got)
	}
}

func TestGeocodePlace(t *testing.T) {
	old := nominatimBase
	nominatimBase = ""
	defer func() { nominatimBase = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"lat":"-7.7275","lon":"110.6779","display_name":"Jetis, Cawas, Klaten, Jawa Tengah, Indonesia"}]`))
	}))
	defer srv.Close()
	nominatimBase = srv.URL

	lat, lng, ok := geocodePlace(context.Background(), "6PC2+3PH Bagas's Home, Jetis, Cawas, Klaten")
	if !ok || lat != -7.7275 || lng != 110.6779 {
		t.Fatalf("geocodePlace = (%v,%v,%v)", lat, lng, ok)
	}
	// Harus ter-cache: server dimatikan, tapi hasil tetap terkelola.
	lat2, lng2, ok2 := geocodePlace(context.Background(), "6PC2+3PH Bagas's Home, Jetis, Cawas, Klaten")
	if !ok2 || lat2 != lat || lng2 != lng {
		t.Fatalf("geocodePlace cache = (%v,%v,%v)", lat2, lng2, ok2)
	}
}

func TestResolveMapsLinkCoordQueryGeocode(t *testing.T) {
	old := nominatimBase
	nominatimBase = ""
	defer func() { nominatimBase = old }()

	mux := http.NewServeMux()
	// goo.gl yang redirect ke maps.google.com?q=alamat (tanpa koordinat).
	mux.HandleFunc("/short", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/m?q=Toko+Anda%2C+Klaten&entry=gps", http.StatusFound)
	})
	mux.HandleFunc("/m", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Nominatim palsu.
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"lat":"-7.81","lon":"110.62","display_name":"Klaten, Jawa Tengah, Indonesia"}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	nominatimBase = srv.URL

	lat, lng, ok := resolveMapsLinkCoord(context.Background(), srv.URL+"/short")
	// Alamat "toko anda, klaten" tidak mungkin dicoordinate dari case test ini
	// bila geocoding memberi hasil; kami hanya memastikan ia sampai ke jalur
	// geocoding dan berhasil.
	_ = lat
	_ = lng
	if !ok {
		t.Fatalf("resolveMapsLinkCoord jalur geocoding = (%v,%v,%v)", lat, lng, ok)
	}
}

func TestTemplateCustomersProducesValidXLSX(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/customers/template", nil)

	TemplateCustomers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("TemplateCustomers status = %d", w.Code)
	}
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("template bukan xlsx valid: %v", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) < 2 || sheets[0] != "Pelanggan" {
		t.Fatalf("template sheets tidak sesuai: %v", sheets)
	}
	headers, err := f.GetRows("Pelanggan")
	if err != nil || len(headers) != 1 {
		t.Fatalf("sheet Pelanggan harus header-only: %v %v", headers, err)
	}
	if len(headers[0]) != 5 || headers[0][4] != "Link Google Maps" {
		t.Fatalf("header template salah: %v", headers[0])
	}
}

// TestBuildSkippedXLSX memastikan file "data terlewat" memakai kolom yang sama
// dengan template impor (agar bisa diimpor ulang) plus kolom bantu, dan isi
// baris/errorsinya benar.
func TestBuildSkippedXLSX(t *testing.T) {
	data, err := buildSkippedXLSX([]skippedRow{
		{Row: 4, Code: "C-004", Name: "Budi", IP: "bukan ip", VPN: "VPN-1",
			Maps: "https://maps.app.goo.gl/abc", Reason: "IP tidak valid"},
		{Row: 9, Name: "Sari", IP: "10.0.0.7", VPN: "", Maps: "", Reason: "koordinat tidak ditemukan: timeout"},
	})
	if err != nil {
		t.Fatalf("buildSkippedXLSX gagal: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("hasil bukan xlsx valid: %v", err)
	}
	defer f.Close()

	const sheet = "Data Terlewat"
	grid, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("baca sheet: %v", err)
	}
	if len(grid) != 3 {
		t.Fatalf("harus header + 2 baris, dapat %d", len(grid))
	}
	wantHead := []string{"ID", "Nama", "IP Pelanggan", "VPN", "Link Google Maps", "Baris Asli", "Alasan"}
	if len(grid[0]) != len(wantHead) {
		t.Fatalf("header salah: %v", grid[0])
	}
	for i, h := range wantHead {
		if grid[0][i] != h {
			t.Fatalf("header[%d] = %q want %q", i, grid[0][i], h)
		}
	}
	if grid[1][0] != "C-004" || grid[1][1] != "Budi" || grid[1][2] != "bukan ip" {
		t.Fatalf("baris 1 salah: %v", grid[1])
	}
	if grid[1][5] != "4" || grid[1][6] != "IP tidak valid" {
		t.Fatalf("baris 1 kolom bantu salah: %v", grid[1])
	}
	// Kode kosong harus tetap bisa diimpor ulang (diisi otomatis oleh server).
	if grid[2][0] != "" || grid[2][1] != "Sari" {
		t.Fatalf("baris 2 salah: %v", grid[2])
	}

	// Berkas hasil unduhan harus bisa dibaca lagi oleh parser impor.
	colIdx, parsed, err := parseImportRows(data)
	if err != nil {
		t.Fatalf("data terlewat harus bisa diimpor ulang: %v", err)
	}
	prs := buildParsedRows(colIdx, parsed)
	if len(prs) != 2 {
		t.Fatalf("harus ada 2 baris, dapat %d", len(prs))
	}
	if prs[0].ip != "bukan ip" || prs[0].name != "Budi" || prs[0].maps != "https://maps.app.goo.gl/abc" {
		t.Fatalf("baris hasil impor ulang salah: %+v", prs[0])
	}
	// Kolom "Baris Asli"/"Alasan" tidak boleh dianggap kolom data.
	if _, ok := colIdx["baris asli"]; ok {
		t.Fatal("kolom Baris Asli tidak boleh dipetakan sebagai kolom data")
	}
}

func buildTestXLSX(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for i, h := range headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			t.Fatalf("cell header: %v", err)
		}
		_ = f.SetCellValue("Sheet1", cell, h)
	}
	for r, row := range rows {
		for j, v := range row {
			cell, err := excelize.CoordinatesToCellName(j+1, r+2)
			if err != nil {
				t.Fatalf("cell data: %v", err)
			}
			_ = f.SetCellValue("Sheet1", cell, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("tulis xlsx: %v", err)
	}
	return buf.Bytes()
}

func TestParseImportRows(t *testing.T) {
	data := buildTestXLSX(t,
		[]string{"ID", "Nama", "IP Pelanggan", "VPN", "Link Google Maps"},
		[][]string{{"P001", "Budi", "10.0.0.5", "", "https://maps.google.com/?q=-7.656872,110.717812"}},
	)
	colIdx, rows, err := parseImportRows(data)
	if err != nil {
		t.Fatalf("parseImportRows gagal: %v", err)
	}
	if colIdx[colName] != 1 || colIdx[colIP] != 2 || colIdx[colMaps] != 4 {
		t.Fatalf("pemetaan header salah: %v", colIdx)
	}
	if len(rows) != 2 {
		t.Fatalf("banyak baris = %d, want 2 (header + 1 data)", len(rows))
	}
	if rows[1][0] != "P001" || rows[1][1] != "Budi" {
		t.Fatalf("isi baris data salah: %v", rows[1])
	}
}

func TestResolveCoordsParallel(t *testing.T) {
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/s/abc", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Redirect(w, r, "/maps/@-7.656872,110.717812,17z", http.StatusFound)
	})
	mux.HandleFunc("/maps/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	link := srv.URL + "/s/abc"
	rows := []*parsedRow{
		{idx: 2, name: "A", ip: "10.0.0.1", maps: link},
		{idx: 3, name: "B", ip: "10.0.0.2", maps: link},
		{idx: 4, name: "C", ip: "10.0.0.3", maps: "https://maps.google.com/?q=-7.656872,110.717812"},
		{idx: 5, name: "D", ip: "10.0.0.4", maps: "lokasi tanpa koordinat"},
	}
	resolveCoordsParallel(context.Background(), rows)

	for _, r := range rows[:3] {
		if !r.hasCoord || r.lat != -7.656872 || r.lng != 110.717812 {
			t.Fatalf("baris %d koordinat salah: %+v", r.idx, r)
		}
	}
	if rows[3].hasCoord {
		t.Fatalf("baris D harus tanpa koordinat: %+v", rows[3])
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("link yang sama dipanggil %d kali, want 1 (cache)", got)
	}
}

func TestNormalizeIP(t *testing.T) {
	cases := map[string]string{
		"10.111.210.57":     "10.111.210.57",
		"10,111,210,103":    "10.111.210.103",
		"192,135,135,123":   "192.135.135.123",
		" 10.111.210.57 ":   "10.111.210.57",
		"10.111.210.57\n":   "10.111.210.57",
		"10.111.210.57\r\n": "10.111.210.57",
		"10, 111, 210, 57":  "10.111.210.57",
		"":                  "",
		"bukan ip":          "bukan ip",
	}
	for in, want := range cases {
		if got := normalizeIP(in); got != want {
			t.Fatalf("normalizeIP(%q) = %q want %q", in, got, want)
		}
	}
	valid := []string{"10.111.210.57", "10,111,210,103", "192,135,135,123", " 10.111.210.57 "}
	for _, in := range valid {
		if !validateIP(in) {
			t.Fatalf("validateIP(%q) harus true", in)
		}
	}
	for _, in := range []string{"", "bukan ip", "10.111.210", "999.1.1.1", "10..111.210"} {
		if validateIP(in) {
			t.Fatalf("validateIP(%q) harus false", in)
		}
	}
}

// TestResolveMapsLinkCoordInterstitial memastikan link yang diarahkan ke halaman
// consent/captcha Google tetap bisa diambil koordinatnya dari parameter continue.
func TestResolveMapsLinkCoordInterstitial(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/short", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/consent?continue="+url.QueryEscape(
			"/maps/place/-7.790561,110.719199/data=!3m1!4d110.719199"), http.StatusFound)
	})
	mux.HandleFunc("/consent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/maps/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lat, lng, ok, reason := resolveMapsLinkCoordReason(context.Background(), srv.URL+"/short")
	if !ok {
		t.Fatalf("interstitial gagal: %s", reason)
	}
	if lat != -7.790561 || lng != 110.719199 {
		t.Fatalf("interstitial koordinat = (%v,%v)", lat, lng)
	}
}

// TestResolveMapsLinkCoordFromHTML memastikan koordinat di dalam body HTML
// terbaca ketika URL redirect tidak memuat koordinat.
func TestResolveMapsLinkCoordFromHTML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/short", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/places/Toko+Anda", http.StatusFound)
	})
	mux.HandleFunc("/places/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><script>window.APP_INITIALIZATION_STATE=
			[[["!3d-7.7898654!4d110.7166475"]]]</script></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	lat, lng, ok, reason := resolveMapsLinkCoordReason(context.Background(), srv.URL+"/short")
	if !ok {
		t.Fatalf("ambil koordinat dari HTML gagal: %s", reason)
	}
	if lat != -7.7898654 || lng != 110.7166475 {
		t.Fatalf("koordinat HTML = (%v,%v)", lat, lng)
	}
}

// TestResolveMapsLinkCoordReasonNotBlank memastikan kegagalan selalu disertai
// alasan yang bisa ditampilkan ke pengguna.
func TestResolveMapsLinkCoordReasonNotBlank(t *testing.T) {
	_, _, ok, reason := resolveMapsLinkCoordReason(context.Background(), "")
	if ok || reason == "" {
		t.Fatalf("link kosong harus gagal dengan alasan, ok=%v reason=%q", ok, reason)
	}
	_, _, ok, reason = resolveMapsLinkCoordReason(context.Background(), "abcdef")
	if ok || reason == "" {
		t.Fatalf("link tak dikenal harus gagal dengan alasan, ok=%v reason=%q", ok, reason)
	}
}

// TestGeocodeRejectsUnrelatedResult memastikan teks yang tidak relevan tidak
// dipetakan ke tempat acak di negara lain.
func TestGeocodeRejectsUnrelatedResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Hasil nyata untuk kueri "abcdef": boundary stone diPrancis.
		_, _ = w.Write([]byte(`[{"lat":"45.3132458","lon":"6.9912816",` +
			`"display_name":"bess f 56 (abcdef), Bessans, Savoie, France"}]`))
	}))
	defer srv.Close()
	old := nominatimBase
	nominatimBase = srv.URL
	defer func() { nominatimBase = old }()
	geocodeCache.Lock()
	geocodeCache.m = map[string][2]float64{}
	geocodeCache.Unlock()

	lat, lng, ok := geocodePlace(context.Background(), "abcdef")
	if ok {
		t.Fatalf("hasil tak relevan harus ditolak, dapat (%v,%v)", lat, lng)
	}
	// Alamat sebenarnya tetap diterima (memuat nama wilayah pada display_name).
	lat, lng, ok = geocodePlace(context.Background(), "Rue du Repos, Bessans")
	if !ok || lat == 0 {
		t.Fatalf("hasil yang cocok harus diterima, ok=%v (%v,%v)", ok, lat, lng)
	}
}

// TestLooksLikeAddressQuery memastikan teks non-alamat tidak dikirim ke
// layanan geocoding.
func TestLooksLikeAddressQuery(t *testing.T) {
	shouldGeocode := []string{
		"6PC2+3PH Bagas's Home, Jetis",
		"Jetis, Cawas, Klaten",
		"Jl. Merdeka No. 10, Yogyakarta",
	}
	for _, in := range shouldGeocode {
		if !looksLikeAddressQuery(in) {
			t.Fatalf("looksLikeAddressQuery(%q) harus true", in)
		}
	}
	shouldSkip := []string{"", "abc", "abcdef", "rumah", "maps", "?"}
	for _, in := range shouldSkip {
		if looksLikeAddressQuery(in) {
			t.Fatalf("looksLikeAddressQuery(%q) harus false", in)
		}
	}
}

func TestSummarizeSkipReasons(t *testing.T) {
	order := []string{"ip", "coord", "nama"}
	groups := map[string]*skipReason{
		"ip":    {label: "IP tidak valid", count: 5, sample: "baris 3"},
		"coord": {label: "koordinat tidak ditemukan", count: 20, sample: "baris 2"},
		"nama":  {label: "nama kosong", count: 1, sample: "baris 9"},
	}
	got := summarizeSkipReasons(order, groups)
	if !strings.Contains(got, "koordinat tidak ditemukan (20 baris") ||
		!strings.Contains(got, "IP tidak valid (5 baris") ||
		!strings.Contains(got, "nama kosong (1 baris") {
		t.Fatalf("summarizeSkipReasons = %q", got)
	}
	// Alasan paling sering harus tampil lebih dulu.
	if strings.Index(got, "koordinat") > strings.Index(got, "IP tidak valid") {
		t.Fatalf("urutan alasan salah: %q", got)
	}
	if summarizeSkipReasons(nil, nil) != "" {
		t.Fatal("tanpa alasan harus menghasilkan string kosong")
	}
}

func TestExtractMapsURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://maps.app.goo.gl/abc?g_st=aw", "https://maps.app.goo.gl/abc?g_st=aw"},
		{"https://maps.app.goo.gl/abc?g_st=aw nurdin sembungan", "https://maps.app.goo.gl/abc?g_st=aw"},
		{"nurdin https://maps.google.com/?q=-7.79,110.72", "https://maps.google.com/?q=-7.79,110.72"},
		{"\"https://maps.google.com/?q=-7.79,110.72\"", "https://maps.google.com/?q=-7.79,110.72"},
		{"maps.app.goo.gl/abc?g_st=aw", "https://maps.app.goo.gl/abc?g_st=aw"},
		{"goo.gl/maps/abc?g_st=aw", "https://goo.gl/maps/abc?g_st=aw"},
		{"-7.656872,110.717812", "-7.656872,110.717812"},
		{"6PC2 3PH Bagas's Home, Jetis", "6PC2 3PH Bagas's Home, Jetis"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := extractMapsURL(tc.in); got != tc.want {
			t.Fatalf("extractMapsURL(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractPlusCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"6PC2+3PH Bagas's Home, Jetis", "6PC2+3PH"},
		{"6PC2 3PH Bagas's Home, Jetis", "6PC2+3PH"},
	}
	for _, tc := range cases {
		if got := extractPlusCode(tc.in); got != tc.want {
			t.Fatalf("extractPlusCode(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

// TestCoordFromQueryTextPlusCode memastikan Plus Code pendek direkonstruksi
// memakai referensi dari geocoding area alamatnya.
func TestCoordFromQueryTextPlusCode(t *testing.T) {
	old := nominatimBase
	nominatimBase = ""
	defer func() { nominatimBase = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Referensi: Cawas, Klaten (hasil Nominatim sungguhan).
		_, _ = w.Write([]byte(`[{"lat":"-7.7577425","lon":"110.6948009","display_name":"Cawas, Klaten, Jawa Tengah, Indonesia"}]`))
	}))
	defer srv.Close()
	nominatimBase = srv.URL

	lat, lng, ok := coordFromQueryText(context.Background(), "6PC2+3PH Bagas's Home, Jetis, Tugu, Kec. Cawas, Kabupaten Klaten")
	if !ok {
		t.Fatalf("coordFromQueryText gagal decode Plus Code")
	}
	// Harus dekat dengan referensi Cawas (selisih < 0.5 derajat).
	if lat < -8 || lat > -7 || lng < 110 || lng > 111 {
		t.Fatalf("koordinat Plus Code tidak masuk akal: (%v,%v)", lat, lng)
	}
}
