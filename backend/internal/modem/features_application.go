package modem

import (
	"context"
	"fmt"
)

// ---- Application ----------------------------------------------------------

// ApplicationUPnP mengembalikan konfigurasi & mapping UPnP.
func (d *Device) ApplicationUPnP(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "upnp", Title: "UPnP"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"upnp":         {"UPnP", "UPnPCfg", "UPnPConfig"},
		"upnp_mapping": {"UPnPPortMapping", "UPnPMapping", "PortMapping", "UPnPMap"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: UPnP tidak tersedia", ErrUnsupported), d, "UPnP")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ApplicationDDNS mengembalikan konfigurasi Dynamic DNS.
func (d *Device) ApplicationDDNS(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "ddns", Title: "DDNS"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"ddns": {"DDNS", "DDNSCfg", "DynamicDNS", "DDNSClient"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: DDNS tidak tersedia", ErrUnsupported), d, "DDNS")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ApplicationDMZ mengembalikan konfigurasi DMZ host.
func (d *Device) ApplicationDMZ(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "dmz", Title: "DMZ Host"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"dmz": {"DMZ", "DMZHost", "DMZCfg", "DmzHost"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: DMZ tidak tersedia", ErrUnsupported), d, "DMZ")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ApplicationPortForwarding mengembalikan aturan port forwarding & port trigger.
func (d *Device) ApplicationPortForwarding(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "port_forwarding", Title: "Port Forwarding"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"port_forward": {"PortForward", "PortForwarding", "VrtsPortFwd", "PortFwd", "NATPortForward"},
		"port_trigger": {"PortTrigger", "PortTriggering", "TriggerPort"},
		"application":  {"ApplicationList", "AppList", "Application"},
		"port_map":     {"VrtsPortFwd", "PortMap", "PortMapping"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: port forwarding tidak tersedia", ErrUnsupported), d, "port forwarding")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ApplicationSNTP mengembalikan konfigurasi waktu/SNTP.
func (d *Device) ApplicationSNTP(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "sntp", Title: "SNTP / Time"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"sntp": {"SNTP", "SNTPCfg", "NTP", "TimeCfg", "Time"},
		"time": {"Time", "DeviceTime", "SysTime"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: SNTP tidak tersedia", ErrUnsupported), d, "SNTP")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ApplicationMulticast mengembalikan konfigurasi IGMP/multicast.
func (d *Device) ApplicationMulticast(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "multicast", Title: "Multicast / IGMP"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"igmp":       {"IGMP", "IGMPCfg", "Multicast", "IGMPProxy", "MldCfg"},
		"igmp_group": {"IGMPGroup", "MulticastGroup", "IGMPMember"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: multicast tidak tersedia", ErrUnsupported), d, "multicast")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ApplicationUSB mengembalikan status & konfigurasi penyimpanan USB.
func (d *Device) ApplicationUSB(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "usb", Title: "USB Storage"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"usb":      {"USB", "USBCfg", "USBStorage", "UsbInfo"},
		"ftp":      {"FTP", "FTPCfg", "FtpServer"},
		"samba":    {"Samba", "SambaCfg", "SMB"},
		"dms":      {"DMS", "DMSConfig", "DLNA"},
		"print_srv": {"PrintServer", "USBPrint", "PrinterSrv"},
	})
	// Fallback: lihat mount USB langsung dari shell.
	if out, err := d.telnet.Exec(ctx, "ls /mnt 2>/dev/null"); err == nil && containsAnyUsb(out) {
		data["mnt"] = out
		ok = true
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: USB tidak tersedia", ErrUnsupported), d, "USB")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ApplicationVoIP mengembalikan konfigurasi VoIP/SIP (akun, media, fax).
func (d *Device) ApplicationVoIP(ctx context.Context) (*Result, error) {
	res := &Result{Section: "application", Feature: "voip", Title: "VoIP"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"sip_account": {"SipAccount", "SIPAccount", "VoIPAccount"},
		"voip_adv":    {"VoIPAdvanced", "VoipAdv", "VoiceAdvanced"},
		"voip_media":  {"VoIPMedia", "VoiceMedia", "MediaCfg"},
		"voip_fax":    {"FAX", "VoIPFax", "FaxCfg"},
		"caller_id":   {"CallerId", "CallerID", "CLIP"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: VoIP tidak tersedia", ErrUnsupported), d, "VoIP")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// SetUPnPEnable mengaktifkan/menonaktifkan UPnP.
func (d *Device) SetUPnPEnable(ctx context.Context, enable bool) (*Result, error) {
	return d.toggleSimple(ctx, "application", "upnp_toggle", "UPnP",
		[]string{"UPnP", "UPnPCfg", "UPnPConfig"},
		[]string{"Enable", "UPnPEnable", "Enabled"})
}

func containsAnyUsb(s string) bool {
	for _, k := range []string{"usb", "sda", "sdb"} {
		if containsFold(s, k) {
			return true
		}
	}
	return false
}
