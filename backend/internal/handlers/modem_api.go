package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"monitoring/internal/crypto"
	"monitoring/internal/database"
	"monitoring/internal/modem"
)

const modemQueryTimeout = 40 * time.Second

// envOr mengembalikan env dengan fallback.
func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// loadModemAccess mengambil alamat IP dan kredensial modem pelanggan dari DB,
// lalu melengkapi dengan nilai default dari environment.
func loadModemAccess(ctx context.Context, customerID int64) (modem.Access, error) {
	var (
		ip          string
		webUser     string
		webPassEnc  string
		webPort     int
		webHTTPS    bool
		telnetUser  string
		telnetPassE string
		telnetPort  int
	)
	err := database.Pool.QueryRow(ctx, `
		SELECT ip_address,
		       COALESCE(modem_web_user, ''), COALESCE(modem_web_pass_encrypted, ''),
		       modem_web_port, modem_web_https,
		       COALESCE(modem_telnet_user, ''), COALESCE(modem_telnet_pass_encrypted, ''),
		       modem_telnet_port
		FROM customers WHERE id = $1`, customerID).Scan(
		&ip, &webUser, &webPassEnc, &webPort, &webHTTPS,
		&telnetUser, &telnetPassE, &telnetPort)
	if err != nil {
		if err == sql.ErrNoRows {
			return modem.Access{}, fmt.Errorf("pelanggan tidak ditemukan")
		}
		return modem.Access{}, err
	}

	access := modem.Access{
		Host:       strings.TrimSpace(ip),
		HTTPPort:   webPort,
		HTTPS:      webHTTPS,
		WebUser:    webUser,
		TelnetPort: telnetPort,
		TelnetUser: telnetUser,
	}

	// Password modem disimpan terenkripsi; dekripsi bila ada.
	if webPassEnc != "" {
		if p, derr := crypto.Decrypt(webPassEnc); derr == nil {
			access.WebPass = p
		}
	}
	if telnetPassE != "" {
		if p, derr := crypto.Decrypt(telnetPassE); derr == nil {
			access.TelnetPass = p
		}
	}

	// Fallback ke kredensial default server.
	if access.WebUser == "" {
		access.WebUser = envOr("MODEM_WEB_USER", "admin")
	}
	if access.WebPass == "" {
		access.WebPass = envOr("MODEM_WEB_PASS", "admin")
	}
	if access.TelnetUser == "" {
		access.TelnetUser = envOr("MODEM_TELNET_USER", "root")
	}
	if access.TelnetPass == "" {
		access.TelnetPass = envOr("MODEM_TELNET_PASS", "")
	}
	if access.HTTPPort == 0 {
		access.HTTPPort = envIntOr("MODEM_WEB_PORT", 80)
	}
	if access.TelnetPort == 0 {
		access.TelnetPort = envIntOr("MODEM_TELNET_PORT", 23)
	}
	if !access.HTTPS {
		access.HTTPS = strings.EqualFold(envOr("MODEM_WEB_HTTPS", "false"), "true")
	}
	return access, nil
}

func envIntOr(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// openModemDevice menyiapkan Device untuk pelanggan tertentu.
func openModemDevice(c *gin.Context) (*modem.Device, context.Context, context.CancelFunc, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id pelanggan tidak valid"})
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), modemQueryTimeout)

	access, err := loadModemAccess(ctx, id)
	if err != nil {
		cancel()
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return nil, nil, nil, false
	}
	if access.Host == "" {
		cancel()
		c.JSON(http.StatusBadRequest, gin.H{"error": "pelanggan tidak punya alamat IP"})
		return nil, nil, nil, false
	}
	return modem.NewDevice(access), ctx, cancel, true
}

// writeResult menulis hasil fitur atau error ke response.
func writeResult(c *gin.Context, res *modem.Result, err error) {
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "tidak ditemukan") ||
			strings.Contains(err.Error(), "tidak tersedia") ||
			strings.Contains(err.Error(), "tidak dikenali") {
			status = http.StatusNotImplemented
		}
		c.JSON(status, gin.H{
			"section": res.Section,
			"feature": res.Feature,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, res)
}

// ---- Metadata -------------------------------------------------------------

// ModemFeatures mengembalikan katalog endpoint modem.
func ModemFeatures(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": modem.Catalog()})
}

// ModemProbe menguji konektivitas web/telnet ke perangkat pelanggan.
func ModemProbe(c *gin.Context) {
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.Probe(ctx)
	writeResult(c, res, err)
}

// ModemHelp mengembalikan info bantuan/versi perangkat.
func ModemHelp(c *gin.Context) {
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.HelpInfo(ctx)
	writeResult(c, res, err)
}

// ---- Status ---------------------------------------------------------------

func ModemStatusDevice(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.StatusDevice(ctx)
	})
}

func ModemStatusNetworkInfo(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.StatusNetworkInfo(ctx)
	})
}

func ModemStatusUserInfo(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.StatusUserInfo(ctx)
	})
}

func ModemStatusVoice(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.StatusVoice(ctx)
	})
}

func ModemStatusRemote(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.StatusRemoteManagement(ctx)
	})
}

// ---- Network --------------------------------------------------------------

func ModemNetworkWAN(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.NetworkWAN(ctx)
	})
}

// ModemNetworkWANUpdate mengubah satu field koneksi WAN.
func ModemNetworkWANUpdate(c *gin.Context) {
	var body struct {
		Table string `json:"table"`
		Index int    `json:"index"`
		Field string `json:"field" binding:"required"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh field & value"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.UpdateWANField(ctx, body.Table, body.Index, body.Field, body.Value)
	writeResult(c, res, err)
}

func ModemNetworkLAN(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.NetworkLAN(ctx)
	})
}

// ModemNetworkDHCPToggle mengaktifkan/menonaktifkan DHCP server.
func ModemNetworkDHCPToggle(c *gin.Context) {
	var body struct {
		Enable bool `json:"enable"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh enable"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.ToggleDHCP(ctx, body.Enable)
	writeResult(c, res, err)
}

func ModemNetworkWLAN(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.NetworkWLAN(ctx)
	})
}

// ModemNetworkSetSSID mengubah SSID WiFi.
func ModemNetworkSetSSID(c *gin.Context) {
	var body struct {
		Band  string `json:"band"`
		Index int    `json:"index"`
		SSID  string `json:"ssid" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh ssid"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.SetWLANSSID(ctx, strings.ToLower(body.Band), body.Index, body.SSID)
	writeResult(c, res, err)
}

func ModemNetworkRouting(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.NetworkRouting(ctx)
	})
}

func ModemNetworkDNS(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.NetworkDNS(ctx)
	})
}

func ModemNetworkPortBinding(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.NetworkPortBinding(ctx)
	})
}

// ---- Security -------------------------------------------------------------

func ModemSecurityFirewall(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.SecurityFirewall(ctx)
	})
}

func ModemSecurityFirewallToggle(c *gin.Context) {
	var body struct {
		Enable bool `json:"enable"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh enable"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.SetFirewallEnable(ctx, body.Enable)
	writeResult(c, res, err)
}

func ModemSecurityIPFilter(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.SecurityIPFilter(ctx)
	})
}

func ModemSecurityMACFilter(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.SecurityMACFilter(ctx)
	})
}

func ModemSecurityURLFilter(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.SecurityURLFilter(ctx)
	})
}

func ModemSecurityALG(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.SecurityALG(ctx)
	})
}

func ModemSecurityALGToggle(c *gin.Context) {
	var body struct {
		Proto  string `json:"proto" binding:"required"`
		Enable bool   `json:"enable"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh proto & enable"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.SetALGEnable(ctx, strings.ToLower(body.Proto), body.Enable)
	writeResult(c, res, err)
}

// ---- Application ----------------------------------------------------------

func ModemAppUPnP(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationUPnP(ctx)
	})
}

func ModemAppUPnPToggle(c *gin.Context) {
	var body struct {
		Enable bool `json:"enable"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh enable"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.SetUPnPEnable(ctx, body.Enable)
	writeResult(c, res, err)
}

func ModemAppDDNS(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationDDNS(ctx)
	})
}

func ModemAppDMZ(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationDMZ(ctx)
	})
}

func ModemAppPortForwarding(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationPortForwarding(ctx)
	})
}

func ModemAppSNTP(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationSNTP(ctx)
	})
}

func ModemAppMulticast(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationMulticast(ctx)
	})
}

func ModemAppUSB(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationUSB(ctx)
	})
}

func ModemAppVoIP(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ApplicationVoIP(ctx)
	})
}

// ---- Manage ---------------------------------------------------------------

func ModemManageDevice(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ManageDevice(ctx)
	})
}

func ModemManageUsers(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ManageUsers(ctx)
	})
}

func ModemManageUserUpdate(c *gin.Context) {
	var body struct {
		Index    int    `json:"index"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.ManageSetUserPassword(ctx, body.Index, body.User, body.Password)
	writeResult(c, res, err)
}

func ModemManageReboot(c *gin.Context) {
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.ManageReboot(ctx)
	writeResult(c, res, err)
}

func ModemManageFactoryReset(c *gin.Context) {
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.ManageFactoryReset(ctx)
	writeResult(c, res, err)
}

// ModemManageConfigBackup mengunduh config.bin perangkat.
func ModemManageConfigBackup(c *gin.Context) {
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	data, filename, err := d.ManageDownloadConfig(ctx)
	if err != nil {
		writeResult(c, &modem.Result{Section: "manage", Feature: "config_backup"}, err)
		return
	}
	if filename == "" {
		filename = "config.bin"
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/octet-stream", data)
}

// ModemManageConfigRestore memulihkan konfigurasi dari file yang diunggah.
func ModemManageConfigRestore(c *gin.Context) {
	file, err := c.FormFile("config")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "butuh file form 'config'"})
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gagal membuka file"})
		return
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gagal membaca file"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.ManageRestoreConfig(ctx, file.Filename, content)
	writeResult(c, res, err)
}

// ModemManageFirmwareUpgrade mengunggah firmware ke perangkat.
func ModemManageFirmwareUpgrade(c *gin.Context) {
	file, err := c.FormFile("firmware")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "butuh file form 'firmware'"})
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gagal membuka file"})
		return
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gagal membaca file"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.ManageFirmwareUpgrade(ctx, file.Filename, content)
	writeResult(c, res, err)
}

func ModemManageTime(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ManageTime(ctx)
	})
}

func ModemManageSetTime(c *gin.Context) {
	var body struct {
		Value string `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh value (YYYY-MM-DD HH:MM:SS)"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.ManageSetTime(ctx, body.Value)
	writeResult(c, res, err)
}

func ModemManageLog(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.ManageLog(ctx)
	})
}

// ---- Diagnosis ------------------------------------------------------------

func ModemDiagnosisPing(c *gin.Context) {
	var body struct {
		Host  string `json:"host" binding:"required"`
		Count int    `json:"count"`
		Size  int    `json:"size"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh host"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.DiagnosisPing(ctx, body.Host, body.Count, body.Size)
	writeResult(c, res, err)
}

func ModemDiagnosisTraceroute(c *gin.Context) {
	var body struct {
		Host    string `json:"host" binding:"required"`
		MaxHops int    `json:"max_hops"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh host"})
		return
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := d.DiagnosisTraceroute(ctx, body.Host, body.MaxHops)
	writeResult(c, res, err)
}

func ModemDiagnosisARP(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.DiagnosisARPTable(ctx)
	})
}

func ModemDiagnosisMACTable(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.DiagnosisMACTable(ctx)
	})
}

func ModemDiagnosisOptical(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.DiagnosisOptical(ctx)
	})
}

func ModemDiagnosisLoopback(c *gin.Context) {
	runModemFetch(c, func(ctx context.Context, d *modem.Device) (*modem.Result, error) {
		return d.DiagnosisLoopback(ctx)
	})
}

// ModemRaw menjalankan perintah telnet bebas (admin only, untuk debug).
func ModemRaw(c *gin.Context) {
	var body struct {
		Command string `json:"command" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak valid: butuh command"})
		return
	}
	// Batasi perintah destruktif yang mudah tidak sengaja dijalankan.
	blocked := []string{"rm -rf", "mkfs", "dd if=", "reboot", "format"}
	low := strings.ToLower(body.Command)
	for _, b := range blocked {
		if strings.Contains(low, b) {
			c.JSON(http.StatusForbidden, gin.H{"error": "perintah diblokir: " + b})
			return
		}
	}
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	out, err := d.Telnet().Run(ctx, body.Command)
	res := &modem.Result{
		Section: "help", Feature: "raw", Title: "Raw Telnet Command",
		Source: "telnet", Data: out,
	}
	writeResult(c, res, err)
}

// ---- helper ---------------------------------------------------------------

// runModemFetch membungkus handler GET yang hanya membaca satu fitur.
func runModemFetch(c *gin.Context, fetch func(context.Context, *modem.Device) (*modem.Result, error)) {
	d, ctx, cancel, ok := openModemDevice(c)
	if !ok {
		return
	}
	defer cancel()
	defer d.Close()
	res, err := fetch(ctx, d)
	writeResult(c, res, err)
}
