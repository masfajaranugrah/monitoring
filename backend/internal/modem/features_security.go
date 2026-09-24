package modem

import (
	"context"
	"fmt"
)

// ---- Security -------------------------------------------------------------

// SecurityFirewall mengembalikan konfigurasi firewall (level, serangan, dst).
func (d *Device) SecurityFirewall(ctx context.Context) (*Result, error) {
	res := &Result{Section: "security", Feature: "firewall", Title: "Firewall"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"firewall":     {"Firewall", "FirewallCfg", "FWCfg", "FWLevel"},
		"fw_attack":    {"FWAttack", "FirewallAttack", "AttackProtect"},
		"fw_service":   {"FWService", "FirewallService"},
		"fw_schedule":  {"FWSchedule", "FirewallSchedule"},
		"fw_sc":        {"FWSC", "ServiceControl", "FirewallServiceControl"},
	})
	if !ok {
		if err := d.EnsureWeb(ctx); err == nil {
			for _, page := range []string{"firewall", "security"} {
				if body, _, err := d.web.WebCGI(ctx, page, nil); err == nil && len(body) > 0 {
					data["web"] = decodeJSONOrRaw(body)
					data["web_page"] = page
					ok = true
					break
				}
			}
		}
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: firewall tidak tersedia", ErrUnsupported), d, "firewall")
	}
	res.Source = "telnet"
	if _, hasWeb := data["web"]; hasWeb {
		res.Source = "web"
	}
	res.Data = data
	return res, nil
}

// SecurityIPFilter mengembalikan aturan IP filter.
func (d *Device) SecurityIPFilter(ctx context.Context) (*Result, error) {
	res := &Result{Section: "security", Feature: "ip_filter", Title: "IP Filter"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"ip_filter": {"IpFilter", "IPFilter", "FWIP", "IPFilterRule"},
		"acl":       {"ACL", "AclCfg", "AccessControl"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: IP filter tidak tersedia", ErrUnsupported), d, "IP filter")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// SecurityMACFilter mengembalikan aturan MAC filter.
func (d *Device) SecurityMACFilter(ctx context.Context) (*Result, error) {
	res := &Result{Section: "security", Feature: "mac_filter", Title: "MAC Filter"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"mac_filter": {"MacFilter", "MACFilter", "FWMAC", "MACFilterRule", "WLANMacFilter"},
		"acl":        {"ACL", "AclCfg"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: MAC filter tidak tersedia", ErrUnsupported), d, "MAC filter")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// SecurityURLFilter mengembalikan aturan URL filter.
func (d *Device) SecurityURLFilter(ctx context.Context) (*Result, error) {
	res := &Result{Section: "security", Feature: "url_filter", Title: "URL Filter"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"url_filter": {"UrlFilter", "URLFilter", "FWURL", "URLFilterRule", "ParentalControl"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: URL filter tidak tersedia", ErrUnsupported), d, "URL filter")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// SecurityALG mengembalikan konfigurasi ALG (FTP/SIP/RTSP/H323/PPTP).
func (d *Device) SecurityALG(ctx context.Context) (*Result, error) {
	res := &Result{Section: "security", Feature: "alg", Title: "ALG"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"alg": {"ALG", "ALGCfg", "AlgCfg", "ApplicationLayerGateway"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: ALG tidak tersedia", ErrUnsupported), d, "ALG")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// SetFirewallEnable mengaktifkan/menonaktifkan firewall.
func (d *Device) SetFirewallEnable(ctx context.Context, enable bool) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	tbl, name, err := d.FirstTable(ctx, "Firewall", "FirewallCfg", "FWCfg", "FWLevel")
	if err != nil {
		return nil, fmt.Errorf("%w: tabel firewall tidak ditemukan: %v", ErrNotFound, err)
	}
	field := pickField(tbl.Rows[0], "Enable", "FirewallEnable", "FWEnable", "Enabled")
	if field == "" {
		return nil, fmt.Errorf("%w: field enable firewall tidak dikenali pada %s", ErrUnsupported, name)
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
		Section: "security", Feature: "firewall_toggle", Title: "Toggle Firewall",
		Source: "telnet",
		Data:   map[string]interface{}{"table": name, "field": field, "value": val, "set": out, "save": save},
	}, nil
}

// SetALGEnable mengaktifkan/menonaktifkan satu anggota ALG (ftp/sip/rtsp/h323/pptp).
func (d *Device) SetALGEnable(ctx context.Context, proto string, enable bool) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	tbl, name, err := d.FirstTable(ctx, "ALG", "ALGCfg", "AlgCfg")
	if err != nil {
		return nil, fmt.Errorf("%w: tabel ALG tidak ditemukan: %v", ErrNotFound, err)
	}
	field := pickField(tbl.Rows[0],
		proto+"Enable", "Enable"+proto,
		"Enable"+stringsTitle(proto), stringsTitle(proto)+"Enable")
	if field == "" {
		return nil, fmt.Errorf("%w: field ALG %q tidak dikenali pada %s", ErrUnsupported, proto, name)
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
		Section: "security", Feature: "alg_toggle", Title: "Toggle ALG " + proto,
		Source: "telnet",
		Data:   map[string]interface{}{"table": name, "field": field, "value": val, "set": out, "save": save},
	}, nil
}

// stringsTitle mengapitalkan huruf pertama.
func stringsTitle(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 32
	}
	return string(b)
}
