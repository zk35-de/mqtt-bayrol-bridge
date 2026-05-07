package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed ui.html
var uiHTML []byte

// ── value store ──────────────────────────────────────────────────────────────

type valueEntry struct {
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type valueStore struct {
	mu     sync.RWMutex
	values map[string]valueEntry
}

func newValueStore() *valueStore {
	return &valueStore{values: make(map[string]valueEntry)}
}

func (s *valueStore) set(key, val string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = valueEntry{Value: val, UpdatedAt: time.Now()}
}

func (s *valueStore) snapshot() map[string]valueEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]valueEntry, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out
}

// ── connection status ─────────────────────────────────────────────────────────

type connStatus struct {
	mu              sync.RWMutex
	haConnected     bool
	bayrolConnected bool
	startedAt       time.Time
}

func (cs *connStatus) setHA(v bool) {
	cs.mu.Lock()
	cs.haConnected = v
	cs.mu.Unlock()
}

func (cs *connStatus) setBayrol(v bool) {
	cs.mu.Lock()
	cs.bayrolConnected = v
	cs.mu.Unlock()
}

func (cs *connStatus) get() (ha, bayrol bool, uptime time.Duration) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.haConnected, cs.bayrolConnected, time.Since(cs.startedAt)
}

// ── raw logger (always-on ring buffer) ───────────────────────────────────────

type rawEntry struct {
	At      time.Time `json:"at"`
	Topic   string    `json:"topic"`
	Payload string    `json:"payload"`
}

type rawLogger struct {
	mu    sync.Mutex
	buf   []rawEntry
	size  int
	head  int
	count int
}

func newRawLogger(size int) *rawLogger {
	if size <= 0 {
		size = 200
	}
	return &rawLogger{
		buf:  make([]rawEntry, size),
		size: size,
	}
}

func (r *rawLogger) log(topic string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head] = rawEntry{At: time.Now(), Topic: topic, Payload: string(payload)}
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

func (r *rawLogger) snapshot() []rawEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.count
	if n == 0 {
		return nil
	}
	out := make([]rawEntry, n)
	for i := range n {
		idx := (r.head - n + i + r.size) % r.size
		out[i] = r.buf[idx]
	}
	return out
}

// ── file logger ───────────────────────────────────────────────────────────────

type fileLogger struct {
	mu      sync.Mutex
	enabled bool
	path    string
}

func newFileLogger(enabled bool, path string) *fileLogger {
	return &fileLogger{enabled: enabled, path: path}
}

func (f *fileLogger) isEnabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.enabled
}

func (f *fileLogger) toggle() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = !f.enabled
	return f.enabled
}

func (f *fileLogger) log(topic string, payload []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.enabled || f.path == "" {
		return
	}
	fh, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer fh.Close()
	fmt.Fprintf(fh, "%s %s %s\n", time.Now().UTC().Format(time.RFC3339), topic, payload)
}

// ── cert expiry ───────────────────────────────────────────────────────────────

func certExpiry(certPath string) *time.Time {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		cert, err := x509.ParseCertificate(data)
		if err != nil {
			return nil
		}
		t := cert.NotAfter
		return &t
	}
	cert, err := tls.X509KeyPair(data, data)
	if err != nil {
		c, err2 := x509.ParseCertificate(block.Bytes)
		if err2 != nil {
			return nil
		}
		t := c.NotAfter
		return &t
	}
	_ = cert
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	t := c.NotAfter
	return &t
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ── web server ────────────────────────────────────────────────────────────────

func (b *bridge) startWebServer(addr, cfgPath string) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(uiHTML)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ha, bayrol, uptime := b.status.get()
		resp := map[string]any{
			"ok":               ha && bayrol,
			"ha_connected":     ha,
			"bayrol_connected": bayrol,
			"uptime_s":         int(uptime.Seconds()),
			"file_log_enabled": b.fileLog.isEnabled(),
			"version":          version,
		}
		if exp := certExpiry(b.cfg.Mosquitto.CertPath); exp != nil {
			resp["cert_expires"] = exp.UTC().Format(time.RFC3339)
			resp["cert_days_left"] = int(time.Until(*exp).Hours() / 24)
		}
		writeJSON(w, resp)
	})

	mux.HandleFunc("GET /api/values", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, b.store.snapshot())
	})

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := b.cfg
		cfg.HABroker.Password = "***"
		writeJSON(w, cfg)
	})

	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var incoming Config
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if incoming.HABroker.Password == "***" {
			incoming.HABroker.Password = b.cfg.HABroker.Password
		}
		data, err := yaml.Marshal(incoming)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(cfgPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		old := b.cfg
		b.cfg = incoming

		haChanged := incoming.HABroker.Host != old.HABroker.Host ||
			incoming.HABroker.Port != old.HABroker.Port ||
			incoming.HABroker.Username != old.HABroker.Username ||
			incoming.HABroker.Password != old.HABroker.Password

		prefixChanged := incoming.OutputPrefix != old.OutputPrefix

		if haChanged {
			log.Println("config: HA broker changed, reconnecting")
			go b.reconnectHA()
		}
		if prefixChanged {
			log.Printf("config: output prefix changed to %s", incoming.OutputPrefix)
			b.prefix = incoming.OutputPrefix
			if b.getSerial() != "" {
				go b.publishDiscovery()
			}
		}

		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/debug/raw", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"file_log_enabled": b.fileLog.isEnabled(),
			"file_log_path":    b.cfg.Debug.LogPath,
			"entries":          b.rawLog.snapshot(),
		})
	})

	mux.HandleFunc("POST /api/debug/toggle", func(w http.ResponseWriter, r *http.Request) {
		enabled := b.fileLog.toggle()
		log.Printf("file logger: %v", enabled)
		writeJSON(w, map[string]any{"file_log_enabled": enabled})
	})

	log.Printf("web ui listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("web server: %v", err)
	}
}
