package handlers

import (
	"bytes"
	"compress/gzip"
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
	"monitoring/internal/middleware"
)

var (
	cookieDomainRe = regexp.MustCompile(`(?i)(^|;\s*)Domain=[^;]+`)
	cookiePathRe   = regexp.MustCompile(`(?i)(^|;\s*)Path=[^;]+`)
	attrUrlRe      = regexp.MustCompile(`(?i)(\b(?:href|src|action|formaction)\s*=\s*["'])([^"']*)`)
	cssUrlRe       = regexp.MustCompile(`(?i)url\(\s*(['"]?)/([^'"]*)`)
	metaRefreshRe  = regexp.MustCompile(`(?i)(\bhttp-equiv\s*=\s*["']refresh["'][^>]*\bcontent\s*=\s*["'][^"']*\burl\s*=\s*)([^;"']+)`)
	baseTagRe      = regexp.MustCompile(`(?i)(<base\b[^>]*\bhref\s*=\s*["'])([^"']*)(["'])`)
	pathLitRe      = regexp.MustCompile(`(?i)(["'])(/[^"'\s]*\.(?:ghtml|shtm|shtml|cgi|asp|aspx|php|do|action|html|htm|css|js|png|jpe?g|gif|ico|svg|json|xml|txt|bin|dat))(["'])`)
)

// rewriteRootRelative menambahkan prefiks proksi pada URL absolut-path
// seperti /login, /js/app.js supaya tetap diproksikan lewat server.
// URL protokol-relatif (//host), absolut (http(s)://) dan yang sudah
// ber-prefiks proksi dibiarkan apa adanya.
func rewriteRootRelative(html, prefix string) string {
	return attrUrlRe.ReplaceAllStringFunc(html, func(m string) string {
		idx := attrUrlRe.FindStringSubmatchIndex(m)
		if idx == nil {
			return m
		}
		attr := m[idx[2]:idx[3]]
		val := m[idx[4]:idx[5]]
		if strings.HasPrefix(val, "/") && !strings.HasPrefix(val, "//") &&
			!strings.HasPrefix(strings.ToLower(val), "/api/modem/proxy") {
			return attr + prefix + strings.TrimPrefix(val, "/")
		}
		return m
	})
}

// rewriteAbsIP mengarahkan URL absolut ke IP modem (http(s)://<ip>[:port]/...)
// kembali lewat proksi. Referensi semacam ini sering dipakai firmware modem di
// CSS/JS/HTML untuk memuat asset atau menavigasi pasca-login; browser tidak
// bisa menjangkaunya langsung karena IP bersifat privat.
func rewriteAbsIP(s, ip, prefix string) string {
	re := regexp.MustCompile(`(?i)https?://` + regexp.QuoteMeta(ip) + `(?::[0-9]+)?/`)
	return re.ReplaceAllString(s, prefix)
}

func rewriteCSSURLs(s, prefix string) string {
	return cssUrlRe.ReplaceAllString(s, `url(${1}`+prefix+`$2`)
}

func rewriteMetaRefresh(html, prefix string) string {
	return metaRefreshRe.ReplaceAllStringFunc(html, func(m string) string {
		idx := metaRefreshRe.FindStringSubmatchIndex(m)
		if idx == nil {
			return m
		}
		head := m[idx[2]:idx[3]]
		urlval := m[idx[4]:idx[5]]
		if strings.HasPrefix(urlval, "/") && !strings.HasPrefix(urlval, "//") &&
			!strings.HasPrefix(strings.ToLower(urlval), "/api/modem/proxy") {
			return head + prefix + strings.TrimPrefix(urlval, "/")
		}
		return m
	})
}

// rewritePathLiterals mengubah literal path root-relatif ("/start.ghtml",
// "/login.cgi", dst) di carian dan teks skrip menjadi path proksi. Pergantian
// ini hanya mengganti isi string URL dengan isi string URL, jadi tidak merusak
// sintaksis skrip page-builder.
func rewritePathLiterals(s, prefix string) string {
	return pathLitRe.ReplaceAllStringFunc(s, func(m string) string {
		idx := pathLitRe.FindStringSubmatchIndex(m)
		if idx == nil {
			return m
		}
		open := m[idx[2]:idx[3]]
		path := m[idx[4]:idx[5]]
		close := m[idx[6]:idx[7]]
		if strings.HasPrefix(path, "//") ||
			strings.HasPrefix(strings.ToLower(path), "/api/modem/proxy") {
			return m
		}
		return open + prefix + strings.TrimPrefix(path, "/") + close
	})
}

// rewriteRedirect menyesuaikan header Location respons redirect (302/303/307)
// agar navigasi tetap berada di dalam path proksi. ReverseProxy bawaan Go tidak
// selalu menulis ulang Location absolut/root-relatif dari upstream, sehingga
// browser bisa "keluar" ke root aplikasi (/start.ghtml).
func rewriteRedirect(loc, prefix string) string {
	l := strings.TrimSpace(loc)
	if l == "" || strings.HasPrefix(l, "//") ||
		strings.HasPrefix(strings.ToLower(l), "/api/modem/proxy") {
		return l
	}
	if strings.HasPrefix(l, "/") {
		return prefix + strings.TrimPrefix(l, "/")
	}
	u, err := url.Parse(l)
	if err != nil || !u.IsAbs() {
		return l
	}
	out := prefix + strings.TrimPrefix(u.Path, "/")
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}

func rewriteHTML(s, ip, prefix string) string {
	// Teks <script>/<style>/komentar dibiarkan utuh; hanya atribut di dalam tag
	// yang di-rewrite supaya logika skrip page-builder tidak rusak.
	s = rewriteHtmlAttrs(s, prefix)
	s = rewritePathLiterals(s, prefix)
	s = rewriteAbsIP(s, ip, prefix)
	s = rewriteCSSURLs(s, prefix)
	s = rewriteMetaRefresh(s, prefix)

	// Shim navigasi dimuat sebagai berkas JS eksternal (bukan inline) dengan
	// data-rocket-ignore supaya rocket-loader/WP-Rocket di halaman modem tidak
	// mendefer/mengubahnya.
	nav := `<script src="` + prefix + `__nav__.js" data-rocket-ignore></script>`

	insertAt := headInsertPoint(s)
	if baseTagRe.MatchString(s) {
		// Sudah ada <base> sendiri di halaman modem: gunakan yang sama, tapi arahkan ke proksi
		// (aturan HTML: hanya <base> pertama yang dihormati).
		s = rewriteBase(s, prefix)
		if insertAt < 0 {
			return nav + s
		}
		return s[:insertAt] + "\n" + nav + s[insertAt:]
	}
	// Tidak ada <base>: sematkan <base> proksi + tag skrip shim sekaligus.
	nav = `<base href="` + prefix + `">\n` + nav
	if insertAt < 0 {
		return nav + s
	}
	return s[:insertAt] + "\n" + nav + s[insertAt:]
}

// rewriteHtmlAttrs mengubah atribut (href/src/action/formaction) hanya di dalam
// tag HTML. Daerah komentar, <script> dan <style> disalin verbatim agar isi
// kode milik halaman modem tidak berubah.
func rewriteHtmlAttrs(s, prefix string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	rest := s
	for {
		lt := strings.IndexByte(rest, '<')
		if lt < 0 {
			buf.WriteString(rest)
			return buf.String()
		}
		low := strings.ToLower(rest[lt:])
		if strings.HasPrefix(low, "<!--") {
			if end := strings.Index(rest[lt:], "-->"); end >= 0 {
				buf.WriteString(rest[:lt+end+3])
				rest = rest[lt+end+3:]
				continue
			}
			buf.WriteString(rest)
			return buf.String()
		}
		if strings.HasPrefix(low, "<script") {
			if end := strings.Index(rest[lt:], "</script>"); end >= 0 {
				buf.WriteString(rest[:lt+end+len("</script>")])
				rest = rest[lt+end+len("</script>"):]
				continue
			}
		}
		if strings.HasPrefix(low, "<style") {
			if end := strings.Index(rest[lt:], "</style>"); end >= 0 {
				buf.WriteString(rest[:lt+end+len("</style>")])
				rest = rest[lt+end+len("</style>"):]
				continue
			}
		}
		gt := strings.IndexByte(rest[lt:], '>')
		if gt < 0 {
			buf.WriteString(rest)
			return buf.String()
		}
		tag := rest[lt : lt+gt+1]
		buf.WriteString(rewriteRootRelative(tag, prefix))
		rest = rest[lt+gt+1:]
	}
}

// navShim adalah skrip yang menahan navigasi absolut-path milik halaman modem
// (termasuk top/parent) supaya tetap di dalam iframe proksi, bukan keluar modal.
func navShim(prefix string) string {
	return `(function(){
(function(){
	var p=(` + strconv.Quote(prefix) + `);
	function f(u){
		if(typeof u==='string' && u.charAt(0)==='/' && u.charAt(1)!=='/' && u.indexOf('/api/modem/proxy')!==0){
			return p+u.replace(/^\//,'');
		}
		return u;
	}
function g(d){
	try{
		var real=d.location;
		Object.defineProperty(d,'location',{
			get:function(){return real},
			set:function(v){
				var n=f(v);
				if(n!==v){real.replace(n)}else{real.href=v}
			},
			configurable:true
		});
		// top/parent.location.href = ... juga dicegat (langsung ubah properti Location).
		if(d!==window){
			try{
				var snap=real.href;
				Object.defineProperty(real,'href',{
					get:function(){return snap},
					set:function(v){real.replace(f(v))},
					configurable:true
				});
			}catch(e){}
		}
	}catch(e){}
}
	g(window);
	try{if(top&&top!==self){g(top)}}catch(e){}
	try{if(parent&&parent!==self&&parent!==top){g(parent)}}catch(e){}
})();
})();`
}

// rewriteBase mengubah nilai href elemen <base> pertama menjadi prefix proksi.
func rewriteBase(s, prefix string) string {
	idx := baseTagRe.FindStringSubmatchIndex(s)
	if idx == nil {
		return s
	}
	return s[:idx[2]] + s[idx[2]:idx[3]] + prefix + s[idx[6]:]
}

// headInsertPoint mengembalikan indeks tepat setelah tag <head ...>, atau -1.
func headInsertPoint(s string) int {
	idx := strings.Index(strings.ToLower(s), "<head")
	if idx < 0 {
		return -1
	}
	closeIdx := strings.Index(s[idx:], ">")
	if closeIdx < 0 {
		return -1
	}
	return idx + closeIdx + 1
}

// readBody membaca body response, dan bila Content-Encoding gzip maka di-gunzip
// terlebih dahulu agar peng-rewrite bisa memprosesnya.
func readBody(resp *http.Response) ([]byte, string) {
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	ce := resp.Header.Get("Content-Encoding")
	if err != nil {
		return body, ce
	}
	if !strings.EqualFold(ce, "gzip") {
		return body, ce
	}
	gr, gerr := gzip.NewReader(bytes.NewReader(body))
	if gerr != nil {
		return body, ce
	}
	un, uerr := io.ReadAll(gr)
	_ = gr.Close()
	if uerr != nil {
		return body, ce
	}
	return un, ce
}

func encodeBody(data []byte, ce string) []byte {
	if !strings.EqualFold(ce, "gzip") {
		return data
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write(data)
	_ = gw.Close()
	return buf.Bytes()
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

	// Skema/port diingat lewat cookie agar request lanjutan pasca-redirect
	// (tanpa query ?scheme=&port=) tetap menuju target yang sama.
	if _, has := c.GetQuery("scheme"); !has {
		if v, verr := c.Cookie("mdm_proxy_scheme_" + idStr); verr == nil && (v == "http" || v == "https") {
			scheme = v
		}
	}
	if _, has := c.GetQuery("port"); !has {
		if v, verr := c.Cookie("mdm_proxy_port_" + idStr); verr == nil {
			if p, perr := strconv.Atoi(v); perr == nil && p > 0 && p < 65536 {
				port = strconv.Itoa(p)
			}
		}
	}
	host = ipAddress
	if p, perr := strconv.Atoi(port); perr == nil && p > 0 && p < 65536 {
		host = net.JoinHostPort(ipAddress, strconv.Itoa(p))
	}
	target, err = url.Parse(scheme + "://" + host)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "target tidak valid"})
		return
	}

	// Berkas shim navigasi disajikan langsung oleh server (tidak diteruskan ke
	// modem) supaya bebas dari rewrite/modifikasi rocket-loader halaman modem.
	if strings.HasSuffix(strings.ToLower(c.Param("path")), "__nav__.js") {
		c.Header("Content-Type", "application/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		_, _ = c.Writer.WriteString(navShim(proxyPath))
		return
	}

	// Autentikasi: token JWT diterima via ?token=/Authorization header pada
	// kunjungan pertama, lalu disimpan sebagai cookie sesi agar asset halaman
	// modem (URL relatif tanpa header Authorization) juga terautentikasi.
	cookieName := "mdm_proxy_" + idStr
	token := c.Query("token")
	if token == "" {
		if ah := c.GetHeader("Authorization"); ah != "" {
			parts := strings.Split(ah, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}
	}
	if token == "" {
		if ck, cerr := c.Cookie(cookieName); cerr == nil && ck != "" {
			token = ck
		}
	}
	if _, ok := middleware.ValidateToken(token); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     proxyPath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "mdm_proxy_scheme_" + idStr,
		Value:    scheme,
		Path:     proxyPath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "mdm_proxy_port_" + idStr,
		Value:    port,
		Path:     proxyPath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

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
			q.Del("token")
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

			// Redirect (302 dst) diarahkan tetap lewat proksi, bukan ke root aplikasi.
			if loc := resp.Header.Get("Location"); loc != "" {
				resp.Header.Set("Location", rewriteRedirect(loc, proxyPath))
			}

			ct := strings.ToLower(resp.Header.Get("Content-Type"))
			isHTML := strings.Contains(ct, "text/html")
			isCSS := strings.Contains(ct, "text/css") || strings.Contains(ct, "application/x-css")
			isJS := strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript")
			if !isHTML && !isCSS && !isJS {
				return nil
			}

			data, ce := readBody(resp)
			if isHTML {
				data = []byte(rewriteHTML(string(data), ipAddress, proxyPath))
			} else if isCSS {
				s := rewriteCSSURLs(string(data), proxyPath)
				s = rewriteAbsIP(s, ipAddress, proxyPath)
				data = []byte(s)
			} else if isJS {
				data = []byte(rewriteAbsIP(string(data), ipAddress, proxyPath))
			}

			data = encodeBody(data, ce)
			resp.Body = io.NopCloser(bytes.NewReader(data))
			resp.ContentLength = int64(len(data))
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
