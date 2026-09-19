package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProxyIDFromReferer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		ref    string
		wantID int64
		wantOK bool
	}{
		{"template dengan query", "/api/modem/proxy/12/template.gch?pid=1002", 12, true},
		{"halaman root", "/api/modem/proxy/7/", 7, true},
		{"tanpa trailing slash", "https://monitoring.jernih.net.id/api/modem/proxy/42", 42, true},
		{"huruf besar", "/API/Modem/Proxy/99/x", 99, true},
		{"tanpa proxy", "/status_dev_info_t.gch", 0, false},
		{"referer kosong", "", 0, false},
		{"id tidak angka", "/api/modem/proxy/img/jiao_bg.gif", 0, false},
		{"nol tidak valid", "/api/modem/proxy/0/x", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.ref != "" {
				req.Header.Set("Referer", tt.ref)
			}
			c.Request = req

			id, ok := proxyIDFromReferer(c)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && id != tt.wantID {
				t.Fatalf("id = %d, want %d", id, tt.wantID)
			}
		})
	}
}

func TestModemProxyRedirectsMissingIDFromReferer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Simulasi asset modem yang jatuh tanpa id pelanggan:
	// URL diminta = /api/modem/proxy/img/jiao_bg.gif,
	// halaman induk (referer) = /api/modem/proxy/12/template.gch.
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/modem/proxy/img/jiao_bg.gif", nil)
	c.Request.Header.Set("Referer",
		"https://monitoring.jernih.net.id/api/modem/proxy/12/template.gch?pid=1002")
	c.Params = gin.Params{
		{Key: "id", Value: "img"},
		{Key: "path", Value: "/jiao_bg.gif"},
	}

	ModemProxy(c)

	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusTemporaryRedirect, w.Body.String())
	}
	loc := w.Header().Get("Location")
	want := "/api/modem/proxy/12/img/jiao_bg.gif"
	if loc != want {
		t.Fatalf("Location = %q, want %q", loc, want)
	}
}