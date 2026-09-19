package modem

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// listTables menjalankan beberapa kandidat tabel lalu menggabungkan hasilnya
// ke dalam map. Mengembalikan true bila minimal satu tabel berhasil dibaca.
func (d *Device) listTables(ctx context.Context, target map[string]interface{}, groups map[string][]string) bool {
	ok := false
	for key, candidates := range groups {
		if tbl, name, err := d.FirstTable(ctx, candidates...); err == nil && tbl != nil && len(tbl.Rows) > 0 {
			target[key] = tbl.Rows
			target[key+"_table"] = name
			ok = true
		}
	}
	return ok
}

// ---- Network: WAN ---------------------------------------------------------

// NetworkWAN mengembalikan konfigurasi WAN (PPPoE/IP/DHCP, VLAN, NAT).
func (d *Device) NetworkWAN(ctx context.Context) (*Result, error) {
	res := &Result{Section: "network", Feature: "wan", Title: "WAN Connection"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"wan_ppp":    {"WANPPPConnection", "WANPPP", "PPPConnection"},
		"wan_ip":     {"WANIPConnection", "WANIP", "WANC", "WANConnection"},
		"wan_common": {"WANC", "WANCommon", "WANCommonIf"},
		"wan_vlan":   {"WANVlan", "VlanCfg", "WANVLAN"},
		"wan_route":  {"WANRoute", "DefaultGateway"},
	})
	if ok {
		res.Source = "telnet"
	}
	if !ok {
		if err := d.EnsureWeb(ctx); err == nil {
			for _, page := range []string{"wan", "wannet", "wan_conn"} {
				if body, _, err := d.web.WebCGI(ctx, page, nil); err == nil && len(body) > 0 {
					data["web"] = decodeJSONOrRaw(body)
					data["web_page"] = page
					res.Source = "web"
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: WAN tidak tersedia", ErrUnsupported), d, "WAN")
	}
	res.Data = data
	return res, nil
}

// ---- Network: LAN ---------------------------------------------------------

// NetworkLAN mengembalikan konfigurasi LAN (IP, DHCP server, binding, lease).
func (d *Device) NetworkLAN(ctx context.Context) (*Result, error) {
	res := &Result{Section: "network", Feature: "lan", Title: "LAN / DHCP"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"lan":          {"LANCfg", "LanCfg", "LANHostCfg", "LAN"},
		"dhcp":         {"DHCPv4IFCfg", "DHCPServer", "DHCPv4Server", "DHCPS"},
		"dhcp_binding": {"DHCPv4Binding", "DHCPBinding", "StaticDHCP"},
		"dhcp_lease":   {"DHCPv4Client", "DHCPClient", "DHCPLease"},
		"lan_ipv6":     {"DHCPv6Server", "LANIPv6Cfg", "IPv6LanCfg"},
	})
	if ok {
		res.Source = "telnet"
	}
	if !ok {
		if err := d.EnsureWeb(ctx); err == nil {
			for _, page := range []string{"lan", "lan_dhcp"} {
				if body, _, err := d.web.WebCGI(ctx, page, nil); err == nil && len(body) > 0 {
					data["web"] = decodeJSONOrRaw(body)
					data["web_page"] = page
					res.Source = "web"
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: LAN tidak tersedia", ErrUnsupported), d, "LAN")
	}
	res.Data = data
	return res, nil
}

// ---- Network: WLAN --------------------------------------------------------

// NetworkWLAN mengembalikan konfigurasi WiFi 2.4G & 5G (SSID, security, ACL,
// dan perangkat yang terhubung).
func (d *Device) NetworkWLAN(ctx context.Context) (*Result, error) {
	res := &Result{Section: "network", Feature: "wlan", Title: "WLAN / WiFi"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"wlan_24":        {"WLANCfg", "WLAN", "WlanCfg", "WLANBaseCfg"},
		"wlan_5":         {"WLAN5GCfg", "WLAN5G", "Wlan5gCfg", "WLANACfg"},
		"wlan_ssid":      {"WLANSSID", "SSIDCfg", "WlanSSID"},
		"wlan_security":  {"WLANSecurity", "WlanSecurity", "WLANAuth"},
		"wlan_acl":       {"WLANACL", "WlanAccessControl", "WLANMacFilter"},
		"wlan_associated": {"WLANSTAInfo", "WlanStaInfo", "WLANAssociatedDevice"},
		"wlan_wps":       {"WPS", "WlanWPS", "WSC"},
	})
	if ok {
		res.Source = "telnet"
	}
	if !ok {
		if err := d.EnsureWeb(ctx); err == nil {
			for _, page := range []string{"wlan", "wlan_basic", "wifi"} {
				if body, _, err := d.web.WebCGI(ctx, page, nil); err == nil && len(body) > 0 {
					data["web"] = decodeJSONOrRaw(body)
					data["web_page"] = page
					res.Source = "web"
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: WLAN tidak tersedia", ErrUnsupported), d, "WLAN")
	}
	res.Data = data
	return res, nil
}

// ---- Network: Routing -----------------------------------------------------

// NetworkRouting mengembalikan tabel routing IPv4 dan IPv6.
func (d *Device) NetworkRouting(ctx context.Context) (*Result, error) {
	res := &Result{Section: "network", Feature: "routing", Title: "Routing"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"routing_v4":     {"Route", "Router", "IPRoute", "RouteCfg"},
		"static_v4":      {"StaticRoute", "RtStatic", "RouteStatic"},
		"routing_table":  {"RoutingTable", "RouteTable", "IPRouteTable"},
		"default_gw":     {"DefaultGateway", "DefaultGW", "Gateway"},
		"policy_route":   {"PolicyRoute", "RoutePolicy"},
		"routing_v6":     {"RouteIPv6", "IPv6Route", "Route6"},
		"static_v6":      {"StaticRoute6", "IPv6StaticRoute"},
		"routing_table6": {"RoutingTable6", "IPv6RouteTable"},
	})
	// Tabel routing dinamis kadang hanya tersedia via perintah `route -n`.
	if routeOut, err := d.telnet.Exec(ctx, "route -n"); err == nil && strings.TrimSpace(routeOut) != "" {
		data["kernel_route"] = strings.TrimSpace(routeOut)
		ok = true
	}
	if ip6Out, err := d.telnet.Exec(ctx, "ip -6 route"); err == nil && strings.TrimSpace(ip6Out) != "" {
		data["kernel_route6"] = strings.TrimSpace(ip6Out)
		ok = true
	}
	if ok {
		res.Source = "telnet"
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: routing tidak tersedia", ErrUnsupported), d, "routing")
	}
	res.Data = data
	return res, nil
}

// ---- Network: DNS ---------------------------------------------------------

// NetworkDNS mengembalikan konfigurasi DNS (server, hosts, domain).
func (d *Device) NetworkDNS(ctx context.Context) (*Result, error) {
	res := &Result{Section: "network", Feature: "dns", Title: "DNS"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"dns_client": {"DNS", "DNSCfg", "DNSClient", "DNSRelay"},
		"dns_hosts":  {"Hosts", "DnsHosts", "HostCfg"},
		"domain":     {"Domain", "DomainName", "DomainCfg"},
		"ddns":       {"DDNS", "DDNSCfg", "DynamicDNS"},
	})
	if ok {
		res.Source = "telnet"
	}
	if !ok {
		if err := d.EnsureWeb(ctx); err == nil {
			for _, page := range []string{"dns", "dns_service"} {
				if body, _, err := d.web.WebCGI(ctx, page, nil); err == nil && len(body) > 0 {
					data["web"] = decodeJSONOrRaw(body)
					res.Source = "web"
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: DNS tidak tersedia", ErrUnsupported), d, "DNS")
	}
	res.Data = data
	return res, nil
}

// ---- Network: Port Binding ------------------------------------------------

// NetworkPortBinding mengembalikan port binding WAN-LAN.
func (d *Device) NetworkPortBinding(ctx context.Context) (*Result, error) {
	res := &Result{Section: "network", Feature: "port_binding", Title: "Port Binding"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"port_binding": {"PortBinding", "WANPortBinding", "LANPortBinding"},
		"lan":          {"LANCfg", "LanCfg"},
		"wlan":         {"WLANCfg", "WLAN"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: port binding tidak tersedia", ErrUnsupported), d, "port binding")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ---- Network: WAN write helpers -------------------------------------------

// UpdateWANField mengubah satu field pada koneksi WAN lalu menyimpan DB.
// Parameter: table (opsional; auto-deteksi), index, field, value.
func (d *Device) UpdateWANField(ctx context.Context, table string, index int, field, value string) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	if table == "" {
		if _, name, err := d.FirstTable(ctx, "WANPPPConnection", "WANIPConnection", "WANC", "WANConnection"); err == nil {
			table = name
		} else {
			return nil, fmt.Errorf("%w: tabel WAN tidak ditemukan", ErrNotFound)
		}
	}
	out, err := d.telnet.DBSet(ctx, table, index, field, value)
	if err != nil {
		return nil, err
	}
	save, saveErr := d.telnet.DBSave(ctx)
	return &Result{
		Section: "network", Feature: "wan_update", Title: "Update WAN",
		Source: "telnet",
		Data: map[string]interface{}{
			"set":  out,
			"save": save,
		},
		Error: errString(saveErr),
	}, nil
}

// ---- Network: WLAN write helpers ------------------------------------------

// SetWLANSSID mengubah SSID WiFi. band = "2.4g" atau "5g".
func (d *Device) SetWLANSSID(ctx context.Context, band string, index int, ssid string) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	var candidates []string
	if band == "5g" {
		candidates = []string{"WLAN5GCfg", "WLAN5G", "Wlan5gCfg", "WLANACfg"}
	} else {
		candidates = []string{"WLANCfg", "WLAN", "WlanCfg", "WLANBaseCfg"}
	}
	tbl, name, err := d.FirstTable(ctx, candidates...)
	if err != nil {
		return nil, fmt.Errorf("%w: tabel WLAN band %s tidak ditemukan: %v", ErrNotFound, band, err)
	}
	_ = tbl
	field := pickField(tbl.Rows[0], "SSID1", "SSID", "SSIDName", "ESSID")
	if field == "" {
		return nil, fmt.Errorf("%w: field SSID tidak dikenali pada tabel %s", ErrUnsupported, name)
	}
	out, err := d.telnet.DBSet(ctx, name, index, field, ssid)
	if err != nil {
		return nil, err
	}
	save, _ := d.telnet.DBSave(ctx)
	return &Result{
		Section: "network", Feature: "wlan_ssid", Title: "Set SSID " + band,
		Source: "telnet",
		Data:   map[string]interface{}{"table": name, "field": field, "set": out, "save": save},
	}, nil
}

// ---- Network: DHCP write helpers ------------------------------------------

// ToggleDHCP mengaktifkan/menonaktifkan DHCP server LAN.
func (d *Device) ToggleDHCP(ctx context.Context, enable bool) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	tbl, name, err := d.FirstTable(ctx, "DHCPv4IFCfg", "DHCPServer", "DHCPv4Server", "DHCPS")
	if err != nil {
		return nil, fmt.Errorf("%w: tabel DHCP tidak ditemukan: %v", ErrNotFound, err)
	}
	field := pickField(tbl.Rows[0], "Enable", "DHCPServerEnable", "DHCPEnable", "Enabled")
	if field == "" {
		return nil, fmt.Errorf("%w: field enable DHCP tidak dikenali pada %s", ErrUnsupported, name)
	}
	val := "0"
	if enable {
		val = "1"
	}
	out, err := d.telnet.DBSet(ctx, name, 0, field, val)
	if err != nil {
		return nil, err
	}
	save, _ := d.telnet.DBSave(ctx)
	return &Result{
		Section: "network", Feature: "dhcp_toggle", Title: "Toggle DHCP",
		Source: "telnet",
		Data:   map[string]interface{}{"table": name, "field": field, "value": val, "set": out, "save": save},
	}, nil
}

// pickField mencari nama field pertama yang ada pada row.
func pickField(r Row, names ...string) string {
	for _, n := range names {
		if _, ok := r[n]; ok {
			return n
		}
	}
	return ""
}

// asInt mengubah nilai string menjadi int dengan aman.
func asInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// formFromMap membentuk url.Values dari map string.
func formFromMap(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		v.Set(k, val)
	}
	return v
}

// errString mengubah error menjadi string kosong bila nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
