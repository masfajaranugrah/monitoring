package modem

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	pingLossRe = regexp.MustCompile(`(?i)([0-9.]+)%\s*(?:packet\s*)?loss`)
	pingRttRe  = regexp.MustCompile(`(?i)(?:rtt|round-trip).*?=\s*([0-9.]+)/([0-9.]+)/([0-9.]+)`)
	pingTimeRe = regexp.MustCompile(`(?i)time[=<]\s*([0-9.]+)\s*ms`)
)

// ---- Diagnosis: Ping ------------------------------------------------------

// DiagnosisPing menjalankan ping dari sisi modem ke host target.
func (d *Device) DiagnosisPing(ctx context.Context, host string, count, size int) (*Result, error) {
	if strings.TrimSpace(host) == "" {
		return nil, fmt.Errorf("host tujuan wajib diisi")
	}
	if count <= 0 {
		count = 4
	}
	if count > 20 {
		count = 20
	}
	data := map[string]interface{}{"host": host, "count": count, "size": size}

	// Jalur telnet (hasil paling akurat).
	if err := d.EnsureTelnet(ctx); err == nil {
		cmd := fmt.Sprintf("ping -c %d", count)
		if size > 0 {
			cmd += fmt.Sprintf(" -s %d", size)
		}
		cmd += " " + shellQuote(host)
		out, err := d.telnet.Exec(ctx, cmd)
		if err == nil && strings.TrimSpace(out) != "" {
			data["command"] = cmd
			data["output"] = out
			data["summary"] = parsePingSummary(out)
			return &Result{Section: "diagnosis", Feature: "ping", Title: "Ping Diagnosis", Source: "telnet", Data: data}, nil
		}
	}

	// Fallback: web diagnosis.
	if err := d.EnsureWeb(ctx); err == nil {
		form := d.withSession(ctx, url.Values{})
		form.Set("Host", host)
		form.Set("host", host)
		form.Set("Count", strconv.Itoa(count))
		form.Set("PingNum", strconv.Itoa(count))
		for _, ep := range []string{
			"/cgi-bin/web_cgi?page=ping",
			"/getpage.gch?pid=1002&nextpage=manager_diag_ping_t.gch",
		} {
			body, status, err := d.web.PostForm(ctx, ep, form)
			if err == nil && status == 200 && len(body) > 0 {
				data["endpoint"] = ep
				data["response"] = decodeJSONOrRaw(body)
				data["output"] = string(body)
				data["summary"] = parsePingSummary(string(body))
				return &Result{Section: "diagnosis", Feature: "ping", Title: "Ping Diagnosis", Source: "web", Data: data}, nil
			}
		}
	}
	return nil, describeErr(fmt.Errorf("%w: ping diagnosis tidak tersedia", ErrUnsupported), d, "ping diagnosis")
}

// ---- Diagnosis: Traceroute ------------------------------------------------

// DiagnosisTraceroute menjalankan traceroute dari sisi modem.
func (d *Device) DiagnosisTraceroute(ctx context.Context, host string, maxHops int) (*Result, error) {
	if strings.TrimSpace(host) == "" {
		return nil, fmt.Errorf("host tujuan wajib diisi")
	}
	if maxHops <= 0 {
		maxHops = 15
	}
	if maxHops > 30 {
		maxHops = 30
	}
	data := map[string]interface{}{"host": host, "max_hops": maxHops}

	if err := d.EnsureTelnet(ctx); err == nil {
		cmd := fmt.Sprintf("traceroute -m %d %s", maxHops, shellQuote(host))
		out, err := d.telnet.Exec(ctx, cmd)
		if err != nil || strings.TrimSpace(out) == "" {
			// BusyBox kadang hanya punya `traceroute` tanpa flag -m.
			cmd = "traceroute " + shellQuote(host)
			out, err = d.telnet.Exec(ctx, cmd)
		}
		if err == nil && strings.TrimSpace(out) != "" {
			data["command"] = cmd
			data["output"] = out
			data["hops"] = parseTraceroute(out)
			return &Result{Section: "diagnosis", Feature: "traceroute", Title: "Trace Route", Source: "telnet", Data: data}, nil
		}
	}

	if err := d.EnsureWeb(ctx); err == nil {
		form := d.withSession(ctx, url.Values{})
		form.Set("Host", host)
		form.Set("host", host)
		form.Set("MaxHop", strconv.Itoa(maxHops))
		for _, ep := range []string{
			"/cgi-bin/web_cgi?page=traceroute",
			"/getpage.gch?pid=1002&nextpage=manager_diag_tracert_t.gch",
		} {
			body, status, err := d.web.PostForm(ctx, ep, form)
			if err == nil && status == 200 && len(body) > 0 {
				data["endpoint"] = ep
				data["response"] = decodeJSONOrRaw(body)
				data["output"] = string(body)
				return &Result{Section: "diagnosis", Feature: "traceroute", Title: "Trace Route", Source: "web", Data: data}, nil
			}
		}
	}
	return nil, describeErr(fmt.Errorf("%w: traceroute tidak tersedia", ErrUnsupported), d, "traceroute")
}

// ---- Diagnosis: ARP Table -------------------------------------------------

// DiagnosisARPTable mengembalikan tabel ARP perangkat.
func (d *Device) DiagnosisARPTable(ctx context.Context) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	data := map[string]interface{}{}
	if out, err := d.telnet.Exec(ctx, "arp -a"); err == nil && strings.TrimSpace(out) != "" {
		data["arp_a"] = strings.TrimSpace(out)
	} else if out, err := d.telnet.Exec(ctx, "cat /proc/net/arp"); err == nil && strings.TrimSpace(out) != "" {
		data["proc_net_arp"] = strings.TrimSpace(out)
	}
	if out, err := d.telnet.Exec(ctx, "ip neigh"); err == nil && strings.TrimSpace(out) != "" {
		data["ip_neigh"] = strings.TrimSpace(out)
	}
	if len(data) == 0 {
		return nil, describeErr(fmt.Errorf("%w: ARP table tidak tersedia", ErrUnsupported), d, "ARP table")
	}
	return &Result{Section: "diagnosis", Feature: "arp_table", Title: "ARP Table", Source: "telnet", Data: data}, nil
}

// ---- Diagnosis: MAC Table -------------------------------------------------

// DiagnosisMACTable mengembalikan tabel MAC/FDB bridge perangkat.
func (d *Device) DiagnosisMACTable(ctx context.Context) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	data := map[string]interface{}{}
	for cmd, key := range map[string]string{
		"brctl showmacs br0": "bridge_mac",
		"cat /proc/net/br0/br_forward": "fdb",
		"cat /proc/net/dev":  "net_dev",
	} {
		if out, err := d.telnet.Exec(ctx, cmd); err == nil && strings.TrimSpace(out) != "" {
			data[key] = strings.TrimSpace(out)
		}
	}
	if len(data) == 0 {
		return nil, describeErr(fmt.Errorf("%w: MAC table tidak tersedia", ErrUnsupported), d, "MAC table")
	}
	return &Result{Section: "diagnosis", Feature: "mac_table", Title: "MAC Table", Source: "telnet", Data: data}, nil
}

// ---- Diagnosis: Optical / PON ---------------------------------------------

// DiagnosisOptical mengembalikan daya optik dan status registrasi PON.
func (d *Device) DiagnosisOptical(ctx context.Context) (*Result, error) {
	res := &Result{Section: "diagnosis", Feature: "optical", Title: "Optical / PON Status"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"optical":    {"GponOptical", "OPTICAL", "OpticalInfo", "GponOpticalInfo", "PonOptical"},
		"pon_status": {"PONInfo", "GponInfo", "GPONCfg", "GponStatus", "PonState"},
		"ploam":      {"PLOAM", "PLOAMCfg", "GponPLOAM"},
	})

	// Beberapa firmware menyediakan perintah khusus untuk daya optik.
	for cmd, key := range map[string]string{
		"gponcmd optic info": "gponcmd_optic_info",
		"omcicmd show pm":    "omcicmd_pm",
	} {
		if out, err := d.telnet.Exec(ctx, cmd); err == nil && strings.TrimSpace(out) != "" {
			data[key] = strings.TrimSpace(out)
			ok = true
		}
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: status optik tidak tersedia", ErrUnsupported), d, "optical")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ---- Diagnosis: Loopback --------------------------------------------------

// DiagnosisLoopback mengembalikan konfigurasi loopback detection.
func (d *Device) DiagnosisLoopback(ctx context.Context) (*Result, error) {
	res := &Result{Section: "diagnosis", Feature: "loopback", Title: "Loopback Detection"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"loopback": {"LoopbackDetect", "LoopbackDetection", "LoopbackDetectCfg"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: loopback detection tidak tersedia", ErrUnsupported), d, "loopback")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ---- helpers --------------------------------------------------------------

func parsePingSummary(out string) map[string]interface{} {
	summary := map[string]interface{}{}
	if m := pingLossRe.FindStringSubmatch(out); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			summary["loss_percent"] = v
		}
	}
	if m := pingRttRe.FindStringSubmatch(out); len(m) == 4 {
		if avg, err := strconv.ParseFloat(m[2], 64); err == nil {
			summary["rtt_avg_ms"] = avg
		}
		if min, err := strconv.ParseFloat(m[1], 64); err == nil {
			summary["rtt_min_ms"] = min
		}
		if max, err := strconv.ParseFloat(m[3], 64); err == nil {
			summary["rtt_max_ms"] = max
		}
	}
	times := pingTimeRe.FindAllStringSubmatch(out, -1)
	if len(times) > 0 {
		summary["replies"] = len(times)
	}
	return summary
}

// parseTraceroute memparse baris hop traceroute menjadi struct ringkas.
func parseTraceroute(out string) []map[string]interface{} {
	hops := []map[string]interface{}{}
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) == 0 {
			continue
		}
		hopNum, err := strconv.Atoi(strings.TrimSuffix(fields[0], "."))
		if err != nil {
			continue
		}
		hop := map[string]interface{}{"hop": hopNum, "raw": ln}
		// Ambil IP/host hop (field setelah nomor hop).
		for _, f := range fields[1:] {
			if strings.Contains(f, ".") || strings.Contains(f, ":") {
				hop["host"] = strings.TrimSpace(f)
				break
			}
		}
		hops = append(hops, hop)
	}
	return hops
}
