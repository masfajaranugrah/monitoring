package modem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func tableResult(section, feature, title, source string, tbl *TableResult) *Result {
	return &Result{
		Section: section,
		Feature: feature,
		Title:   title,
		Source:  source,
		Data:    tbl,
	}
}

func mapResult(section, feature, title, source string, data interface{}) *Result {
	return &Result{Section: section, Feature: feature, Title: title, Source: source, Data: data}
}

// ---- Status: Device Information -------------------------------------------

// StatusDevice mengembalikan Device Information (serial, versi, uptime, dsb).
func (d *Device) StatusDevice(ctx context.Context) (*Result, error) {
	res := &Result{Section: "status", Feature: "device", Title: "Device Information"}
	data := map[string]interface{}{}

	// Jalur telnet (paling lengkap).
	if tbl, name, err := d.FirstTable(ctx,
		"DeviceInfo", "Device", "DeviceCfg", "SysInfo", "DeviceStatus"); err == nil && len(tbl.Rows) > 0 {
		data["device_info"] = tbl.Rows
		data["device_table"] = name
		res.Source = "telnet"
	}

	if tbl, name, err := d.FirstTable(ctx,
		"DeviceUpTime", "UpTime", "DeviceUpTimeInfo", "SysUpTime"); err == nil && len(tbl.Rows) > 0 {
		data["uptime"] = tbl.Rows
		data["uptime_table"] = name
		if res.Source == "" {
			res.Source = "telnet"
		}
	}

	// Jalur web sebagai pelengkap/pengganti.
	if res.Source == "" {
		if err := d.EnsureWeb(ctx); err == nil {
			if body, _, err := d.web.WebCGI(ctx, "devstatus", nil); err == nil {
				data["web"] = decodeJSONOrRaw(body)
				res.Source = "web"
			} else if body, _, err := d.web.GCH(ctx, "1002", "status_dev_info_t.gch", nil); err == nil {
				data["web"] = map[string]string{"raw": string(body)}
				res.Source = "web"
			}
		}
	}

	if res.Source == "" {
		return nil, describeErr(fmt.Errorf("%w: device information tidak tersedia", ErrUnsupported),
			d, "device information")
	}

	// Lengkapi uptime mentah menjadi teks yang mudah dibaca.
	if up, ok := data["uptime"].([]Row); ok && len(up) > 0 {
		if secs, ok := firstNumeric(up[0], "UpTime", "UpTimeSince", "Secs", "upTime"); ok {
			data["uptime_text"] = HumanDuration(time.Duration(secs) * time.Second)
		}
	}

	res.Data = data
	return res, nil
}

// ---- Status: Network Information ------------------------------------------

// StatusNetworkInfo mengembalikan informasi koneksi jaringan (WAN + PON + LAN).
func (d *Device) StatusNetworkInfo(ctx context.Context) (*Result, error) {
	res := &Result{Section: "status", Feature: "network_info", Title: "Network Information"}
	data := map[string]interface{}{}

	var errs []string
	for key, candidates := range map[string][]string{
		"wan":     {"WANPPPConnection", "WANIPConnection", "WANC", "WANConnection", "WANPPP"},
		"pon":     {"PONInfo", "GponInfo", "GPONCfg", "GPONStat", "PonInfo", "GponStatus"},
		"lan":     {"LANCfg", "LANHostCfg", "DHCPv4IFCfg", "LanCfg", "LAN"},
		"optical": {"GponOptical", "OPTICAL", "OpticalInfo", "GponOpticalInfo"},
		"eth":     {"EthStat", "LANEthIf", "EthInfo"},
	} {
		if tbl, name, err := d.FirstTable(ctx, candidates...); err == nil && len(tbl.Rows) > 0 {
			data[key] = tbl.Rows
			data[key+"_table"] = name
			res.Source = "telnet"
		} else if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", key, err))
		}
	}

	if res.Source == "" {
		// Web fallback: page "networkinfo"/"wannet"/"netinfo".
		if err := d.EnsureWeb(ctx); err == nil {
			for _, page := range []string{"netinfo", "networkinfo", "wannet", "net_status"} {
				if body, _, err := d.web.WebCGI(ctx, page, nil); err == nil && len(body) > 0 {
					data["web"] = decodeJSONOrRaw(body)
					data["web_page"] = page
					res.Source = "web"
					break
				}
			}
		}
	}

	if res.Source == "" {
		return nil, describeErr(fmt.Errorf("%w: network information tidak tersedia", ErrUnsupported),
			d, "network information")
	}
	if len(errs) > 0 {
		res.Warning = strings.Join(errs, "; ")
	}
	res.Data = data
	return res, nil
}

// ---- Status: User Information ---------------------------------------------

// StatusUserInfo mengembalikan informasi perangkat pengguna yang terhubung
// (WLAN station, ethernet, DHCP lease).
func (d *Device) StatusUserInfo(ctx context.Context) (*Result, error) {
	res := &Result{Section: "status", Feature: "user_info", Title: "User Information"}
	data := map[string]interface{}{}

	for key, candidates := range map[string][]string{
		"wlan_associated": {"WLANSTAInfo", "WlanStaInfo", "WLANAssociatedDevice", "AssocDevice"},
		"dhcp_leases":     {"DHCPv4Client", "DHCPClient", "DHCPLease", "LanHostCfg"},
		"ethernet":        {"EthStat", "LANEthIf"},
		"wlan_ssid":       {"WLANCfg", "WLAN", "WLAN5GCfg"},
	} {
		if tbl, name, err := d.FirstTable(ctx, candidates...); err == nil && len(tbl.Rows) > 0 {
			data[key] = tbl.Rows
			data[key+"_table"] = name
			res.Source = "telnet"
		}
	}

	if res.Source == "" {
		if err := d.EnsureWeb(ctx); err == nil {
			if body, _, err := d.web.WebCGI(ctx, "userinfo", nil); err == nil && len(body) > 0 {
				data["web"] = decodeJSONOrRaw(body)
				res.Source = "web"
			}
		}
	}
	if res.Source == "" {
		return nil, describeErr(fmt.Errorf("%w: user information tidak tersedia", ErrUnsupported), d, "user information")
	}
	res.Data = data
	return res, nil
}

// ---- Status: Voice Message ------------------------------------------------

// StatusVoice mengembalikan status VoIP/SIP dan pesan suara.
func (d *Device) StatusVoice(ctx context.Context) (*Result, error) {
	res := &Result{Section: "status", Feature: "voice", Title: "Voice Message"}
	data := map[string]interface{}{}

	for key, candidates := range map[string][]string{
		"service":     {"VoIPService", "VoipService", "VoiceService"},
		"sip_account": {"SipAccount", "SIPAccount", "VoIPAccount"},
		"voice_prof":  {"VoiceProfile", "VoiceProf", "VoipProf"},
		"call_state":  {"CallState", "VoiceCallState", "VoipStatus"},
		"mwi":         {"VoiceMessage", "MWI", "MessageWaiting"},
	} {
		if tbl, name, err := d.FirstTable(ctx, candidates...); err == nil && len(tbl.Rows) > 0 {
			data[key] = tbl.Rows
			data[key+"_table"] = name
			res.Source = "telnet"
		}
	}

	if res.Source == "" {
		res.Warning = "perangkat kemungkinan tidak memiliki modul suara (VoIP) atau firmware tidak mengekspos tabelnya"
		data["available"] = false
		res.Source = "telnet"
	} else {
		data["available"] = true
	}
	res.Data = data
	return res, nil
}

// ---- Status: Remote Management --------------------------------------------

// StatusRemoteManagement mengembalikan status manajemen jarak jauh
// (TR-069/CWMP, ACS, dan akses manajemen).
func (d *Device) StatusRemoteManagement(ctx context.Context) (*Result, error) {
	res := &Result{Section: "status", Feature: "remote_management", Title: "Remote Management"}
	data := map[string]interface{}{}

	for key, candidates := range map[string][]string{
		"tr069":        {"TR069", "TR069Cfg", "TR069Config", "CWMP"},
		"acs":          {"ACSCfg", "ACS", "TR069ACS", "ManagementServer"},
		"mgmt_access":  {"MgmtAccess", "RemoteMgmt", "WebMgmt", "ManagementAccess"},
		"upgrade":      {"FirmwareUpgrade", "UpgradeCfg", "SoftwareUpgrade"},
		"connecting":   {"TR069Session", "CWMPStatus", "TR069Status"},
	} {
		if tbl, name, err := d.FirstTable(ctx, candidates...); err == nil && len(tbl.Rows) > 0 {
			data[key] = tbl.Rows
			data[key+"_table"] = name
			res.Source = "telnet"
		}
	}

	if res.Source == "" {
		if err := d.EnsureWeb(ctx); err == nil {
			if body, _, err := d.web.WebCGI(ctx, "remote_management", nil); err == nil && len(body) > 0 {
				data["web"] = decodeJSONOrRaw(body)
				res.Source = "web"
			}
		}
	}
	if res.Source == "" {
		return nil, describeErr(fmt.Errorf("%w: remote management tidak tersedia", ErrUnsupported), d, "remote management")
	}
	res.Data = data
	return res, nil
}

// ---- helper ---------------------------------------------------------------

func firstNumeric(r Row, keys ...string) (int64, bool) {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != "" {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// HumanDuration mengubah durasi menjadi teks ringkas (mis. "3d 4h 12m").
func HumanDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int64(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	hours := int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	mins := int64(d / time.Minute)
	secs := int64((d - time.Duration(mins)*time.Minute) / time.Second)
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}

// describeErr membungkus error dengan petunjuk konektivitas.
func describeErr(err error, d *Device, what string) error {
	a := d.access
	return fmt.Errorf("%s (host=%s, web=%v, telnet_port=%d): %w",
		what, a.Host, a.Scheme(), a.TelnetPort, err)
}

// encodeJSON adalah util kecil untuk logging data kompleks.
func encodeJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// withSession menambahkan _SESSION_TOKEN ke form aksi tulis bila tersedia.
func (d *Device) withSession(ctx context.Context, form url.Values) url.Values {
	if form == nil {
		form = url.Values{}
	}
	if tok := d.web.SessionToken(ctx); tok != "" {
		form.Set("_SESSION_TOKEN", tok)
	}
	return form
}
