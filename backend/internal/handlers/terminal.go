package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
}

const (
	terminalInitialCols = 120
	terminalInitialRows = 32
)

// TerminalWS upgrades the request to a WebSocket and attaches it to an
// interactive shell (PTY) running on the server. Client messages are forwarded
// to the PTY; a JSON message of the form {"type":"resize","cols":N,"rows":N}
// resizes the PTY so the shell keeps its layout when the browser resizes.
func TerminalWS(c *gin.Context) {
	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell, "-l")
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: terminalInitialRows, Cols: terminalInitialCols})
	if err != nil {
		_ = conn.WriteControl(websocket.CloseInternalServerErr, []byte(err.Error()), time.Now().Add(time.Second))
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
	}()

	// PTY -> client.
	go func() {
		buf := make([]byte, 8192)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	// client -> PTY.
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if mt == websocket.TextMessage && len(data) > 0 && data[0] == '{' {
			var rc struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(data, &rc) == nil && rc.Type == "resize" && rc.Cols > 0 && rc.Rows > 0 {
				_ = pty.Setsize(ptmx, &pty.Winsize{Rows: rc.Rows, Cols: rc.Cols})
			}
			continue
		}
		if mt == websocket.TextMessage || mt == websocket.BinaryMessage {
			_, _ = ptmx.Write(data)
		}
	}
}
