package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/open-location-code/go"
	"github.com/xuri/excelize/v2"

	"monitoring/internal/database"
)

// ---- Google Maps link → koordinat ----

// parseCoordPair membaca pasangan "lat,lng" (atau "lat,lng,zoom") dari string.
// Toleran terhadap tanda kurung/quotes, teks sisa setelah koordinat (mis. nama
// pelanggan di sel yang sama), dan akhiran seperti "17z" / "1136m".
func parseCoordPair(s string) (float64, float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	// Buang bungkus kurung/quotes umum, lalu hentikan di token pertama.
	s = strings.Trim(s, "()[]{}<>«»\"'`’‘“” ")
	// Path/trailing "/data=..." di URL redirect tidak pernah memuat koordinat.
	if i := strings.IndexAny(s, "/;"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ",")
	if len(parts) < 2 {
		return 0, 0, false
	}
	latStr := strings.TrimSpace(parts[0])
	lngStr := strings.TrimSpace(strings.TrimSpace(parts[1]))
	// Akhiran lng: "17z", "1136m", atau teks setelah spasi.
	if i := strings.IndexAny(lngStr, " \tzZ"); i >= 0 {
		lngStr = strings.TrimSpace(lngStr[:i])
	}
	lat, err1 := strconv.ParseFloat(latStr, 64)
	lng, err2 := strconv.ParseFloat(lngStr, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if !validateCoord(lat, lng) {
		return 0, 0, false
	}
	return lat, lng, true
}

// googleMapsHosts adalah host link Google Maps yang dikenal, dipakai untuk
// mengenali URL yang tertulis tanpa "http://" di tengah teks sel Excel.
var googleMapsHosts = []string{
	"maps.app.goo.gl",
	"maps.google.com",
	"maps.google.co",
	"google.com/maps",
	"goo.gl/maps",
}

// extractMapsURL mengambil token URL dari sebuah sel yang mungkin memuat teks
// tambahan (mis. nama pelanggan yang diketik setelah link). Bila tidak ada URL
// yang dikenali, string asli (mis. "lat,lng") dikembalikan apa adanya.
func extractMapsURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	start := -1
	if i := strings.Index(lower, "https://"); i >= 0 {
		start = i
	} else if i := strings.Index(lower, "http://"); i >= 0 {
		start = i
	}
	viaHost := false
	if start < 0 {
		for _, host := range googleMapsHosts {
			if h := strings.Index(lower, host); h >= 0 {
				start = h
				viaHost = true
				break
			}
		}
	}
	if start < 0 {
		return s
	}
	s = s[start:]
	if i := strings.IndexAny(s, " \t\r\n"); i >= 0 {
		s = s[:i]
	}
	if s = strings.Trim(s, "\"'`’‘“”()[]{}<>"); s == "" {
		return ""
	}
	if viaHost {
		s = "https://" + s
	}
	return s
}

// parseGoogleMapsCoord mengambil latitude/longitude dari berbagai format link
// Google Maps (maps.google.com/?q=..., google.com/maps/@..., ?ll=..., dst).
func parseGoogleMapsCoord(s string) (float64, float64, bool) {
	s = extractMapsURL(s)
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}

	// Format @lat,lng,zoom (path di google.com/maps/...@... )
	if i := strings.Index(s, "@"); i >= 0 {
		if lat, lng, ok := parseCoordPair(s[i+1:]); ok {
			return lat, lng, true
		}
	}

	// Format /maps/place/LAT,LNG/... yang sering dipakai URL hasil redirect
	// link pendek maps.app.goo.gl (mis. /maps/place/-7.807025,110.689765/data=...)
	if i := strings.Index(s, "/maps/place/"); i >= 0 {
		rest := s[i+len("/maps/place/"):]
		if j := strings.IndexAny(rest, "/?&"); j >= 0 {
			rest = rest[:j]
		}
		if lat, lng, ok := parseCoordPair(rest); ok {
			return lat, lng, true
		}
	}

	// Format query q=lat,lng atau q=loc:lat,lng
	if i := strings.Index(s, "q="); i >= 0 {
		rest := s[i+2:]
		if j := strings.IndexAny(rest, "&"); j >= 0 {
			rest = rest[:j]
		}
		if strings.HasPrefix(strings.ToLower(rest), "loc:") {
			rest = rest[4:]
		}
		if lat, lng, ok := parseCoordPair(rest); ok {
			return lat, lng, true
		}
	}

	// Format query data=...!3dLAT!4dLNG...
	if i3 := strings.Index(s, "!3d"); i3 >= 0 {
		i4 := strings.Index(s, "!4d")
		if i4 > i3 {
			latStr := s[i3+3 : i4]
			lngRest := s[i4+3:]
			if j := strings.IndexAny(lngRest, "!&"); j >= 0 {
				lngRest = lngRest[:j]
			}
			latS := strings.TrimSpace(latStr)
			lngS := strings.TrimSpace(lngRest)
			if lat, errLat := strconv.ParseFloat(latS, 64); errLat == nil {
				if lng, errLng := strconv.ParseFloat(lngS, 64); errLng == nil && validateCoord(lat, lng) {
					return lat, lng, true
				}
			}
		}
	}

	// Format query ll=lat,lng
	if i := strings.Index(s, "ll="); i >= 0 {
		rest := s[i+3:]
		if j := strings.IndexAny(rest, "&"); j >= 0 {
			rest = rest[:j]
		}
		if lat, lng, ok := parseCoordPair(rest); ok {
			return lat, lng, true
		}
	}

	// Baris langsung berisi "lat,lng" tanpa ada link lengkap.
	return parseCoordPair(s)
}

// resolveMapsLink mengikuti redirect link Google Maps pendek (contoh:
// https://maps.app.goo.gl/Da67oGYnbAhQpiMJA?g_st=ac) dan mengembalikan URL
// akhir yang memuat koordinat.
func resolveMapsLink(ctx context.Context, raw string) string {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return raw
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return raw
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return raw
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if u := resp.Request.URL; u != nil {
		return u.String()
	}
	return raw
}

// resolveMapsLinkCoord mencoba membaca koordinat langsung; bila gagal dan
// link merupakan URL, ikuti redirectnya lalu baca koordinat dari URL akhir.
// Bila URL akhir pun tidak memuat koordinat (link share titik/pin Google Maps
// yang berisi "Plus Code" + alamat), lakukan decode Plus Code dengan referensi
// lokasi dari Nominatim, lalu terakhir fallback geocoding alamatnya.
func resolveMapsLinkCoord(ctx context.Context, maps string) (float64, float64, bool) {
	maps = extractMapsURL(maps)
	maps = strings.TrimSpace(maps)
	if maps == "" {
		return 0, 0, false
	}
	if lat, lng, ok := parseGoogleMapsCoord(maps); ok {
		return lat, lng, true
	}
	lower := strings.ToLower(maps)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		final := resolveMapsLink(ctx, maps)
		if lat, lng, ok := parseGoogleMapsCoord(final); ok {
			return lat, lng, true
		}
		if text := mapsQueryText(final); text != "" {
			return coordFromQueryText(ctx, text)
		}
		return 0, 0, false
	}
	// Teks alamat/Plus Code langsung (tanpa URL).
	return coordFromQueryText(ctx, maps)
}

// mapsQueryText mengambil teks alamat dari URL Google Maps yang tidak memuat
// koordinat. Prioritas: parameter q, lalu nama tempat di path /place/... .
func mapsQueryText(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if q := strings.TrimSpace(u.Query().Get("q")); q != "" {
		return q
	}
	if i := strings.Index(u.Path, "/place/"); i >= 0 {
		seg := u.Path[i+len("/place/"):]
		if j := strings.Index(seg, "/"); j >= 0 {
			seg = seg[:j]
		}
		if strings.HasPrefix(seg, "@") {
			return ""
		}
		if decoded, err := url.PathUnescape(seg); err == nil {
			// Google Maps pakai "+" sebagai spasi pada nama tempat (path).
			return strings.TrimSpace(strings.ReplaceAll(decoded, "+", " "))
		}
		return strings.TrimSpace(seg)
	}
	return ""
}

// geocodeCache menghindari request berulang ke layanan eksternal untuk alamat
// yang sama saat import file besar.
var geocodeCache = struct {
	sync.Mutex
	m map[string][2]float64
}{m: map[string][2]float64{}}

// nominatimBase dapat diganti saat pengujian (httptest).
var nominatimBase = "https://nominatim.openstreetmap.org"

// geocodeMu memacu panggilan ke Nominatim agar tidak melampaui kebijakan
// layanan (maks. 1 request/detik).
var (
	geocodeMu       sync.Mutex
	lastGeocodeCall time.Time
)

// sleepCtx menunggu selama d; mengembalikan false bila context selesai dulu.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// geocodePlace mencari koordinat sebuah alamat/Plus Code via Nominatim
// (OpenStreetMap). Panggilan dipacu 1 request/detik dan di-retry singkat saat
// layanan membalas 429/5xx atau jaringan gagal.
func geocodePlace(ctx context.Context, query string) (float64, float64, bool) {
	key := strings.TrimSpace(strings.ToLower(query))
	geocodeCache.Lock()
	if v, ok := geocodeCache.m[key]; ok {
		geocodeCache.Unlock()
		return v[0], v[1], true
	}
	geocodeCache.Unlock()

	const minInterval = 1100 * time.Millisecond
	for attempt := 0; attempt < 3; attempt++ {
		geocodeMu.Lock()
		if wait := time.Until(lastGeocodeCall.Add(minInterval)); wait > 0 {
			geocodeMu.Unlock()
			if !sleepCtx(ctx, wait) {
				return 0, 0, false
			}
			geocodeMu.Lock()
		}
		lastGeocodeCall = time.Now()
		geocodeMu.Unlock()

		u := nominatimBase + "/search?format=jsonv2&limit=1&q=" + url.QueryEscape(key)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return 0, 0, false
		}
		req.Header.Set("User-Agent", "monitoring-dashboard/1.0 (import pelanggan)")
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			if attempt < 2 && sleepCtx(ctx, time.Duration(attempt+1)*2*time.Second) {
				continue
			}
			return 0, 0, false
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()

		// 429/5xx → layanan sibuk; retry sebentar lalu menyerah.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < 2 && sleepCtx(ctx, time.Duration(attempt+1)*2*time.Second) {
				continue
			}
			return 0, 0, false
		}
		if resp.StatusCode != http.StatusOK {
			return 0, 0, false
		}

		var results []struct {
			Lat string `json:"lat"`
			Lon string `json:"lon"`
		}
		if err := json.Unmarshal(body, &results); err != nil {
			return 0, 0, false
		}
		for _, r := range results {
			lat, errLat := strconv.ParseFloat(r.Lat, 64)
			lng, errLng := strconv.ParseFloat(r.Lon, 64)
			if errLat == nil && errLng == nil && validateCoord(lat, lng) {
				geocodeCache.Lock()
				geocodeCache.m[key] = [2]float64{lat, lng}
				geocodeCache.Unlock()
				return lat, lng, true
			}
		}
		return 0, 0, false
	}
	return 0, 0, false
}

// ---- Plus Code (Open Location Code) ----

// Alfabet Plus Code (Open Location Code) tanpa digit 0,1 dan huruf I,O,L,U,
// dipakai untuk mengenali pola kode di dalam teks alamat.
var plusCodeRe = regexp.MustCompile(`(?i)[23456789cfghjmpqrvwx]{4,8}[ +\t]+[23456789cfghjmpqrvwx]{2,4}`)

// extractPlusCode menormalisasi Plus Code yang mungkin tertulis "6PC2 3PH"
// (setelah URL-decode "+" berubah jadi spasi) menjadi "6PC2+3PH".
func extractPlusCode(s string) string {
	m := plusCodeRe.FindString(s)
	if m == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(m); i++ {
		if m[i] == ' ' || m[i] == '+' || m[i] == '\t' {
			b.WriteByte('+')
		} else {
			b.WriteByte(m[i])
		}
	}
	return b.String()
}

// candidateQueries membuat daftar query referensi yang makin pendek dari teks
// alamat (membuang segmen nama tempat di depan), karena Nominatim sering lebih
// berhasil pada tingkat kecamatan/kabupaten daripada alamat lengkap rumah.
func candidateQueries(text string) []string {
	text = strings.TrimSpace(text)
	var out []string
	if text != "" {
		out = append(out, text)
	}
	parts := strings.Split(text, ",")
	for i := 1; i < len(parts); i++ {
		q := strings.TrimSpace(strings.Join(parts[i:], ","))
		if q != "" && q != text {
			out = append(out, q)
		}
	}
	return out
}

// coordFromQueryText mengekstrak koordinat dari teks alamat:
//  1. bila mengandung Plus Code, decode Plus Code dengan referensi lokasi
//     dari Nominatim (akurat, tidak perlu API key);
//  2. bila tidak, geocoding alamat via Nominatim.
func coordFromQueryText(ctx context.Context, text string) (float64, float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, 0, false
	}

	if raw := plusCodeRe.FindString(text); raw != "" {
		code := extractPlusCode(raw)
		addr := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, raw, " "), code, " "))
		for _, ref := range candidateQueries(addr) {
			rlat, rlng, rok := geocodePlace(ctx, ref)
			if !rok {
				continue
			}
			full, err := olc.RecoverNearest(code, rlat, rlng)
			if err != nil {
				continue
			}
			area, err := olc.Decode(full)
			if err != nil {
				continue
			}
			lat, lng := area.Center()
			if validateCoord(lat, lng) {
				return lat, lng, true
			}
		}
		return 0, 0, false
	}

	for _, cand := range candidateQueries(text) {
		if lat, lng, ok := geocodePlace(ctx, cand); ok {
			return lat, lng, true
		}
	}
	return 0, 0, false
}

// ---- Import dari Excel (.xlsx) ----

// Kolom Excel dipetakan dengan header yang toleran (casing/space tidak
// diperhatikan). Normalisasi: huruf kecil & hapus spasi.
var customerExcelAliases = map[string]string{
	"id":               colCustomerCode,
	"nomerid":          colCustomerCode,
	"no":               colCustomerCode,
	"noid":             colCustomerCode,
	"id_pelanggan":     colCustomerCode,
	"kode":             colCustomerCode,
	"kodecustomer":     colCustomerCode,
	"customer_code":    colCustomerCode,
	"code":             colCustomerCode,
	"nama":             colName,
	"namapelanggan":    colName,
	"name":             colName,
	"customername":     colName,
	"customer_name":    colName,
	"ip":               colIP,
	"ipalamat":         colIP,
	"ipaddress":        colIP,
	"ip_pelanggan":     colIP,
	"ipalpelanggan":    colIP,
	"ipelanggan":       colIP,
	"vpn":              colVPN,
	"namavpn":          colVPN,
	"vpnname":          colVPN,
	"vpn_name":         colVPN,
	"pilihvpn":         colVPN,
	"pilih_vpn":        colVPN,
	"link":             colMaps,
	"linkgoogle":       colMaps,
	"linkgooglemaps":   colMaps,
	"linkmaps":         colMaps,
	"link_google":      colMaps,
	"link_googlemaps":  colMaps,
	"link_google_maps": colMaps,
	"googlemaps":       colMaps,
	"maps":             colMaps,
	"lokasi":           colMaps,
	"koordinat":        colMaps,
	"location":         colMaps,
	"latitude":         colLatitude,
	"longitude":        colLongitude,
}

const (
	colCustomerCode = "code"
	colName         = "name"
	colIP           = "ip"
	colVPN          = "vpn"
	colMaps         = "maps"
	colLatitude     = "latitude"
	colLongitude    = "longitude"
)

func normalizeAlias(alias string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(alias, " ", "_")))
}

// importCustomersSheet me-map header baris pertama untuk menemukan kolom.
func mapExcelHeaders(headers []string) map[string]int {
	colIdx := map[string]int{}
	for i, h := range headers {
		norm := normalizeAlias(h)
		if target, ok := customerExcelAliases[norm]; ok {
			colIdx[target] = i
		}
	}
	return colIdx
}

// resolveVPNID mencari id VPN berdasarkan nama.
func resolveVPNID(ctx context.Context, name string) (*int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	var id int64
	err := database.Pool.QueryRow(ctx,
		`SELECT id FROM vpn_connections WHERE LOWER(TRIM(name)) = $1`, strings.ToLower(name)).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// Impor pelanggan kini berjalan secara asinkron: handler ImportCustomers hanya
// menerima/memeriksa file lalu mengantrekannya ke proses latar belakang
// (lihat customer_import.go). Status impor dibaca via GET /customers/import/:id.

// ---- Export ke Excel (.xlsx) ----

// ---- Template Excel untuk impor ----

// TemplateCustomers menghasilkan file Excel panduan format impor: ID, Nama,
// IP Pelanggan, VPN, Link Google Maps. Sheet pertama ("Pelanggan") kosong dan
// siap diisi data; sheet kedua ("Contoh") berisi contoh baris beserta variasi
// format link Google Maps yang diterima. Impor membaca sheet pertama saja,
// sehingga sheet contoh aman bila tidak dihapus.
func TemplateCustomers(c *gin.Context) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Pelanggan"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"ID", "Nama", "IP Pelanggan", "VPN", "Link Google Maps"}
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#e2e8f0"}, Pattern: 1},
	})
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	f.SetCellStyle(sheet, "A1", "E1", style)

	// Sheet contoh memperlihatkan variasi format link yang bisa diimpor.
	contoh := "Contoh"
	f.NewSheet(contoh)
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(contoh, cell, h)
	}
	f.SetCellStyle(contoh, "A1", "E1", style)

	examples := [][]interface{}{
		{"CONTOH-1", "Contoh Pelanggan 1", "192.168.1.10", "- Tanpa VPN -", "https://maps.google.com/?q=-7.790466,110.721451"},
		{"CONTOH-2", "Contoh Pelanggan 2", "192.168.1.11", "- Tanpa VPN -", "https://www.google.com/maps/@-7.792141,110.720955,17z"},
		{"CONTOH-3", "Contoh Pelanggan 3", "192.168.1.12", "- Tanpa VPN -", "https://www.google.com/maps/place/-7.792030,110.726463/data=!4m6!3m5!1s0!7e2!8m2!3d-7.792030!4d110.726463!18m1!1e1"},
		{"CONTOH-4", "Contoh Pelanggan 4", "192.168.1.13", "- Tanpa VPN -", "-7.791032,110.722221"},
	}
	for i, row := range examples {
		for j, v := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			f.SetCellValue(contoh, cell, v)
		}
	}
	setRowHint := func(row int, hint string) {
		cell, _ := excelize.CoordinatesToCellName(6, row)
		f.SetCellValue(contoh, cell, hint)
	}
	f.SetColWidth(contoh, "F", "F", 60)
	setRowHint(2, "← maps.google.com/?q=LAT,LNG (disalin dari Google Maps).")
	setRowHint(3, "← google.com/maps/@LAT,LNG,zoom.")
	setRowHint(4, "← /place/LAT,LNG/data=...!3d...!4d... (hasil click share).")
	setRowHint(5, "← bisa langsung LAT,LNG tanpa link. Link pendek maps.app.goo.gl / goo.gl/maps juga didukung.")

	c.Header("Content-Disposition", `attachment; filename="template_import_pelanggan.xlsx"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat file template: " + err.Error()})
		return
	}
	c.Data(http.StatusOK,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

// ExportCustomers menghasilkan file Excel berisi seluruh pelanggan: nomor
// ID/identitas (mis. JMK.12), nama, IP, VPN, dan link Google Maps. ID ini yang
// dipakai saat impor ulang untuk memperbarui data yang sama.
func ExportCustomers(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), msQueryTimeout)
	defer cancel()

	rows, err := database.Pool.Query(ctx, `
		SELECT c.customer_code, c.customer_name, c.ip_address,
		       COALESCE(v.name, ''), c.latitude, c.longitude
		FROM customers c
		LEFT JOIN vpn_connections v ON v.id = c.vpn_id
		ORDER BY c.customer_code ASC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal memuat pelanggan: " + err.Error()})
		return
	}
	defer rows.Close()

	f := excelize.NewFile()
	defer f.Close()
	sheet := "Pelanggan"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"ID", "Nama", "IP Pelanggan", "VPN", "Link Google Maps"}
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#e2e8f0"}, Pattern: 1},
	})
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	f.SetCellStyle(sheet, "A1", "E1", style)

	rowIdx := 2
	for rows.Next() {
		var (
			code, name, ip, vpnName string
			lat, lng                float64
		)
		if err := rows.Scan(&code, &name, &ip, &vpnName, &lat, &lng); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca pelanggan: " + err.Error()})
			return
		}
		mapsLink := fmt.Sprintf("https://maps.google.com/?q=%f,%f", lat, lng)
		vals := []interface{}{code, name, ip, vpnName, mapsLink}
		for i, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(i+1, rowIdx)
			f.SetCellValue(sheet, cell, v)
		}
		rowIdx++
	}

	c.Header("Content-Disposition", `attachment; filename="pelanggan.xlsx"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat file excel: " + err.Error()})
		return
	}
	c.Data(http.StatusOK,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}
