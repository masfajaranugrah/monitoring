package handlers

import (
	"strings"
	"testing"
)

// Simulasi template.gch khas ZTE: CSS inline, style attr, atribut HTML, dan JS
// page-builder yang menulis url relatif/root-relatif.
const zteTemplate = `<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=gb2312">
<title>ZTE</title>
<style type="text/css">
body { background: #fff url('img/jiao_bg.gif') no-repeat; }
#wrap { background: url("/img/up_bg.gif") repeat-x; }
td.tl { background: url(img/content_bg.gif); }
span { background: url( /img/left_bg.gif ) no-repeat; }
.icon { background-image:url("img/px35.gif"); }
.match { width: 1px; background: url("/img/button_bg_2.gif"); }
</style>
</head>
<body>
<img src="img/jiao_bg.gif" alt="">
<img src="/img/up_bg.gif">
<table><td style="background-image:url('/img/content_bg.gif')">x</td></table>
<script type="text/javascript">
var dir = "/status_dev_info_t.gch";
function openLink(u){ location.href = u; }
document.write('<img src="img/px35.gif">');
var bg = 'url(/img/left_bg.gif)';
</script>
<a href="/getpage.gch?pid=1002&nextpage=app_ddns_conf_t.gch">DDNS</a>
</body>
</html>`

func TestRewriteHTMLProducesNoMissingIDProxyRef(t *testing.T) {
	prefix := "/api/modem/proxy/12/"
	ip := "192.168.1.1"
	out := rewriteHTML(zteTemplate, ip, prefix)

	// Setiap referensi proksi harus membawa id pelanggan setelah /api/modem/proxy/.
	lower := strings.ToLower(out)
	for _, bad := range scanBadProxy(out) {
		t.Fatalf("rewriteHTML menghasilkan referensi proksi tanpa id: %s", bad)
	}
	_ = lower

	// <base> proksi harus ada supaya url relatif ikut diproksikan.
	if !strings.Contains(out, `<base href="`+prefix+`">`) {
		t.Fatalf("base proksi tidak tersisip:\n%s", out)
	}

	for _, want := range []string{
		prefix + "img/up_bg.gif",
		prefix + "img/content_bg.gif",
		prefix + "getpage.gch?pid=1002&nextpage=app_ddns_conf_t.gch",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("tidak menemukan %q dalam output rewrite:\n%s", want, out)
		}
	}

	// Tidak boleh ada prefix ganda (double prefix) hasil benturan dua rewrite.
	double := prefix + "api/modem/proxy/"
	if strings.Contains(out, double) {
		t.Fatalf("ditemukan double prefix %q dalam output rewrite:\n%s", double, out)
	}
}
