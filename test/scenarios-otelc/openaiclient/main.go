// OpenAI scenario: embeds a mock API server (mirrors the upstream
// integration test) and self-drives one non-streaming chat completion, which
// the injected GenAI middleware turns into a "chat <model>" CLIENT span with
// usage attributes.
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

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

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
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var reqBody struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":     "chatcmpl-test-123",
			"object": "chat.completion",
			"model":  reqBody.Model,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
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
		runClient(fmt.Sprintf("http://127.0.0.1:%s/v1", *port))
		time.Sleep(1 * time.Second)
		ready.Store(true)
	}()

	select {}
}

func runClient(baseURL string) {
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("test-key"),
	)
	completion, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Say hello in one word"),
		},
		Model: openai.ChatModel("gpt-4o-mini"),
	})
	if err != nil {
		log.Printf("chat completion failed: %v", err)
		return
	}
	log.Printf("response: %s", completion.Choices[0].Message.Content)
}
