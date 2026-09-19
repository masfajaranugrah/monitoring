package modem

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FeatureInfo mendeskripsikan satu fitur yang diekspos API.
type FeatureInfo struct {
	Section string `json:"section"`
	Name    string `json:"name"`
	Title   string `json:"title"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Access  string `json:"access"` // web | telnet | combined
}

// Catalog adalah daftar seluruh fitur yang tersedia di API modem.
func Catalog() []FeatureInfo {
	return []FeatureInfo{
		// Status
		{"status", "device", "Device Information", "GET", "/status/device", "combined"},
		{"status", "network_info", "Network Information", "GET", "/status/network-info", "combined"},
		{"status", "user_info", "User Information", "GET", "/status/user-info", "combined"},
		{"status", "voice", "Voice Message", "GET", "/status/voice", "telnet"},
		{"status", "remote_management", "Remote Management", "GET", "/status/remote-management", "combined"},

		// Network
		{"network", "wan", "WAN Connection", "GET", "/network/wan", "combined"},
		{"network", "wan", "Update WAN field", "POST", "/network/wan", "telnet"},
		{"network", "lan", "LAN / DHCP", "GET", "/network/lan", "combined"},
		{"network", "lan", "Toggle DHCP", "POST", "/network/lan/dhcp", "telnet"},
		{"network", "wlan", "WLAN / WiFi", "GET", "/network/wlan", "combined"},
		{"network", "wlan", "Set SSID", "POST", "/network/wlan/ssid", "telnet"},
		{"network", "routing", "Routing", "GET", "/network/routing", "telnet"},
		{"network", "dns", "DNS", "GET", "/network/dns", "combined"},
		{"network", "port_binding", "Port Binding", "GET", "/network/port-binding", "telnet"},

		// Security
		{"security", "firewall", "Firewall", "GET", "/security/firewall", "combined"},
		{"security", "firewall", "Toggle Firewall", "POST", "/security/firewall", "telnet"},
		{"security", "ip_filter", "IP Filter", "GET", "/security/ip-filter", "telnet"},
		{"security", "mac_filter", "MAC Filter", "GET", "/security/mac-filter", "telnet"},
		{"security", "url_filter", "URL Filter", "GET", "/security/url-filter", "telnet"},
		{"security", "alg", "ALG", "GET", "/security/alg", "telnet"},
		{"security", "alg", "Toggle ALG", "POST", "/security/alg", "telnet"},

		// Application
		{"application", "upnp", "UPnP", "GET", "/application/upnp", "telnet"},
		{"application", "upnp", "Toggle UPnP", "POST", "/application/upnp", "telnet"},
		{"application", "ddns", "DDNS", "GET", "/application/ddns", "telnet"},
		{"application", "dmz", "DMZ Host", "GET", "/application/dmz", "telnet"},
		{"application", "port_forwarding", "Port Forwarding", "GET", "/application/port-forwarding", "telnet"},
		{"application", "sntp", "SNTP / Time", "GET", "/application/sntp", "telnet"},
		{"application", "multicast", "Multicast / IGMP", "GET", "/application/multicast", "telnet"},
		{"application", "usb", "USB Storage", "GET", "/application/usb", "telnet"},
		{"application", "voip", "VoIP", "GET", "/application/voip", "telnet"},

		// Manage
		{"manage", "device", "Device Management", "GET", "/manage/device", "telnet"},
		{"manage", "users", "User Management", "GET", "/manage/users", "telnet"},
		{"manage", "users", "Update User", "POST", "/manage/users", "telnet"},
		{"manage", "reboot", "Reboot", "POST", "/manage/reboot", "combined"},
		{"manage", "factory_reset", "Factory Reset", "POST", "/manage/factory-reset", "combined"},
		{"manage", "config_backup", "Download Config", "GET", "/manage/config/backup", "web"},
		{"manage", "config_restore", "Restore Config", "POST", "/manage/config/restore", "web"},
		{"manage", "firmware_upgrade", "Firmware Upgrade", "POST", "/manage/firmware", "web"},
		{"manage", "time", "Time Settings", "GET", "/manage/time", "telnet"},
		{"manage", "time", "Set Time", "POST", "/manage/time", "telnet"},
		{"manage", "log", "Log Management", "GET", "/manage/log", "telnet"},

		// Diagnosis
		{"diagnosis", "ping", "Ping Diagnosis", "POST", "/diagnosis/ping", "combined"},
		{"diagnosis", "traceroute", "Trace Route", "POST", "/diagnosis/traceroute", "combined"},
		{"diagnosis", "arp_table", "ARP Table", "GET", "/diagnosis/arp", "telnet"},
		{"diagnosis", "mac_table", "MAC Table", "GET", "/diagnosis/mac-table", "telnet"},
		{"diagnosis", "optical", "Optical / PON Status", "GET", "/diagnosis/optical", "telnet"},
		{"diagnosis", "loopback", "Loopback Detection", "GET", "/diagnosis/loopback", "telnet"},

		// Help
		{"help", "info", "Help / Device Info", "GET", "/help", "combined"},
		{"help", "catalog", "Feature Catalog", "GET", "/features", "-"},
		{"help", "probe", "Connectivity Probe", "GET", "/probe", "combined"},
	}
}

// ---- Help -----------------------------------------------------------------

// HelpInfo mengembalikan informasi bantuan perangkat: versi firmware, model,
// dan halaman help yang tersedia.
func (d *Device) HelpInfo(ctx context.Context) (*Result, error) {
	res := &Result{Section: "help", Feature: "info", Title: "Help / Device Info"}
	data := map[string]interface{}{
		"model":        "ZXHN F663NV9",
		"api_version":  "1.0",
		"generated_at": time.Now().Format(time.RFC3339),
	}

	if err := d.EnsureTelnet(ctx); err == nil {
		if out, err := d.telnet.Exec(ctx, "cat /etc/version 2>/dev/null"); err == nil && strings.TrimSpace(out) != "" {
			data["version_file"] = strings.TrimSpace(out)
		}
		if out, err := d.telnet.Exec(ctx, "uname -a"); err == nil && strings.TrimSpace(out) != "" {
			data["uname"] = strings.TrimSpace(out)
		}
		if tbl, _, err := d.FirstTable(ctx, "DeviceInfo", "Device", "SysInfo"); err == nil && len(tbl.Rows) > 0 {
			data["device_info"] = tbl.Rows
		}
	}

	// Halaman help bawaan web UI.
	if err := d.EnsureWeb(ctx); err == nil {
		for _, page := range []string{"help", "help_frame", "about"} {
			if body, status, err := d.web.WebCGI(ctx, page, nil); err == nil && status == 200 && len(body) > 0 {
				data["web_help"] = map[string]interface{}{
					"page": page,
					"body": string(body),
				}
				break
			}
		}
	}

	res.Source = "combined"
	res.Data = data
	return res, nil
}

// Probe memeriksa konektivitas jalur web dan telnet dan mengembalikan ringkasan.
func (d *Device) Probe(ctx context.Context) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	probe := map[string]interface{}{
		"host": d.access.Host,
		"web":  map[string]interface{}{},
		"telnet": map[string]interface{}{},
	}

	web := probe["web"].(map[string]interface{})
	web["scheme"] = d.access.Scheme()
	web["url"] = d.web.BaseURL()
	if err := d.EnsureWeb(ctx); err != nil {
		web["ok"] = false
		web["error"] = err.Error()
	} else {
		web["ok"] = true
		web["style"] = d.web.style
	}

	tel := probe["telnet"].(map[string]interface{})
	port := d.access.TelnetPort
	if port <= 0 {
		port = 23
	}
	tel["port"] = port
	if err := d.EnsureTelnet(ctx); err != nil {
		tel["ok"] = false
		tel["error"] = err.Error()
	} else {
		tel["ok"] = true
		if out, err := d.telnet.Exec(ctx, "echo __OK__"); err == nil && strings.Contains(out, "__OK__") {
			tel["shell"] = true
		}
	}

	webOK, _ := web["ok"].(bool)
	telOK, _ := tel["ok"].(bool)
	probe["reachable"] = webOK || telOK

	// Petunjuk troubleshooting bila tidak ada jalur yang hidup.
	if !webOK && !telOK {
		probe["hint"] = fmt.Sprintf(
			"Perangkat %s tidak dapat dijangkau. Pastikan routing/VPN ke IP pelanggan aktif, "+
				"port web tervalidasi (scheme/port), dan telnet diaktifkan pada perangkat.", d.access.Host)
	}

	return &Result{
		Section: "help", Feature: "probe", Title: "Connectivity Probe",
		Source: "combined", Data: probe,
	}, nil
}
