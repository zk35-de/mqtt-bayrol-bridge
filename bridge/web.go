package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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

// ── raw logger (ring buffer, #14/#19) ────────────────────────────────────────

type rawEntry struct {
	At      time.Time `json:"at"`
	Topic   string    `json:"topic"`
	Payload string    `json:"payload"`
}

type rawLogger struct {
	mu      sync.Mutex
	enabled bool
	buf     []rawEntry
	size    int
	head    int
	count   int
}

func newRawLogger(enabled bool, size int) *rawLogger {
	if size <= 0 {
		size = 200
	}
	return &rawLogger{
		enabled: enabled,
		buf:     make([]rawEntry, size),
		size:    size,
	}
}

func (r *rawLogger) isEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled
}

func (r *rawLogger) toggle() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = !r.enabled
	return r.enabled
}

func (r *rawLogger) log(topic string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return
	}
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

// ── cert expiry (#10) ─────────────────────────────────────────────────────────

func certExpiry(certPath string) *time.Time {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		// try DER
		cert, err := x509.ParseCertificate(data)
		if err != nil {
			return nil
		}
		t := cert.NotAfter
		return &t
	}
	cert, err := tls.X509KeyPair(data, data)
	if err != nil {
		// parse just the cert block
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
			"debug_enabled":    b.rawLog.isEnabled(),
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
		b.cfg = incoming
		w.WriteHeader(http.StatusNoContent)
	})

	// #14: raw topic log
	mux.HandleFunc("GET /api/debug/raw", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"enabled": b.rawLog.isEnabled(),
			"entries": b.rawLog.snapshot(),
		})
	})

	mux.HandleFunc("POST /api/debug/toggle", func(w http.ResponseWriter, r *http.Request) {
		enabled := b.rawLog.toggle()
		log.Printf("raw logger: %v", enabled)
		writeJSON(w, map[string]any{"enabled": enabled})
	})

	log.Printf("web ui listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("web server: %v", err)
	}
}
