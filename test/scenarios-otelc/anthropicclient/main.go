// Anthropic scenario: embeds a mock API server (mirrors the upstream
// integration test, including prompt-cache usage fields) and self-drives one
// messages call + one count_tokens call, producing "chat claude-sonnet-4-5"
// and "count_tokens claude-sonnet-4-5" GenAI CLIENT spans.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	_ "github.com/kakj-go/go-inject-trace-contrib/otelc"
)

var port = flag.String("port", "8080", "The mock+health server port")

var ready atomic.Bool

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/messages/count_tokens", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"input_tokens": 5})
	})
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		var reqBody struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":   "msg_test_123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "Hello!"},
			},
			"model":       reqBody.Model,
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":              10,
				"output_tokens":             20,
				"cache_read_input_tokens":   5,
				"cache_creation_input_tokens": 2,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	ln, err := net.Listen("tcp", fmt.Sprintf(":%s", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	go func() {
		if err := http.Serve(ln, mux); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	go func() {
		time.Sleep(500 * time.Millisecond)
		runClient(fmt.Sprintf("http://127.0.0.1:%s", *port))
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runClient(baseURL string) {
	client := anthropic.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("test-key"),
	)

	msg, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Say hello in one word")),
		},
		Model: anthropic.ModelClaudeSonnet4_5,
	})
	if err != nil {
		log.Printf("messages failed: %v", err)
		return
	}
	log.Printf("response: %s", msg.Content[0].Text)

	_, err = client.Messages.CountTokens(context.Background(), anthropic.MessageCountTokensParams{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Say hello in one word")),
		},
		Model: anthropic.ModelClaudeSonnet4_5,
	})
	if err != nil {
		log.Printf("count tokens failed: %v", err)
	}
}
