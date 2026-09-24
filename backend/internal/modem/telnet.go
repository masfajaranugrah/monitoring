package modem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Konstanta protokol Telnet (RFC 854).
const (
	telnetIAC  = 255
	telnetDONT = 254
	telnetDO   = 253
	telnetWONT = 252
	telnetWILL = 251
	telnetSB   = 250
	telnetSE   = 240
)

var (
	loginPromptRe    = regexp.MustCompile(`(?i)(login|username|user name)\s*[:：]\s*$`)
	passwordPromptRe = regexp.MustCompile(`(?i)(password|passwd)\s*[:：]\s*$`)
	promptRe         = regexp.MustCompile(`(?m)[\r\n][^\r\n]{0,64}[>#$%]\s?$`)
	telnetCmdRe      = regexp.MustCompile(`\x1b?\[[0-9;?]*[a-zA-Z]`)
)

// TelnetClient adalah sesi telnet minimal ke shell perangkat ZTE.
type TelnetClient struct {
	access Access
	conn   net.Conn
	mu     sync.Mutex
	closed bool
}

// NewTelnetClient membuat sesi telnet (belum terhubung).
func NewTelnetClient(a Access) *TelnetClient {
	return &TelnetClient{access: a}
}

// Connect membuka koneksi TCP ke port telnet perangkat.
func (t *TelnetClient) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		return nil
	}
	port := t.access.TelnetPort
	if port <= 0 {
		port = 23
	}
	addr := net.JoinHostPort(t.access.Host, fmt.Sprintf("%d", port))
	d := &net.Dialer{Timeout: defaultTelnetTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTelnetDisable, err)
	}
	t.conn = conn
	return nil
}

// Close menutup sesi telnet.
func (t *TelnetClient) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return nil
	}
	err := t.conn.Close()
	t.conn = nil
	t.closed = true
	return err
}

// Login melakukan autentikasi shell telnet.
func (t *TelnetClient) Login(ctx context.Context) error {
	if err := t.Connect(ctx); err != nil {
		return err
	}
	if _, err := t.readUntil(ctx, loginPromptRe, 6*time.Second); err != nil {
		// Sebagian firmware langsung memberi prompt shell (tanpa login).
		if _, err2 := t.readUntil(ctx, promptRe, 1500*time.Millisecond); err2 == nil {
			return nil
		}
		return fmt.Errorf("%w: prompt login tidak muncul", ErrLoginFailed)
	}
	if _, err := t.writeLine(t.access.TelnetUser); err != nil {
		return err
	}
	if _, err := t.readUntil(ctx, passwordPromptRe, 6*time.Second); err != nil {
		return fmt.Errorf("%w: prompt password tidak muncul", ErrLoginFailed)
	}
	if _, err := t.writeLine(t.access.TelnetPass); err != nil {
		return err
	}
	// Tunggu prompt shell setelah login.
	if _, err := t.readUntil(ctx, promptRe, 6*time.Second); err != nil {
		// Beberapa shell tidak memberi prompt jelas; anggap berhasil bila
		// tidak ada lagi prompt login.
		out, _ := t.readUntilIdle(ctx, 1200*time.Millisecond)
		if loginPromptRe.MatchString(strings.TrimSpace(out)) {
			return fmt.Errorf("%w: kredensial ditolak", ErrLoginFailed)
		}
	}
	return nil
}

// Exec menjalankan satu perintah shell dan mengembalikan keluarannya.
func (t *TelnetClient) Exec(ctx context.Context, cmd string) (string, error) {
	if err := t.Login(ctx); err != nil {
		return "", err
	}
	if _, err := t.writeLine(cmd); err != nil {
		return "", err
	}
	out, err := t.readUntilIdle(ctx, 1500*time.Millisecond)
	if err != nil {
		return out, err
	}
	return cleanTelnetOutput(out, cmd), nil
}

// writeLine mengirim satu baris perintah.
func (t *TelnetClient) writeLine(line string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return "", ErrTelnetDisable
	}
	_ = t.conn.SetWriteDeadline(time.Now().Add(defaultTelnetTimeout))
	_, err := t.conn.Write([]byte(line + "\r\n"))
	return line, err
}

// readUntilIdle membaca sampai tidak ada data selama idle.
func (t *TelnetClient) readUntilIdle(ctx context.Context, idle time.Duration) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return "", ErrTelnetDisable
	}
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	lastData := time.Now()
	for {
		select {
		case <-ctx.Done():
			return buf.String(), ctx.Err()
		default:
		}
		_ = t.conn.SetReadDeadline(time.Now().Add(idle))
		n, err := t.conn.Read(tmp)
		if n > 0 {
			lastData = time.Now()
			filtered := t.negotiate(tmp[:n])
			buf.Write(filtered)
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if time.Since(lastData) >= idle && buf.Len() > 0 {
					return buf.String(), nil
				}
				if buf.Len() > 0 {
					return buf.String(), nil
				}
				// Belum ada data sama sekali; tunggu sekali lagi.
				continue
			}
			if err == io.EOF {
				return buf.String(), nil
			}
			return buf.String(), err
		}
		if buf.Len() > 0 && time.Since(lastData) >= idle {
			return buf.String(), nil
		}
	}
}

// readUntil membaca sampai regex terpenuhi atau timeout.
func (t *TelnetClient) readUntil(ctx context.Context, re *regexp.Regexp, timeout time.Duration) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return "", ErrTelnetDisable
	}
	deadline := time.Now().Add(timeout)
	var buf bytes.Buffer
	tmp := make([]byte, 1024)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return buf.String(), ctx.Err()
		default:
		}
		_ = t.conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		n, err := t.conn.Read(tmp)
		if n > 0 {
			buf.Write(t.negotiate(tmp[:n]))
			if re.Match(buf.Bytes()) {
				return buf.String(), nil
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if err == io.EOF {
				return buf.String(), nil
			}
			return buf.String(), err
		}
	}
	return buf.String(), fmt.Errorf("timeout menunggu prompt")
}

// negotiate merespons negosiasi opsi telnet dan membuang byte kontrol dari output.
func (t *TelnetClient) negotiate(data []byte) []byte {
	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		b := data[i]
		if b != telnetIAC {
			out = append(out, b)
			i++
			continue
		}
		if i+1 >= len(data) {
			break
		}
		cmd := data[i+1]
		switch cmd {
		case telnetWILL, telnetWONT, telnetDO, telnetDONT:
			if i+2 >= len(data) {
				i = len(data)
				break
			}
			opt := data[i+2]
			var reply []byte
			switch cmd {
			case telnetDO:
				reply = []byte{telnetIAC, telnetWONT, opt}
			case telnetWILL:
				reply = []byte{telnetIAC, telnetDONT, opt}
			}
			if len(reply) > 0 && t.conn != nil {
				_ = t.conn.SetWriteDeadline(time.Now().Add(time.Second))
				_, _ = t.conn.Write(reply)
			}
			i += 3
		case telnetSB:
			// Lewati subnegosiasi sampai IAC SE.
			j := i + 2
			for j < len(data)-1 {
				if data[j] == telnetIAC && data[j+1] == telnetSE {
					break
				}
				j++
			}
			i = j + 2
		default:
			i += 2
		}
	}
	return out
}

// cleanTelnetOutput membersihkan echo perintah, escape ANSI, dan prompt.
func cleanTelnetOutput(out, cmd string) string {
	out = telnetCmdRe.ReplaceAllString(out, "")
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		if trimmed == strings.TrimSpace(cmd) {
			continue
		}
		if strings.HasPrefix(trimmed, strings.TrimSpace(cmd)+" ") && strings.Contains(trimmed, "sendcmd") {
			continue
		}
		if promptRe.MatchString("\n" + ln) {
			// Buang baris yang hanya berisi prompt.
			stripped := strings.TrimRight(trimmed, ">#$% ")
			if stripped == "" || stripped == strings.TrimSpace(cmd) {
				continue
			}
		}
		cleaned = append(cleaned, trimmed)
	}
	return strings.Join(cleaned, "\n")
}
