package modem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ---- Manage: Device -------------------------------------------------------

// ManageDevice mengembalikan Device Management (setelan manajemen perangkat).
func (d *Device) ManageDevice(ctx context.Context) (*Result, error) {
	res := &Result{Section: "manage", Feature: "device", Title: "Device Management"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"device":       {"DeviceInfo", "Device", "DeviceCfg"},
		"telnet":       {"TelnetCfg", "Telnet", "TelnetConfig"},
		"web":          {"WebCfg", "WebUser", "WebConfig"},
		"login_timeout": {"LoginTimeout", "SessionCfg", "LoginCfg"},
		"https":        {"HTTPS", "HTTPSCfg", "SSL"},
		"ipv6_switch":  {"IPv6Switch", "IPv6Cfg"},
		"loopback":     {"LoopbackDetect", "LoopbackDetection"},
	})
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: device management tidak tersedia", ErrUnsupported), d, "device management")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ---- Manage: Users --------------------------------------------------------

// ManageUsers mengembalikan daftar user web/telnet perangkat (DevAuthInfo).
func (d *Device) ManageUsers(ctx context.Context) (*Result, error) {
	res := &Result{Section: "manage", Feature: "users", Title: "User Management"}
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	tbl, _, err := d.FirstTable(ctx, "DevAuthInfo", "WebAuthInfo", "AuthInfo", "UserInfo")
	if err != nil {
		return nil, describeErr(fmt.Errorf("%w: user management tidak tersedia", ErrUnsupported), d, "user management")
	}
	// Sembunyikan password pada keluaran (jangan bocorkan hash/plaintext).
	safe := make([]map[string]string, 0, len(tbl.Rows))
	for _, r := range tbl.Rows {
		m := map[string]string{}
		for k, v := range r {
			if strings.Contains(strings.ToLower(k), "pass") {
				m[k] = maskSecret(v)
				continue
			}
			m[k] = v
		}
		safe = append(safe, m)
	}
	res.Source = "telnet"
	res.Data = map[string]interface{}{
		"table": tbl.Table,
		"users": safe,
	}
	return res, nil
}

// ManageSetUserPassword mengubah password/akun web user pada index tertentu.
func (d *Device) ManageSetUserPassword(ctx context.Context, index int, user, password string) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	tbl, name, err := d.FirstTable(ctx, "DevAuthInfo", "WebAuthInfo", "AuthInfo")
	if err != nil {
		return nil, fmt.Errorf("%w: tabel user tidak ditemukan: %v", ErrNotFound, err)
	}
	out := map[string]interface{}{}
	if user != "" {
		field := pickField(tbl.Rows[0], "User", "Username", "UserName", "WebUser")
		if field == "" {
			return nil, fmt.Errorf("%w: field username tidak dikenali", ErrUnsupported)
		}
		r, err := d.telnet.DBSet(ctx, name, index, field, user)
		if err != nil {
			return nil, err
		}
		out["user"] = r
	}
	if password != "" {
		field := pickField(tbl.Rows[0], "Pass", "Password", "WebPass", "Passwd")
		if field == "" {
			return nil, fmt.Errorf("%w: field password tidak dikenali", ErrUnsupported)
		}
		r, err := d.telnet.DBSet(ctx, name, index, field, password)
		if err != nil {
			return nil, err
		}
		out["pass"] = r
	}
	save, _ := d.telnet.DBSave(ctx)
	out["save"] = save
	return &Result{
		Section: "manage", Feature: "users_update", Title: "Update User",
		Source: "telnet", Data: out,
	}, nil
}

// ---- Manage: Reboot & Factory Reset ---------------------------------------

// ManageReboot me-reboot perangkat (telnet `reboot`, fallback web).
func (d *Device) ManageReboot(ctx context.Context) (*Result, error) {
	data := map[string]interface{}{}
	if err := d.EnsureTelnet(ctx); err == nil {
		save, _ := d.telnet.DBSave(ctx)
		data["save"] = save
		out, err := d.telnet.Exec(ctx, "reboot")
		data["reboot"] = RawCommandResult{Command: "reboot", Output: out}
		if err == nil {
			return &Result{Section: "manage", Feature: "reboot", Title: "Reboot", Source: "telnet", Data: data, Warning: errString(err)}, nil
		}
	}
	// Fallback via web.
	if err := d.EnsureWeb(ctx); err != nil {
		return nil, err
	}
	form := d.withSession(ctx, url.Values{})
	form.Set("IF_ACTION", "devrestart")
	form.Set("IF_ERRORSTR", "SUCC")
	form.Set("IF_ERRORPARAM", "SUCC")
	form.Set("IF_ERRORTYPE", "-1")
	form.Set("flag", "1")
	body, status, err := d.web.PostForm(ctx, "/getpage.gch?pid=1002&nextpage=manager_dev_conf_t.gch", form)
	data["web_status"] = status
	data["web_body"] = string(body)
	if err != nil {
		return nil, err
	}
	return &Result{Section: "manage", Feature: "reboot", Title: "Reboot", Source: "web", Data: data}, nil
}

// ManageFactoryReset mengembalikan perangkat ke setelan pabrik.
func (d *Device) ManageFactoryReset(ctx context.Context) (*Result, error) {
	data := map[string]interface{}{}
	if err := d.EnsureTelnet(ctx); err == nil {
		out, err := d.telnet.Exec(ctx, "sendcmd 1 DB def")
		data["reset"] = RawCommandResult{Command: "sendcmd 1 DB def", Output: out}
		if err == nil {
			reboot, _ := d.telnet.Exec(ctx, "reboot")
			data["reboot"] = RawCommandResult{Command: "reboot", Output: reboot}
			return &Result{Section: "manage", Feature: "factory_reset", Title: "Factory Reset", Source: "telnet", Data: data}, nil
		}
	}
	if err := d.EnsureWeb(ctx); err != nil {
		return nil, err
	}
	form := d.withSession(ctx, url.Values{})
	form.Set("IF_ACTION", "devrestore")
	body, status, err := d.web.PostForm(ctx, "/getpage.gch?pid=1002&nextpage=manager_dev_conf_t.gch", form)
	data["web_status"] = status
	data["web_body"] = string(body)
	if err != nil {
		return nil, err
	}
	return &Result{Section: "manage", Feature: "factory_reset", Title: "Factory Reset", Source: "web", Data: data}, nil
}

// ---- Manage: Backup / Restore ---------------------------------------------

// ManageDownloadConfig mengunduh berkas konfigurasi perangkat (config.bin).
func (d *Device) ManageDownloadConfig(ctx context.Context) ([]byte, string, error) {
	if err := d.EnsureWeb(ctx); err != nil {
		return nil, "", err
	}
	// Varian 1: multipart POST klasik ke manager_dev_conf_t.gch.
	endpoints := []string{
		"/getpage.gch?pid=1002&nextpage=manager_dev_conf_t.gch",
		"/cgi-bin/web_cgi?page=config_download",
		"/cgi-bin/cfgfile",
		"/manager_dev_config_t.gch",
	}
	var lastErr error
	for _, ep := range endpoints {
		data, filename, err := d.postConfigBackup(ctx, ep)
		if err == nil && len(data) > 0 {
			return data, filename, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	// Varian 3: ambil XML config langsung via telnet/TFTP-less path.
	if err := d.EnsureTelnet(ctx); err == nil {
		for _, p := range []string{"/userconfig/cfg/db_user_cfg.xml", "/userconfig/cfg/db_backup_cfg.xml"} {
			if out, err := d.telnet.Exec(ctx, "cat "+p); err == nil && len(strings.TrimSpace(out)) > 0 {
				return []byte(out), baseName(p), nil
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: backup config tidak tersedia", ErrUnsupported)
	}
	return nil, "", describeErr(lastErr, d, "backup config")
}

func (d *Device) postConfigBackup(ctx context.Context, path string) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("config", "config.bin")
	if err != nil {
		return nil, "", err
	}
	_, _ = fw.Write([]byte{})
	_ = mw.Close()

	u := path
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.web.base+u, &buf)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Cookie", "_TESTCOOKIESUPPORT=1")
	req.Header.Set("Referer", d.web.base+"/")
	resp, err := d.web.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	filename := "config.bin"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if name := filenameFromDisposition(cd); name != "" {
			filename = name
		}
	}
	if resp.StatusCode != http.StatusOK || len(data) == 0 {
		return data, filename, fmt.Errorf("endpoint %s mengembalikan %d", path, resp.StatusCode)
	}
	return data, filename, nil
}

// ManageRestoreConfig memulihkan konfigurasi dari berkas yang diunggah.
func (d *Device) ManageRestoreConfig(ctx context.Context, filename string, content []byte) (*Result, error) {
	if err := d.EnsureWeb(ctx); err != nil {
		return nil, err
	}
	endpoints := []string{
		"/getpage.gch?pid=1002&nextpage=manager_dev_conf_t.gch",
		"/cgi-bin/web_cgi?page=config_upload",
		"/cgi-bin/upload",
	}
	fieldNames := []string{"config", "filename", "file"}
	var lastErr error
	var lastStatus int
	var lastBody []byte
	for _, ep := range endpoints {
		for _, fn := range fieldNames {
			status, body, err := d.postMultipart(ctx, ep, fn, filename, content)
			lastStatus, lastBody = status, body
			if err == nil && status == http.StatusOK {
				return &Result{
					Section: "manage", Feature: "config_restore", Title: "Restore Config",
					Source: "web",
					Data: map[string]interface{}{
						"endpoint": ep, "field": fn, "status": status, "response": string(body),
					},
				}, nil
			}
			if err != nil {
				lastErr = err
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: restore config tidak tersedia", ErrUnsupported)
	}
	return &Result{
		Section: "manage", Feature: "config_restore", Title: "Restore Config",
		Source: "web",
		Data:   map[string]interface{}{"status": lastStatus, "response": string(lastBody)},
		Error:  lastErr.Error(),
	}, nil
}

func (d *Device) postMultipart(ctx context.Context, path, field, filename string, content []byte) (int, []byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		return 0, nil, err
	}
	if _, err := fw.Write(content); err != nil {
		return 0, nil, err
	}
	_ = mw.Close()

	u := path
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.web.base+u, &buf)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Cookie", "_TESTCOOKIESUPPORT=1")
	req.Header.Set("Referer", d.web.base+"/")
	resp, err := d.web.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	return resp.StatusCode, body, nil
}

// ManageFirmwareUpgrade mengunggah firmware ke perangkat.
func (d *Device) ManageFirmwareUpgrade(ctx context.Context, filename string, content []byte) (*Result, error) {
	if err := d.EnsureWeb(ctx); err != nil {
		return nil, err
	}
	endpoints := []string{
		"/cgi-bin/web_cgi?page=upgrade",
		"/getpage.gch?pid=1002&nextpage=manager_dev_upgrade_t.gch",
		"/cgi-bin/upgrade",
	}
	var lastErr error
	var lastStatus int
	for _, ep := range endpoints {
		status, body, err := d.postMultipart(ctx, ep, "filename", filename, content)
		if err == nil && status == http.StatusOK {
			return &Result{
				Section: "manage", Feature: "firmware_upgrade", Title: "Firmware Upgrade",
				Source: "web",
				Data: map[string]interface{}{
					"endpoint": ep, "status": status, "response": string(body),
				},
			}, nil
		}
		lastStatus = status
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: firmware upgrade tidak tersedia", ErrUnsupported)
	}
	return &Result{
		Section: "manage", Feature: "firmware_upgrade", Title: "Firmware Upgrade",
		Source: "web", Data: map[string]interface{}{"status": lastStatus}, Error: lastErr.Error(),
	}, nil
}

// ---- Manage: Time ---------------------------------------------------------

// ManageTime mengembalikan waktu perangkat & konfigurasi zona waktu.
func (d *Device) ManageTime(ctx context.Context) (*Result, error) {
	res := &Result{Section: "manage", Feature: "time", Title: "Time Settings"}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"timezone": {"TimeZone", "Timezone", "TZ"},
		"time":     {"Time", "DeviceTime", "SysTime"},
		"sntp":     {"SNTP", "SNTPCfg", "NTP"},
	})
	if out, err := d.telnet.Exec(ctx, "date"); err == nil && strings.TrimSpace(out) != "" {
		data["device_date"] = strings.TrimSpace(out)
		ok = true
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: time settings tidak tersedia", ErrUnsupported), d, "time settings")
	}
	res.Source = "telnet"
	res.Data = data
	return res, nil
}

// ManageSetTime mengatur waktu perangkat (format "YYYY-MM-DD HH:MM:SS").
func (d *Device) ManageSetTime(ctx context.Context, value string) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	out, err := d.telnet.Exec(ctx, "date -s "+shellQuote(value))
	if err != nil {
		return nil, err
	}
	return &Result{
		Section: "manage", Feature: "time_update", Title: "Set Time",
		Source: "telnet",
		Data:   map[string]interface{}{"command": out, "value": value, "requested_at": time.Now()},
	}, nil
}

// ---- Manage: Log ----------------------------------------------------------

// ManageLog mengembalikan log sistem perangkat.
func (d *Device) ManageLog(ctx context.Context) (*Result, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, err
	}
	data := map[string]interface{}{}
	ok := d.listTables(ctx, data, map[string][]string{
		"log_cfg": {"LogCfg", "SysLog", "Log", "LogConfig"},
	})
	for cmd, key := range map[string]string{
		"dmesg":          "dmesg",
		"logread":        "logread",
		"cat /var/log/messages": "messages",
		"cat /tmp/log":   "tmp_log",
	} {
		if out, err := d.telnet.Exec(ctx, cmd); err == nil && strings.TrimSpace(out) != "" {
			data[key] = strings.TrimSpace(out)
			ok = true
		}
	}
	if !ok {
		return nil, describeErr(fmt.Errorf("%w: log tidak tersedia", ErrUnsupported), d, "log")
	}
	res := &Result{Section: "manage", Feature: "log", Title: "Log Management", Source: "telnet", Data: data}
	return res, nil
}

// ---- helpers --------------------------------------------------------------

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 2 {
		return "**"
	}
	return s[:1] + strings.Repeat("*", len(s)-2) + s[len(s)-1:]
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func filenameFromDisposition(cd string) string {
	const key = `filename=`
	i := strings.Index(strings.ToLower(cd), key)
	if i < 0 {
		return ""
	}
	v := strings.TrimSpace(cd[i+len(key):])
	v = strings.Trim(v, `"'`)
	if j := strings.IndexByte(v, ';'); j >= 0 {
		v = v[:j]
	}
	return strings.TrimSpace(v)
}
