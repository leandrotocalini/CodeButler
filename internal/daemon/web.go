package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/leandrotocalini/CodeButler/internal/messenger"
)

func (d *Daemon) startWeb() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleDashboard)
	mux.HandleFunc("/api/status", d.handleAPIStatus)
	mux.HandleFunc("/api/logs", d.handleAPILogs)
	mux.HandleFunc("/api/logs/stream", d.handleAPILogsStream)

	port := d.findAvailablePort(3000, 3100)
	if port == 0 {
		d.log.Warn("No available port in range 3000-3100, web UI disabled")
		return
	}

	addr := fmt.Sprintf(":%d", port)
	d.webPort = port

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			d.log.Error("Web server error: %v", err)
		}
	}()
}

func (d *Daemon) findAvailablePort(from, to int) int {
	for port := from; port < to; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 0
}

func (d *Daemon) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	d.msgerMu.Lock()
	connState := d.connState
	d.msgerMu.Unlock()

	status := map[string]interface{}{
		"repo":       filepath.Base(d.repoDir),
		"repoDir":    d.repoDir,
		"group":      d.groupName,
		"messenger":  d.msger.Name(),
		"connection": connState.String(),
		"claudeBusy":    d.isBusy(),
		"conversationActive": d.isConversationActive(),
		"port":       d.webPort,
		"uptime":     time.Since(d.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (d *Daemon) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	entries := d.log.Entries()

	type jsonEntry struct {
		Time    string `json:"time"`
		Level   string `json:"level"`
		Message string `json:"message"`
	}

	out := make([]jsonEntry, len(entries))
	for i, e := range entries {
		out[i] = jsonEntry{
			Time:    e.Time.Format("15:04:05"),
			Level:   e.Level.String(),
			Message: e.Message,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// handleAPILogsStream sends logs as SSE (Server-Sent Events).
func (d *Daemon) handleAPILogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := d.log.Subscribe()
	defer d.log.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-ch:
			data, _ := json.Marshal(map[string]string{
				"time":    entry.Time.Format("15:04:05"),
				"level":   entry.Level.String(),
				"message": entry.Message,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (d *Daemon) handleDashboard(w http.ResponseWriter, r *http.Request) {
	d.msgerMu.Lock()
	connState := d.connState
	d.msgerMu.Unlock()

	repoName := filepath.Base(d.repoDir)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
<title>CodeButler — %s</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "SF Mono", monospace; background: #0d1117; color: #c9d1d9; padding: 20px; }
  h1 { font-size: 1.3em; margin-bottom: 16px; color: #58a6ff; }
  .status { display: flex; gap: 24px; margin-bottom: 20px; flex-wrap: wrap; }
  .badge { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 12px 16px; }
  .badge .label { font-size: 0.75em; color: #8b949e; text-transform: uppercase; margin-bottom: 4px; }
  .badge .value { font-size: 1.1em; }
  .connected { color: #3fb950; }
  .disconnected { color: #f85149; }
  .busy { color: #d29922; }
  .idle { color: #3fb950; }
  .hot { color: #f0883e; }
  #logs { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 16px; height: calc(100vh - 180px); overflow-y: auto; font-size: 0.85em; line-height: 1.6; }
  .log-line { white-space: pre-wrap; word-break: break-all; }
  .log-time { color: #484f58; }
  .log-INFO { color: #c9d1d9; }
  .log-WARN { color: #d29922; }
  .log-ERROR { color: #f85149; }
  .log-DEBUG { color: #8b949e; }
</style>
</head>
<body>
<h1>🤖 CodeButler — %s</h1>
<div class="status">
  <div class="badge"><div class="label">Repo</div><div class="value">%s</div></div>
  <div class="badge"><div class="label">Group</div><div class="value">%s</div></div>
  <div class="badge"><div class="label">Messenger</div><div class="value" id="wa-status" class="%s">%s</div></div>
  <div class="badge"><div class="label">Claude</div><div class="value" id="claude-status">—</div></div>
  <div class="badge"><div class="label">Session</div><div class="value" id="session-status">—</div></div>
</div>
<div id="logs"></div>
<script>
const logsEl = document.getElementById('logs');

// Load existing logs
fetch('/api/logs').then(r => r.json()).then(logs => {
  logs.forEach(addLog);
  logsEl.scrollTop = logsEl.scrollHeight;
});

// Stream new logs
const es = new EventSource('/api/logs/stream');
es.onmessage = (e) => {
  const log = JSON.parse(e.data);
  addLog(log);
  logsEl.scrollTop = logsEl.scrollHeight;
};

// Poll status
setInterval(async () => {
  try {
    const r = await fetch('/api/status');
    const s = await r.json();
    const waEl = document.getElementById('wa-status');
    waEl.textContent = s.messenger + ': ' + s.connection;
    waEl.className = s.connection === 'connected' ? 'connected' : 'disconnected';
    const clEl = document.getElementById('claude-status');
    clEl.textContent = s.claudeBusy ? 'Working...' : 'Idle';
    clEl.className = s.claudeBusy ? 'busy' : 'idle';
    const sessEl = document.getElementById('session-status');
    sessEl.textContent = s.conversationActive ? 'Waiting for reply' : 'Idle';
    sessEl.className = s.conversationActive ? 'hot' : 'idle';
  } catch {}
}, 3000);

function addLog(log) {
  const div = document.createElement('div');
  div.className = 'log-line log-' + log.level;
  div.textContent = log.time + ' ' + log.message;
  logsEl.appendChild(div);
}
</script>
</body>
</html>`, repoName, repoName, repoName, d.repoCfg.WhatsApp.GroupName,
		connStateClass(connState), connState.String())
}

func connStateClass(state messenger.ConnectionState) string {
	if state == messenger.StateConnected {
		return "connected"
	}
	return "disconnected"
}
