package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"monitoring/internal/database"
)

var (
	cookieDomainRe = regexp.MustCompile(`(?i)(^|;\s*)Domain=[^;]+`)
	cookiePathRe   = regexp.MustCompile(`(?i)(^|;\s*)Path=[^;]+`)
	rootRelRe      = regexp.MustCompile(`(?i)(\b(?:href|src|action|formaction)\s*=\s*["'])/(?!/)([^"']*)`)
)

// rewriteRootRelative menambahkan prefiks proksi pada URL absolut-path
// seperti /login, /js/app.js supaya tetap diproksikan lewat server.
func rewriteRootRelative(html, prefix string) string {
	return rootRelRe.ReplaceAllString(html, `${1}`+prefix+`$2`)
}

// ModemProxy membuka halaman login modem secara transparan lewat server.
// Browser tidak perlu bisa menjangkau IP pelanggan (LAN/VPN privat) dan tidak
// ada masalah mixed-content/X-Frame-Options karena respon disajikan same-origin.
//
// URL: /api/modem/proxy/<id>/<path>?scheme=http|https&port=<n>
func ModemProxy(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var ipAddress string
	if err := database.Pool.QueryRow(ctx, `SELECT ip_address FROM customers WHERE id = $1`, id).Scan(&ipAddress); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "pelanggan tidak ditemukan"})
		return
	}
	ipAddress = strings.TrimSpace(ipAddress)
	if ipAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pelanggan tidak punya alamat IP"})
		return
	}

	scheme := c.DefaultQuery("scheme", "http")
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	port := strings.TrimSpace(c.Query("port"))
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	host := ipAddress
	if p, perr := strconv.Atoi(port); perr == nil && p > 0 && p < 65536 {
		host = net.JoinHostPort(ipAddress, strconv.Itoa(p))
	}

	target, err := url.Parse(scheme + "://" + host)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "target tidak valid"})
		return
	}

	idStr := strconv.FormatInt(id, 10)
	proxyPath := "/api/modem/proxy/" + idStr + "/"

	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			sub := strings.TrimPrefix(pr.In.URL.Path, "/api/modem/proxy/"+idStr)
			if sub == "" {
				sub = "/"
			}
			if !strings.HasPrefix(sub, "/") {
				sub = "/" + sub
			}
			pr.Out.URL.Path = sub
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			q := pr.Out.URL.Query()
			q.Del("scheme")
			q.Del("port")
			pr.Out.URL.RawQuery = q.Encode()
		},
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		},
		ModifyResponse: func(resp *http.Response) error {
			// Cookie sesi modem dibatasi ke path proksi pelanggan ini supaya
			// sesi antar-pelanggan tidak saling menimpa.
			if scs := resp.Header.Values("Set-Cookie"); len(scs) > 0 {
				out := make([]string, 0, len(scs))
				for _, sc := range scs {
					out = append(out, rewriteSetCookie(sc, proxyPath))
				}
				resp.Header.Del("Set-Cookie")
				for _, sc := range out {
					resp.Header.Add("Set-Cookie", sc)
				}
			}
			// Hapus header web-safety milik upstream yang tidak berlaku untuk asal kita.
			resp.Header.Del("Content-Security-Policy")
			resp.Header.Del("Content-Security-Policy-Report-Only")
			resp.Header.Del("X-Frame-Options")

			ct := resp.Header.Get("Content-Type")
			if !strings.Contains(strings.ToLower(ct), "text/html") || strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
				return nil
			}
			body, rerr := io.ReadAll(resp.Body)
			if rerr != nil {
				resp.Body.Close()
				return nil
			}
			html := string(body)
			baseTag := `<base href="` + proxyPath + `">`
			// URL absolut-path (/... dari si modem) diarahkan kembali lewat proksi.
			html = rewriteRootRelative(html, proxyPath)
			idx := strings.Index(html, "<head")
			if idx < 0 {
				html = baseTag + html
			} else {
				closeIdx := strings.Index(html[idx:], ">")
				if closeIdx < 0 {
					html = baseTag + html
				} else {
					insertAt := idx + closeIdx + 1
					html = html[:insertAt] + "\n" + baseTag + html[insertAt:]
				}
			}
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewBufferString(html))
			resp.ContentLength = int64(len(html))
			resp.Header.Del("Content-Length")
			return nil
		},
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, perr error) {
		log.Printf("[modem] proxy %s gagal: %v", ipAddress, perr)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"error":"gagal menghubungi modem pelanggan. Pastikan server bisa menjangkau IP %s (periksa VPN/routing) atau cek port/scheme."}`, ipAddress)
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

func rewriteSetCookie(sc, basePath string) string {
	sc = cookieDomainRe.ReplaceAllString(sc, "")
	if cookiePathRe.MatchString(sc) {
		sc = cookiePathRe.ReplaceAllString(sc, `${1}Path=`+basePath)
	} else {
		sc += "; Path=" + basePath
	}
	return sc
}