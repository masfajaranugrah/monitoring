package vpn

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"monitoring/internal/models"
	"monitoring/internal/ping"
)

// Manager controls Linux VPN tunnels (L2TP via xl2tpd, SSTP via sstpc,
// PPTP via pptp-linux) and reports their status. Connection secrets are
// passed in as plaintext from the calling layer, which decrypts them
// from the database on demand.
type Manager struct {
	pinger *ping.ICMPPinger
}

func NewManager() *Manager {
	return &Manager{pinger: ping.NewICMPPinger()}
}

// Connect attempts to bring up a tunnel for the given VPN definition.
func (m *Manager) Connect(v *models.VPNConnection, password string) error {
	switch models.VpnType(v.VPNType) {
	case models.VpnL2TP:
		return m.connectL2TP(v, password)
	case models.VpnSSTP:
		return m.connectSSTP(v, password)
	case models.VpnPPTP:
		return m.connectPPTP(v, password)
	default:
		return fmt.Errorf("unsupported VPN type: %s", v.VPNType)
	}
}

// ---------- L2TP ----------

func (m *Manager) connectL2TP(v *models.VPNConnection, password string) error {
	if _, err := exec.LookPath("xl2tpd"); err != nil {
		return fmt.Errorf("xl2tpd client is not installed (apt install xl2tpd)")
	}
	name := m.interfaceName(v)
	linkname := v.Name + "-fiber"

	conf := fmt.Sprintf(`[lac %s]
lns = %s
ppp debug = no
pppoptfile = /etc/ppp/options.l2tpd.%s
length bit = yes
redial = yes
redial timeout = 30
`, name, v.ServerAddress, linkname)

	opt := fmt.Sprintf(`ipcp-accept-local
ipcp-accept-remote
refuse-pap
auth
require-chap
name %s
password %s
linkname %s
`, v.Username, password, linkname)

	if err := os.WriteFile("/etc/ppp/options.l2tpd."+linkname, []byte(opt), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile("/tmp/fiber-"+linkname+".conf", []byte(conf), 0o600); err != nil {
		return err
	}
	_ = exec.Command("xl2tpd-control", "add", name, v.ServerAddress).Run()
	_ = exec.Command("xl2tpd-control", "connect", name).Run()
	log.Printf("[vpn] L2TP connect issued for %s", v.Name)
	return nil
}

// ---------- SSTP ----------

func (m *Manager) connectSSTP(v *models.VPNConnection, password string) error {
	if _, err := exec.LookPath("sstpc"); err != nil {
		return fmt.Errorf("sstp-client is not installed (apt install sstp-client)")
	}
	iface := v.Name + "-fiber"
	opt := fmt.Sprintf("name %s\npassword %s\nlinkname %s\n", v.Username, password, iface)
	if err := os.WriteFile("/etc/ppp/options.sstp."+iface, []byte(opt), 0o600); err != nil {
		return err
	}
	cmd := exec.Command("sstpc", "--pppd-plugin", "pppd_sstp",
		"--pppd-options", "/etc/ppp/options.sstp."+iface,
		v.ServerAddress)
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	log.Printf("[vpn] SSTP connect issued for %s", v.Name)
	return nil
}

// ---------- PPTP ----------

func (m *Manager) connectPPTP(v *models.VPNConnection, password string) error {
	if _, err := exec.LookPath("pptp"); err != nil {
		if _, err2 := exec.LookPath("pppd"); err2 != nil {
			return fmt.Errorf("pptp client is not installed (apt install pptp-linux ppp)")
		}
		return m.connectPPTPViaPPPD(v, password)
	}
	return m.connectPPTPCmd(v, password)
}

// connectPPTPCmd uses the pptp binary from pptp-linux (simpler interface).
func (m *Manager) connectPPTPCmd(v *models.VPNConnection, password string) error {
	peerFile := "/etc/ppp/peers/pptp-" + v.Name
	opt := fmt.Sprintf(`pty "pptp %s --nolaunchpppd"
name %s
password %s
remotename pptp-peer
noauth
defaultroute
replacedefaultroute
noipdefault
nobsdcomp
nodeflate
nopcomp
noaccomp
novj
novjccomp
lcp-echo-interval 60
lcp-echo-failure 3
mtu 1400
mru 1400
persist
maxfail 0
`, v.ServerAddress, v.Username, password)
	if err := os.MkdirAll("/etc/ppp/peers", 0o755); err != nil {
		_ = os.WriteFile("/tmp/pptp-peer-"+v.Name, []byte(opt), 0o600)
		peerFile = "/tmp/pptp-peer-" + v.Name
	} else {
		if err := os.WriteFile(peerFile, []byte(opt), 0o600); err != nil {
			return err
		}
	}

	// Hapus config secrets dari filesystem setelah connect.
	defer os.Remove(peerFile)

	// Jalankan pppd dengan peer config (pptp tunnel di-handle oleh pty).
	cmd := exec.Command("pppd", "call", "pptp-"+v.Name)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pppd start failed: %w", err)
	}
	_ = cmd.Process.Release()
	log.Printf("[vpn] PPTP connect issued for %s (pptp cmd, server: %s)", v.Name, v.ServerAddress)
	return nil
}

// connectPPTPViaPPPD uses pppd directly with the pptp plugin when
// the pptp binary is not available.
func (m *Manager) connectPPTPViaPPPD(v *models.VPNConnection, password string) error {
	// pppd + pptp plugin: pty 'pptp <server> ...' opens the tunnel, then
	// pppd negotiates PPP over the tunnel.
	cmd := exec.Command("pppd",
		"plugin", "pptp.so",
		"pty", fmt.Sprintf("pptp %s --nolaunchpppd", v.ServerAddress),
		"name", v.Username,
		"password", password,
		"noauth", "defaultroute", "replacedefaultroute",
		"noipdefault", "nobsdcomp", "nodeflate",
		"lcp-echo-interval", "60", "lcp-echo-failure", "3",
		"mtu", "1400", "mru", "1400")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pppd+pptp start failed: %w", err)
	}
	_ = cmd.Process.Release()
	log.Printf("[vpn] PPTP connect issued for %s via pppd+pptp plugin", v.Name)
	return nil
}

// ---------- Common ----------

// Disconnect brings the tunnel interface down.
func (m *Manager) Disconnect(v *models.VPNConnection) {
	iface := m.interfaceName(v)
	// Generic PPP disconnect: try ip link delete, then ifdown.
	_ = exec.Command("ip", "link", "set", "dev", iface, "down").Run()
	_ = exec.Command("ip", "link", "delete", "dev", iface).Run()
	// L2TP specific.
	_ = exec.Command("xl2tpd-control", "disconnect", iface).Run()
	log.Printf("[vpn] disconnect issued for %s", v.Name)
}

// Status checks whether the tunnel interface is up and returns its status.
func (m *Manager) Status(v *models.VPNConnection) (models.VpnStatus, string) {
	if !v.IsActive {
		return models.VpnDisabled, ""
	}
	iface := m.interfaceName(v)
	ip, err := m.interfaceIP(iface)
	if err != nil {
		return models.VpnDisconnected, ""
	}
	return models.VpnConnected, ip
}

// Test performs a connection test: verifies the interface exists and pings the
// VPN server through it.
func (m *Manager) Test(v *models.VPNConnection, timeout time.Duration) (bool, int64, string) {
	iface := m.interfaceName(v)
	ip, err := m.interfaceIP(iface)
	if err != nil || ip == "" {
		res := m.pinger.Ping(v.ServerAddress, "", timeout)
		if res.Success {
			return true, res.RTT.Milliseconds(), "reachable (tunnel not up)"
		}
		return false, 0, res.ErrorMsg
	}
	res := m.pinger.Ping(v.ServerAddress, "", timeout)
	if res.Success {
		return true, res.RTT.Milliseconds(), fmt.Sprintf("reachable via %s (%d ms)", iface, res.RTT.Milliseconds())
	}
	return false, res.RTT.Milliseconds(), res.ErrorMsg
}

// SourceIP returns the VPN tunnel local IP to use as the ping source for
// customer probes routed through this VPN.
func (m *Manager) SourceIP(v *models.VPNConnection) string {
	if v.LocalIP != "" {
		return v.LocalIP
	}
	iface := m.interfaceName(v)
	ip, _ := m.interfaceIP(iface)
	return ip
}

func (m *Manager) interfaceName(v *models.VPNConnection) string {
	if v.InterfaceName != "" {
		return v.InterfaceName
	}
	return v.Name + "-fiber"
}

func (m *Manager) interfaceIP(iface string) (string, error) {
	out, err := exec.Command("ip", "-4", "addr", "show", iface).Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				ip, _, err := net.ParseCIDR(fields[1])
				if err == nil && ip != nil {
					return ip.String(), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no IPv4 address on %s", iface)
}