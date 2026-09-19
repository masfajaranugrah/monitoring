package modem

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Device menggabungkan akses web dan telnet ke satu perangkat dan menyediakan
// method tingkat fitur (Status, Network, Security, Application, Manage,
// Diagnosis, Help).
type Device struct {
	access Access

	web    *WebClient
	telnet *TelnetClient

	mu        sync.Mutex
	webErr    error
	telnetErr error
	webOnce   sync.Once
	telErr    sync.Once
}

// NewDevice membuat handler perangkat dengan akses yang diberikan.
func NewDevice(a Access) *Device {
	if a.TelnetUser == "" {
		a.TelnetUser = "root"
	}
	return &Device{
		access: a,
		web:    NewWebClient(a),
		telnet: NewTelnetClient(a),
	}
}

// Access mengembalikan konfigurasi akses.
func (d *Device) Access() Access { return d.access }

// Web mengembalikan web client.
func (d *Device) Web() *WebClient { return d.web }

// Telnet mengembalikan telnet client.
func (d *Device) Telnet() *TelnetClient { return d.telnet }

// EnsureWeb memastikan sesi web sudah login (hanya sekali per Device).
func (d *Device) EnsureWeb(ctx context.Context) error {
	d.webOnce.Do(func() {
		d.webErr = d.web.Login(ctx)
	})
	return d.webErr
}

// EnsureTelnet memastikan sesi telnet sudah login.
func (d *Device) EnsureTelnet(ctx context.Context) error {
	d.telErr.Do(func() {
		if err := d.telnet.Connect(ctx); err != nil {
			d.telnetErr = err
			return
		}
		if d.access.TelnetUser == "" && d.access.TelnetPass == "" {
			// Tanpa kredensial kita tetap bisa mencoba shell langsung.
			d.telnetErr = nil
			return
		}
		d.telnetErr = d.telnet.Login(ctx)
	})
	return d.telnetErr
}

// Close menutup semua sesi.
func (d *Device) Close() {
	_ = d.telnet.Close()
}

// FirstTable mengembalikan tabel DB pertama yang berhasil dibaca dari daftar
// kandidat nama tabel. Berguna karena penamaan tabel berbeda antar firmware.
func (d *Device) FirstTable(ctx context.Context, names ...string) (*TableResult, string, error) {
	if err := d.EnsureTelnet(ctx); err != nil {
		return nil, "", err
	}
	var lastErr error
	for _, name := range names {
		tbl, err := d.telnet.DBGetTable(ctx, name)
		if err == nil && tbl != nil && len(tbl.Rows) > 0 {
			return tbl, name, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: tidak ada tabel yang cocok dari %s", ErrNotFound, strings.Join(names, ", "))
	}
	return nil, "", lastErr
}

// FirstRow adalah varian FirstTable yang mengembalikan baris pertama.
func (d *Device) FirstRow(ctx context.Context, names ...string) (Row, string, error) {
	tbl, name, err := d.FirstTable(ctx, names...)
	if err != nil {
		return nil, name, err
	}
	return tbl.Rows[0], name, nil
}

// RowToStringMap mengubah Row menjadi map[string]string biasa.
func RowToStringMap(r Row) map[string]string {
	m := make(map[string]string, len(r))
	for k, v := range r {
		m[k] = v
	}
	return m
}
