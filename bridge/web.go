package main

import (
	"encoding/json"
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (b *bridge) startWebServer(addr, cfgPath string) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(uiHTML)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ha, bayrol, uptime := b.status.get()
		writeJSON(w, map[string]any{
			"ok":              ha && bayrol,
			"ha_connected":    ha,
			"bayrol_connected": bayrol,
			"uptime_s":        int(uptime.Seconds()),
		})
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

	log.Printf("web ui listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("web server: %v", err)
	}
}
