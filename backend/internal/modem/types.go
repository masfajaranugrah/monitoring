// Package modem berisi client untuk mengakses perangkat GPON ONT/ONU ZTE
// (diuji untuk keluarga ZXHN F663NV9 / firmware V9) melalui dua jalur:
//
//  1. Web UI  — sesi HTTP ke CGI web management (login token + cookie sesi).
//  2. Telnet  — shell perangkat untuk perintah `sendcmd 1 DB ...` yang
//     memberi akses langsung ke database konfigurasi (lebih lengkap & stabil).
//
// Feature-level method di package ini memetakan seluruh menu Web UI perangkat
// (Status, Network, Security, Application, Manage, Diagnosis, Help) menjadi
// data terstruktur yang bisa dikonsumsi HTTP API.
package modem

import (
	"errors"
	"fmt"
)

// Errors umum yang bisa dicek pemanggil dengan errors.Is.
var (
	ErrNotLoggedIn   = errors.New("modem: belum login")
	ErrLoginFailed   = errors.New("modem: login gagal")
	ErrWebDisabled   = errors.New("modem: akses web tidak tersedia")
	ErrTelnetDisable = errors.New("modem: akses telnet tidak tersedia")
	ErrNotFound      = errors.New("modem: data tidak ditemukan")
	ErrUnsupported   = errors.New("modem: fitur tidak didukung firmware")
)

// Access mendeskripsikan cara menjangkau sebuah perangkat.
type Access struct {
	Host       string `json:"host"`
	HTTPPort   int    `json:"http_port,omitempty"`
	HTTPS      bool   `json:"https,omitempty"`
	WebUser    string `json:"web_user,omitempty"`
	WebPass    string `json:"web_pass,omitempty"`
	TelnetPort int    `json:"telnet_port,omitempty"`
	TelnetUser string `json:"telnet_user,omitempty"`
	TelnetPass string `json:"telnet_pass,omitempty"`
}

// Scheme mengembalikan skema web yang dipakai.
func (a Access) Scheme() string {
	if a.HTTPS {
		return "https"
	}
	return "http"
}

// BaseURL menyusun URL dasar web management.
func (a Access) BaseURL() string {
	port := a.HTTPPort
	if port <= 0 {
		if a.HTTPS {
			port = 443
		} else {
			port = 80
		}
	}
	return fmt.Sprintf("%s://%s:%d", a.Scheme(), a.Host, port)
}

// Result adalah amplop seragam untuk semua keluaran fitur.
type Result struct {
	Section string      `json:"section"`
	Feature string      `json:"feature"`
	Title   string      `json:"title,omitempty"`
	Source  string      `json:"source"` // web | telnet | combined
	Data    interface{} `json:"data,omitempty"`
	Raw     string      `json:"raw,omitempty"`
	Warning string      `json:"warning,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// KeyValue dipakai untuk tabel generik hasil parsing `<DM name= val=>`.
type KeyValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Row adalah satu baris database hasil `sendcmd 1 DB get`.
type Row map[string]string

// Get mengambil nilai kolom, kosong bila tak ada.
func (r Row) Get(key string) string { return r[key] }

// TableResult adalah satu tabel database ZTE yang sudah diparse.
type TableResult struct {
	Table string `json:"table"`
	Rows  []Row  `json:"rows"`
}

// RawCommandResult adalah keluaran mentah sebuah perintah telnet.
type RawCommandResult struct {
	Command string `json:"command"`
	Output  string `json:"output"`
}
