package modem

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Timeout bawaan untuk operasi tunggal ke perangkat.
const (
	defaultHTTPTimeout   = 12 * time.Second
	defaultTelnetTimeout = 8 * time.Second
	maxBodyBytes         = 4 << 20 // 4 MiB
)

var (
	loginTokenJSRe = regexp.MustCompile(`(?i)(?:Frm_Logintoken|logintoken)[^\d]{0,40}?(\d{2,})`)
	loginTokenInRe = regexp.MustCompile(`(?i)name=["']?(?:Frm_Logintoken|logintoken)["']?[^>]*value=["']?(\d+)`)
	sessionJSRe    = regexp.MustCompile(`(?i)var\s+session_token\s*=\s*["']([0-9A-Za-z_\-]+)["']`)
	sessionHiddenRe = regexp.MustCompile(`(?i)name=["']?_SESSION_TOKEN["']?[^>]*value=["']?([0-9A-Za-z_\-]+)`)
)

// WebClient mengelola satu sesi HTTP ke web management perangkat.
type WebClient struct {
	access  Access
	http    *http.Client
	base    string
	mu      sync.Mutex
	loggedIn bool
	// style menyimpan varian login yang berhasil ("gch" atau "webcgi").
	style string
}

// NewWebClient membuat sesi web baru (belum login).
func NewWebClient(a Access) *WebClient {
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   defaultHTTPTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   defaultHTTPTimeout,
		ResponseHeaderTimeout: 20 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}
	return &WebClient{
		access: a,
		base:   strings.TrimRight(a.BaseURL(), "/"),
		http: &http.Client{
			Transport: tr,
			Jar:       jar,
			Timeout:   defaultHTTPTimeout,
		},
	}
}

// BaseURL mengembalikan URL dasar perangkat.
func (w *WebClient) BaseURL() string { return w.base }

// Client mengekspos http.Client (untuk testing/override).
func (w *WebClient) Client() *http.Client { return w.http }

// LoggedIn melaporkan status login.
func (w *WebClient) LoggedIn() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.loggedIn
}

// Login menjalankan alur autentikasi web. Beberapa varian firmware ZTE
// memakai endpoint berbeda; fungsi ini mencoba keduanya sampai berhasil.
func (w *WebClient) Login(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.loggedIn {
		return nil
	}

	token, page, err := w.fetchLoginToken(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWebDisabled, err)
	}
	_ = page

	type attempt struct {
		name string
		fn   func(ctx context.Context, token string) error
	}
	attempts := []attempt{
		{"webcgi", w.loginWebCGI},
		{"gch", w.loginGCH},
	}
	var lastErr error
	for _, a := range attempts {
		if err := a.fn(ctx, token); err != nil {
			lastErr = err
			continue
		}
		if w.probeLoggedIn(ctx) {
			w.style = a.name
			w.loggedIn = true
			return nil
		}
		lastErr = fmt.Errorf("%w: probe sesi gagal setelah login %s", ErrLoginFailed, a.name)
		// Ambil token baru sebelum mencoba style berikutnya.
		if t2, _, e2 := w.fetchLoginToken(ctx); e2 == nil {
			token = t2
		}
	}
	if lastErr == nil {
		lastErr = ErrLoginFailed
	}
	return lastErr
}

// loginWebCGI adalah varian firmware V9: POST /cgi-bin/login?action=login.
func (w *WebClient) loginWebCGI(ctx context.Context, token string) error {
	form := url.Values{}
	form.Set("Frm_Logintoken", token)
	form.Set("UserName", w.access.WebUser)
	form.Set("Passwd", base64.StdEncoding.EncodeToString([]byte(w.access.WebPass)))
	form.Set("Language", "en")
	form.Set("url", "%2F")
	endpoints := []string{
		"/cgi-bin/login?action=login",
		"/cgi-bin/login.cgi?action=login",
	}
	var lastErr error
	for _, ep := range endpoints {
		if _, _, err := w.postForm(ctx, ep, form, ""); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

// loginGCH adalah varian klasik: POST / dengan field gch.
func (w *WebClient) loginGCH(ctx context.Context, token string) error {
	form := url.Values{}
	form.Set("frashnum", "")
	form.Set("action", "login")
	form.Set("Frm_Logintoken", token)
	form.Set("Username", w.access.WebUser)
	form.Set("Password", w.access.WebPass)
	form.Set("UserRandomNum", "")
	if _, _, err := w.postForm(ctx, "/", form, w.base+"/"); err != nil {
		return err
	}
	return nil
}

// probeLoggedIn memeriksa apakah sesi sudah terautentikasi, misalnya dengan
// memuat halaman utama dan mendeteksi tidak adanya form login / adanya token sesi.
func (w *WebClient) probeLoggedIn(ctx context.Context) bool {
	for _, p := range []string{"/start.ghtml", "/template.gch", "/getpage.gch?pid=1002&nextpage=status_dev_info_t.gch", "/cgi-bin/web_cgi?page=status"} {
		body, status, err := w.getRaw(ctx, p)
		if err != nil {
			continue
		}
		if status == http.StatusUnauthorized {
			continue
		}
		low := strings.ToLower(string(body))
		if strings.Contains(low, "frm_logintoken") || strings.Contains(low, "name=\"passwd\"") ||
			strings.Contains(low, "name=\"password\"") {
			continue
		}
		if status == http.StatusOK && len(body) > 0 {
			return true
		}
	}
	return false
}

// fetchLoginToken memuat halaman login dan mengambil nilai Frm_Logintoken.
func (w *WebClient) fetchLoginToken(ctx context.Context) (string, string, error) {
	paths := []string{"/", "/login.gch", "/cgi-bin/index"}
	var lastErr error
	for _, p := range paths {
		body, _, err := w.getRaw(ctx, p)
		if err != nil {
			lastErr = err
			continue
		}
		text := string(body)
		if m := loginTokenJSRe.FindStringSubmatch(text); len(m) == 2 {
			return m[1], text, nil
		}
		if m := loginTokenInRe.FindStringSubmatch(text); len(m) == 2 {
			return m[1], text, nil
		}
		lastErr = fmt.Errorf("%w: token login tidak ditemukan di %s", ErrLoginFailed, p)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: halaman login tidak dapat dibaca", ErrLoginFailed)
	}
	return "", "", lastErr
}

// SessionToken mengambil _SESSION_TOKEN yang diperlukan sebagian aksi tulis.
func (w *WebClient) SessionToken(ctx context.Context) string {
	for _, p := range []string{"/template.gch", "/start.ghtml", "/getpage.gch?pid=1002&nextpage=status_dev_info_t.gch"} {
		body, _, err := w.getRaw(ctx, p)
		if err != nil {
			continue
		}
		text := string(body)
		if m := sessionJSRe.FindStringSubmatch(text); len(m) == 2 {
			return m[1]
		}
		if m := sessionHiddenRe.FindStringSubmatch(text); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

// GetRaw melakukan GET tanpa parsing.
func (w *WebClient) GetRaw(ctx context.Context, path string) ([]byte, int, error) {
	return w.getRaw(ctx, path)
}

func (w *WebClient) getRaw(ctx context.Context, path string) ([]byte, int, error) {
	req, err := w.newRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, 0, err
	}
	return w.do(req)
}

// WebCGI memanggil halaman web_cgi dan mengembalikan body mentah.
func (w *WebClient) WebCGI(ctx context.Context, page string, params url.Values) ([]byte, int, error) {
	u := "/cgi-bin/web_cgi?page=" + url.QueryEscape(page)
	if len(params) > 0 {
		u += "&" + params.Encode()
	}
	return w.getRaw(ctx, u)
}

// GCH memanggil mekanisme getpage.gch.
func (w *WebClient) GCH(ctx context.Context, pid, nextpage string, params url.Values) ([]byte, int, error) {
	u := "/getpage.gch?pid=" + url.QueryEscape(pid) + "&nextpage=" + url.QueryEscape(nextpage)
	if len(params) > 0 {
		u += "&" + params.Encode()
	}
	return w.getRaw(ctx, u)
}

// PostForm mengirim form ke path tertentu.
func (w *WebClient) PostForm(ctx context.Context, path string, form url.Values) ([]byte, int, error) {
	return w.postForm(ctx, path, form, w.base+"/")
}

func (w *WebClient) postForm(ctx context.Context, path string, form url.Values, referer string) ([]byte, int, error) {
	body := form.Encode()
	req, err := w.newRequest(ctx, http.MethodPost, path, strings.NewReader(body), referer)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return w.do(req)
}

func (w *WebClient) newRequest(ctx context.Context, method, path string, body io.Reader, referer string) (*http.Request, error) {
	var full string
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		full = path
	} else {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		full = w.base + path
	}
	req, err := http.NewRequestWithContext(ctx, method, full, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) MonitoringModem/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", "_TESTCOOKIESUPPORT=1")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return req, nil
}

func (w *WebClient) do(req *http.Request) ([]byte, int, error) {
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// DecodeBody mencoba mendekode body sebagai JSON; bila gagal, mengembalikan
// map dengan field "raw" berisi teks mentah.
func decodeJSONOrRaw(body []byte) interface{} {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var v interface{}
		if err := json.Unmarshal(trimmed, &v); err == nil {
			return v
		}
	}
	return map[string]string{"raw": string(body)}
}
