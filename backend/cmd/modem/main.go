// Command modem adalah CLI untuk menguji client modem ZTE (mis. ZXHN F663NV9)
// langsung dari terminal tanpa melalui HTTP API monitoring.
//
// Contoh:
//
//	modem -host 192.168.1.1 -user admin -pass admin probe
//	modem -host 192.168.1.1 status device
//	modem -host 192.168.1.1 network wlan
//	modem -host 192.168.1.1 -target 8.8.8.8 diagnosis ping
//	modem -host 192.168.1.1 -tuser root -tpass Zte521 raw "sendcmd 1 DB get DeviceInfo"
//	modem features
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"monitoring/internal/modem"
)

func main() {
	var (
		host       = flag.String("host", "", "alamat IP/host perangkat")
		https      = flag.Bool("https", false, "pakai HTTPS untuk web management")
		port       = flag.Int("port", 80, "port web management")
		user       = flag.String("user", "admin", "username web")
		pass       = flag.String("pass", "admin", "password web")
		telnetPort = flag.Int("tport", 23, "port telnet")
		telnetUser = flag.String("tuser", "root", "username telnet")
		telnetPass = flag.String("tpass", "", "password telnet")
		target     = flag.String("target", "8.8.8.8", "host tujuan untuk ping/traceroute")
		timeout    = flag.Duration("timeout", 40*time.Second, "timeout total operasi")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	if args[0] == "features" {
		printJSON(modem.Catalog())
		return
	}

	if *host == "" {
		fmt.Fprintln(os.Stderr, "error: -host wajib diisi")
		os.Exit(2)
	}

	access := modem.Access{
		Host:       *host,
		HTTPS:      *https,
		HTTPPort:   *port,
		WebUser:    *user,
		WebPass:    *pass,
		TelnetPort: *telnetPort,
		TelnetUser: *telnetUser,
		TelnetPass: *telnetPass,
	}
	dev := modem.NewDevice(access)
	defer dev.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	section := strings.ToLower(args[0])
	action := ""
	if len(args) > 1 {
		action = strings.ToLower(args[1])
	}

	var (
		res *modem.Result
		err error
	)
	switch section {
	case "probe":
		res, err = dev.Probe(ctx)
	case "help":
		res, err = dev.HelpInfo(ctx)
	case "status":
		res, err = dispatchStatus(ctx, dev, action)
	case "network":
		res, err = dispatchNetwork(ctx, dev, action)
	case "security":
		res, err = dispatchSecurity(ctx, dev, action)
	case "application", "app":
		res, err = dispatchApplication(ctx, dev, action)
	case "manage":
		res, err = dispatchManage(ctx, dev, action)
	case "diagnosis", "diag":
		res, err = dispatchDiagnosis(ctx, dev, action, *target)
	case "raw":
		if len(args) < 2 {
			fatal("raw butuh perintah, mis: raw \"sendcmd 1 DB get DeviceInfo\"")
		}
		cmd := strings.Join(args[1:], " ")
		out, rerr := dev.Telnet().Run(ctx, cmd)
		res, err = &modem.Result{Section: "help", Feature: "raw", Source: "telnet", Data: out}, rerr
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		printJSON(map[string]interface{}{
			"section": section, "feature": action, "error": err.Error(),
		})
		os.Exit(1)
	}
	printJSON(res)
}

func dispatchStatus(ctx context.Context, d *modem.Device, a string) (*modem.Result, error) {
	switch a {
	case "device", "device-info", "":
		return d.StatusDevice(ctx)
	case "network", "network-info":
		return d.StatusNetworkInfo(ctx)
	case "user", "user-info":
		return d.StatusUserInfo(ctx)
	case "voice":
		return d.StatusVoice(ctx)
	case "remote", "remote-management":
		return d.StatusRemoteManagement(ctx)
	}
	fatal("status tidak dikenal: " + a)
	return nil, nil
}

func dispatchNetwork(ctx context.Context, d *modem.Device, a string) (*modem.Result, error) {
	switch a {
	case "wan", "":
		return d.NetworkWAN(ctx)
	case "lan":
		return d.NetworkLAN(ctx)
	case "wlan", "wifi":
		return d.NetworkWLAN(ctx)
	case "routing", "route":
		return d.NetworkRouting(ctx)
	case "dns":
		return d.NetworkDNS(ctx)
	case "port-binding", "port_binding":
		return d.NetworkPortBinding(ctx)
	}
	fatal("network tidak dikenal: " + a)
	return nil, nil
}

func dispatchSecurity(ctx context.Context, d *modem.Device, a string) (*modem.Result, error) {
	switch a {
	case "firewall", "":
		return d.SecurityFirewall(ctx)
	case "ip-filter", "ip":
		return d.SecurityIPFilter(ctx)
	case "mac-filter", "mac":
		return d.SecurityMACFilter(ctx)
	case "url-filter", "url":
		return d.SecurityURLFilter(ctx)
	case "alg":
		return d.SecurityALG(ctx)
	}
	fatal("security tidak dikenal: " + a)
	return nil, nil
}

func dispatchApplication(ctx context.Context, d *modem.Device, a string) (*modem.Result, error) {
	switch a {
	case "upnp", "":
		return d.ApplicationUPnP(ctx)
	case "ddns":
		return d.ApplicationDDNS(ctx)
	case "dmz":
		return d.ApplicationDMZ(ctx)
	case "port-forwarding", "port-forward":
		return d.ApplicationPortForwarding(ctx)
	case "sntp", "time":
		return d.ApplicationSNTP(ctx)
	case "multicast", "igmp":
		return d.ApplicationMulticast(ctx)
	case "usb":
		return d.ApplicationUSB(ctx)
	case "voip":
		return d.ApplicationVoIP(ctx)
	}
	fatal("application tidak dikenal: " + a)
	return nil, nil
}

func dispatchManage(ctx context.Context, d *modem.Device, a string) (*modem.Result, error) {
	switch a {
	case "device", "":
		return d.ManageDevice(ctx)
	case "users":
		return d.ManageUsers(ctx)
	case "time":
		return d.ManageTime(ctx)
	case "log":
		return d.ManageLog(ctx)
	case "reboot":
		return d.ManageReboot(ctx)
	}
	fatal("manage tidak dikenal: " + a)
	return nil, nil
}

func dispatchDiagnosis(ctx context.Context, d *modem.Device, a, target string) (*modem.Result, error) {
	switch a {
	case "ping", "":
		return d.DiagnosisPing(ctx, target, 4, 0)
	case "traceroute", "trace":
		return d.DiagnosisTraceroute(ctx, target, 15)
	case "arp":
		return d.DiagnosisARPTable(ctx)
	case "mac-table", "mac":
		return d.DiagnosisMACTable(ctx)
	case "optical", "pon":
		return d.DiagnosisOptical(ctx)
	case "loopback":
		return d.DiagnosisLoopback(ctx)
	}
	fatal("diagnosis tidak dikenal: " + a)
	return nil, nil
}

func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "encode error:", err)
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(2)
}

func usage() {
	fmt.Fprintln(os.Stderr, `modem — client CLI untuk perangkat ZTE ZXHN F663NV9

Pemakaian:
  modem [flags] probe
  modem [flags] help
  modem [flags] status   [device|network|user|voice|remote]
  modem [flags] network  [wan|lan|wlan|routing|dns|port-binding]
  modem [flags] security [firewall|ip-filter|mac-filter|url-filter|alg]
  modem [flags] app      [upnp|ddns|dmz|port-forwarding|sntp|multicast|usb|voip]
  modem [flags] manage   [device|users|time|log|reboot]
  modem [flags] diagnosis [ping|traceroute|arp|mac-table|optical|loopback]
  modem [flags] raw "perintah shell"
  modem features`)
}
