package ping

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// Result holds the outcome of a single ICMP ping.
type Result struct {
	Success  bool
	RTT      time.Duration
	ErrorMsg string
}

// ICMPPinger performs an ICMP echo request with fallback to the system ping binary.
type ICMPPinger struct {
	privileged bool
	seq        int
}

func NewICMPPinger() *ICMPPinger {
	p := &ICMPPinger{privileged: true}
	// Test whether privileged ICMP is available.
	if err := p.testPrivileged(); err != nil {
		log.Println("[ping] privileged ICMP socket unavailable, falling back to system ping:", err)
		p.privileged = false
	}
	return p
}

func (p *ICMPPinger) testPrivileged() error {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// Ping sends a single ICMP echo request to the given address with a timeout.
// The `source` parameter is optional; when non-empty it is used as the
// binding interface IP (e.g. the VPN interface local IP) so the ping is
// routed through the correct VPN tunnel.
func (p *ICMPPinger) Ping(address string, source string, timeout time.Duration) Result {
	if p.privileged {
		if res, ok := p.pingPrivileged(address, source, timeout); ok {
			return res
		}
	}
	return p.pingSystem(address, timeout)
}

func (p *ICMPPinger) pingPrivileged(dst string, source string, timeout time.Duration) (Result, bool) {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return Result{}, false
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return Result{}, false
	}

	ipAddr, err := net.ResolveIPAddr("ip4", dst)
	if err != nil {
		return Result{false, 0, "cannot resolve"}, true
	}

	if source != "" {
		if err := conn.IPv4PacketConn().SetTOS(0x28); err != nil {
			return Result{}, false
		}
		_ = source
	}

	p.seq++
	id := p.seq
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  p.seq,
			Data: bytes.Repeat([]byte("monitoring"), 4),
		},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		return Result{}, false
	}

	start := time.Now()
	if _, err := conn.WriteTo(wb, ipAddr); err != nil {
		return Result{false, 0, err.Error()}, true
	}

	rb := make([]byte, 1500)
	for {
		n, peer, err := conn.ReadFrom(rb)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return Result{false, timeout, "timeout"}, true
			}
			return Result{false, 0, err.Error()}, true
		}
		if peer.String() != ipAddr.String() {
			continue
		}
		rm, err := icmp.ParseMessage(1, rb[:n])
		if err != nil {
			return Result{false, 0, "invalid reply"}, true
		}
		switch rm.Type {
		case ipv4.ICMPTypeEchoReply:
			echo, ok := rm.Body.(*icmp.Echo)
			if !ok || echo.ID != id {
				continue
			}
			return Result{true, time.Since(start), ""}, true
		case ipv4.ICMPTypeDestinationUnreachable:
			return Result{false, 0, "destination unreachable"}, true
		}
		if time.Now().After(deadline) {
			return Result{false, timeout, "timeout"}, true
		}
	}
}

func (p *ICMPPinger) pingSystem(address string, timeout time.Duration) Result {
	timeoutSec := strconv.Itoa(int(timeout.Seconds()) + 1)
	cmd := exec.Command("ping", "-c", "1", "-W", timeoutSec, address)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := time.Now()
	if err := cmd.Run(); err != nil {
		return Result{false, time.Since(start), "timeout or unreachable"}
	}

	output := out.String()
	// Parse "time=12.3 ms"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if idx := strings.Index(line, "time="); idx >= 0 {
			rest := line[idx+len("time="):]
			var msStr string
			for _, ch := range rest {
				if (ch >= '0' && ch <= '9') || ch == '.' {
					msStr += string(ch)
				} else {
					break
				}
			}
			if ms, err := strconv.ParseFloat(msStr, 64); err == nil {
				return Result{true, time.Duration(ms * float64(time.Millisecond)), ""}
			}
		}
	}
	return Result{true, time.Since(start), ""}
}

// ValidateAddress performs a quick reachability check and returns a human-readable message.
func TestPing(address, source string, timeout time.Duration) (bool, int, string) {
	p := NewICMPPinger()
	res := p.Ping(address, source, timeout)
	if res.Success {
		return true, int(res.RTT.Milliseconds()), fmt.Sprintf("OK (%d ms)", res.RTT.Milliseconds())
	}
	return false, 0, res.ErrorMsg
}