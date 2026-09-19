package modem

import (
	"strings"
	"testing"
	"time"
)

func TestParseDBTables(t *testing.T) {
	out := `sendcmd 1 DB get DeviceInfo
<Row>
<Tbl name="DeviceInfo" RowCount="1">
<Row No="0">
<DM name="SerialNumber" val="ZTEG12345678"/>
<DM name="SoftwareVersion" val="V2.2.0P1T8"/>
<DM name="HardwareVersion" val="V1.0"/>
<DM name="MACAddress" val="AA:BB:CC:DD:EE:FF"/>
</Row>
</Tbl>
</Row>`
	tables := parseDBTables(out)
	if len(tables) != 1 {
		t.Fatalf("harap 1 tabel, dapat %d", len(tables))
	}
	tbl := tables[0]
	if tbl.Table != "DeviceInfo" {
		t.Errorf("nama tabel salah: %s", tbl.Table)
	}
	if len(tbl.Rows) != 1 {
		t.Fatalf("harap 1 baris, dapat %d", len(tbl.Rows))
	}
	if got := tbl.Rows[0]["SerialNumber"]; got != "ZTEG12345678" {
		t.Errorf("SerialNumber salah: %q", got)
	}
	if got := tbl.Rows[0]["SoftwareVersion"]; got != "V2.2.0P1T8" {
		t.Errorf("SoftwareVersion salah: %q", got)
	}
}

func TestParseDBTablesMulti(t *testing.T) {
	out := `<Tbl name="WLANCfg" RowCount="2">
<Row No="0"><DM name="SSID1" val="Home-2G"/></Row>
<Row No="1"><DM name="SSID1" val="Home-5G"/></Row>
</Tbl>
<Tbl name="WANPPPConnection" RowCount="1">
<Row No="0"><DM name="Username" val="user@isp"/></Row>
</Tbl>`
	tables := parseDBTables(out)
	if len(tables) != 2 {
		t.Fatalf("harap 2 tabel, dapat %d", len(tables))
	}
	if len(tables[0].Rows) != 2 {
		t.Fatalf("WLANCfg harap 2 baris, dapat %d", len(tables[0].Rows))
	}
	if tables[0].Rows[1]["SSID1"] != "Home-5G" {
		t.Errorf("SSID baris kedua salah: %q", tables[0].Rows[1]["SSID1"])
	}
	if tables[1].Rows[0]["Username"] != "user@isp" {
		t.Errorf("Username salah: %q", tables[1].Rows[0]["Username"])
	}
}

func TestParsePingSummary(t *testing.T) {
	out := `PING 8.8.8.8 (8.8.8.8): 56 data bytes
64 bytes from 8.8.8.8: icmp_seq=0 ttl=118 time=12.3 ms
64 bytes from 8.8.8.8: icmp_seq=1 ttl=118 time=13.1 ms

--- 8.8.8.8 ping statistics ---
2 packets transmitted, 2 packets received, 0% packet loss
round-trip min/avg/max = 12.3/12.7/13.1 ms`
	s := parsePingSummary(out)
	if s["loss_percent"] != float64(0) {
		t.Errorf("loss salah: %v", s["loss_percent"])
	}
	if s["rtt_avg_ms"] != 12.7 {
		t.Errorf("avg salah: %v", s["rtt_avg_ms"])
	}
	if s["rtt_min_ms"] != 12.3 || s["rtt_max_ms"] != 13.1 {
		t.Errorf("min/max salah: %v / %v", s["rtt_min_ms"], s["rtt_max_ms"])
	}
	if s["replies"] != 2 {
		t.Errorf("replies salah: %v", s["replies"])
	}
}

func TestParseTraceroute(t *testing.T) {
	out := `traceroute to 8.8.8.8 (8.8.8.8), 15 hops max
 1  192.168.1.1  1.234 ms  1.100 ms
 2  10.0.0.1  5.432 ms
 3  8.8.8.8  12.345 ms`
	hops := parseTraceroute(out)
	if len(hops) != 3 {
		t.Fatalf("harap 3 hop, dapat %d", len(hops))
	}
	if hops[0]["hop"] != 1 || hops[0]["host"] != "192.168.1.1" {
		t.Errorf("hop pertama salah: %v", hops[0])
	}
	if hops[2]["host"] != "8.8.8.8" {
		t.Errorf("hop terakhir salah: %v", hops[2])
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{90 * time.Minute, "1h 30m"},
		{26 * time.Hour, "1d 2h"},
	}
	for _, c := range cases {
		if got := HumanDuration(c.d); got != c.want {
			t.Errorf("HumanDuration(%v) = %q, mau %q", c.d, got, c.want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("simple"); got != "simple" {
		t.Errorf("simple salah: %q", got)
	}
	if got := shellQuote("a b"); got != `"a b"` {
		t.Errorf("spasi salah: %q", got)
	}
	if got := shellQuote(""); got != `""` {
		t.Errorf("kosong salah: %q", got)
	}
}

func TestAttrValue(t *testing.T) {
	tag := `<DM name="SSID1" val="My &amp; Home"/>`
	if got := attrValue(tag, "val"); got != "My & Home" {
		t.Errorf("attrValue salah: %q", got)
	}
	if got := attrValue(tag, "name"); got != "SSID1" {
		t.Errorf("attrValue name salah: %q", got)
	}
	if got := attrValue(tag, "missing"); got != "" {
		t.Errorf("attrValue hilang harus kosong: %q", got)
	}
}

func TestCatalogNonEmpty(t *testing.T) {
	cat := Catalog()
	if len(cat) < 40 {
		t.Fatalf("katalog terlalu sedikit: %d", len(cat))
	}
	sections := map[string]bool{}
	for _, f := range cat {
		sections[f.Section] = true
		if !strings.HasPrefix(f.Path, "/") {
			t.Errorf("path fitur tidak valid: %+v", f)
		}
	}
	for _, want := range []string{"status", "network", "security", "application", "manage", "diagnosis", "help"} {
		if !sections[want] {
			t.Errorf("section %s tidak ada di katalog", want)
		}
	}
}

func TestAccessBaseURL(t *testing.T) {
	a := Access{Host: "10.0.0.1"}
	if got := a.BaseURL(); got != "http://10.0.0.1:80" {
		t.Errorf("BaseURL http salah: %s", got)
	}
	b := Access{Host: "10.0.0.1", HTTPS: true, HTTPPort: 8443}
	if got := b.BaseURL(); got != "https://10.0.0.1:8443" {
		t.Errorf("BaseURL https salah: %s", got)
	}
}
