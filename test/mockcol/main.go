// mockcol is a minimal OTLP receiver for A/B scenario validation: it accepts
// http/protobuf OTLP on the spec-default endpoints (/v1/traces, /v1/metrics,
// /v1/logs), accumulates the data in memory, and exposes
//
//	GET /receiveData  – all received data as OTLP JSON ({"resourceSpans":...,
//	                    "resourceMetrics":..., "resourceLogs":...})
//	GET /clear        – drop everything received so far
//
// It plays the role the SkyWalking mock-collector image plays for the
// skywalking scenarios: both build sides export to it, and the runner diffs
// the normalized dumps. Design ported from go.opentelemetry.io/otelc
// test/testutil/collector.go, extended to record metrics and logs.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type store struct {
	mu      sync.Mutex
	traces  ptrace.Traces
	metrics pmetric.Metrics
	logs    plog.Logs
}

func newStore() *store {
	return &store{
		traces:  ptrace.NewTraces(),
		metrics: pmetric.NewMetrics(),
		logs:    plog.NewLogs(),
	}
}

func (s *store) receiveData(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Each pdata JSONMarshaler returns a full OTLP JSON object with a single
	// top-level field; unwrap it so /receiveData emits standard OTLP JSON.
	// Absent signals stay absent instead of serializing as null.
	out := []byte{'{'}
	first := true
	appendSignal := func(key string, marshal func() ([]byte, error)) {
		b, err := marshal()
		if err != nil {
			log.Printf("marshal %s: %v", key, err)
			return
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) != nil {
			return
		}
		v, ok := m[key]
		if !ok || string(v) == "null" || string(v) == "[]" { // nothing received for this signal
			return
		}
		if !first {
			out = append(out, ',')
		}
		first = false
		out = append(out, '"')
		out = append(out, key...)
		out = append(out, '"', ':')
		out = append(out, v...)
	}
	appendSignal("resourceSpans", func() ([]byte, error) {
		return (&ptrace.JSONMarshaler{}).MarshalTraces(s.traces)
	})
	appendSignal("resourceMetrics", func() ([]byte, error) {
		return (&pmetric.JSONMarshaler{}).MarshalMetrics(s.metrics)
	})
	appendSignal("resourceLogs", func() ([]byte, error) {
		return (&plog.JSONMarshaler{}).MarshalLogs(s.logs)
	})
	out = append(out, '}')

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (s *store) clear(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.traces = ptrace.NewTraces()
	s.metrics = pmetric.NewMetrics()
	s.logs = plog.NewLogs()
	w.WriteHeader(http.StatusOK)
}

func handleTraces(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		var u ptrace.ProtoUnmarshaler
		td, err := u.UnmarshalTraces(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		td.ResourceSpans().MoveAndAppendTo(s.traces.ResourceSpans())
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func handleMetrics(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		var u pmetric.ProtoUnmarshaler
		md, err := u.UnmarshalMetrics(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		md.ResourceMetrics().MoveAndAppendTo(s.metrics.ResourceMetrics())
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func handleLogs(s *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		var u plog.ProtoUnmarshaler
		ld, err := u.UnmarshalLogs(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		ld.ResourceLogs().MoveAndAppendTo(s.logs.ResourceLogs())
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}
}

func main() {
	addr := os.Getenv("MOCKCOL_ADDR")
	if addr == "" {
		addr = ":4318"
	}

	s := newStore()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", handleTraces(s))
	mux.HandleFunc("/v1/metrics", handleMetrics(s))
	mux.HandleFunc("/v1/logs", handleLogs(s))
	mux.HandleFunc("/receiveData", s.receiveData)
	mux.HandleFunc("/clear", s.clear)

	fmt.Printf("mockcol listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
