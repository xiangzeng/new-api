package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const auditBufferSize = 500
const auditFileName = "error-audit.jsonl"

type AuditEntry struct {
	Timestamp    string `json:"ts"`
	RequestId    string `json:"req_id"`
	ChannelId    int    `json:"ch_id"`
	ChannelName  string `json:"ch_name"`
	StatusCode   int    `json:"status"`
	ModelName    string `json:"model"`
	ErrorCode    string `json:"err_code"`
	ErrorSummary string `json:"err_summary"`
	DbLogged     bool   `json:"db_logged"`
	SkipReason   string `json:"skip_reason,omitempty"`
}

type errorAuditRing struct {
	mu      sync.Mutex
	entries []AuditEntry
	logDir  string
}

var globalAuditRing *errorAuditRing

func InitErrorAudit(logDir string) {
	ring := &errorAuditRing{
		entries: make([]AuditEntry, 0, auditBufferSize),
		logDir:  logDir,
	}
	if logDir != "" {
		ring.loadFromDisk()
	}
	globalAuditRing = ring
}

func RecordErrorAudit(entry AuditEntry) {
	if globalAuditRing == nil {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().Format(time.RFC3339)
	}
	entry.ErrorSummary = truncateRunes(entry.ErrorSummary, 200)

	globalAuditRing.mu.Lock()
	defer globalAuditRing.mu.Unlock()

	if len(globalAuditRing.entries) >= auditBufferSize {
		globalAuditRing.entries = globalAuditRing.entries[1:]
	}
	globalAuditRing.entries = append(globalAuditRing.entries, entry)
	globalAuditRing.flushToDisk()
}

func (r *errorAuditRing) flushToDisk() {
	if r.logDir == "" {
		return
	}
	target := filepath.Join(r.logDir, auditFileName)
	tmp := target + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for i := range r.entries {
		_ = enc.Encode(&r.entries[i])
	}
	_ = w.Flush()
	_ = f.Close()
	_ = os.Rename(tmp, target)
}

func (r *errorAuditRing) loadFromDisk() {
	target := filepath.Join(r.logDir, auditFileName)
	f, err := os.Open(target)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry AuditEntry
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			r.entries = append(r.entries, entry)
		}
	}
	if len(r.entries) > auditBufferSize {
		r.entries = r.entries[len(r.entries)-auditBufferSize:]
	}
}

func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
