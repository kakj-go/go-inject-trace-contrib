// anthropic.go: the anthropic-sdk-go middleware, ported from
// go.opentelemetry.io/otelc instrumentation/github.com/anthropics/anthropic-sdk-go
// (middleware.go). Streaming requests are passed through uninstrumented (span
// accumulation is not implemented upstream either); SSE responses end the
// span without response attributes.
package genai

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"

	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

// anthropicProviderEntries mirrors the upstream provider mapping.
var anthropicProviderEntries = []struct{ keyword, provider string }{
	{"anthropic.com", "anthropic"},
	{"localhost", "local"},
	{"127.0.0.1", "local"},
}

func getAnthropicProviderName(host string) string {
	for _, e := range anthropicProviderEntries {
		if strings.Contains(host, e.keyword) {
			return e.provider
		}
	}
	return "anthropic"
}

func classifyAnthropicOperation(path string) operationType {
	if strings.HasSuffix(path, "/messages/count_tokens") {
		return opCountTokens
	}
	if strings.HasSuffix(path, "/messages") {
		return opMessages
	}
	return opUnknown
}

func anthropicOperationName(op operationType) string {
	if op == opMessages {
		return "chat"
	}
	if op == opCountTokens {
		// GenAI semconv has no standard name for token counting yet.
		return "count_tokens"
	}
	return ""
}

// AnthropicMiddleware returns the anthropic-sdk-go HTTP middleware for the
// given scope name.
func AnthropicMiddleware(scope string) func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	state := ensureScope(scope)
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		if req.Body == nil {
			return next(req)
		}

		op := classifyAnthropicOperation(req.URL.Path)
		if op == opUnknown {
			return next(req)
		}

		start := time.Now()
		provider := getAnthropicProviderName(req.URL.Host)
		opName := anthropicOperationName(op)

		bodyBytes, body, err := teeBody(req.Body, maxRequestBodySize)
		req.Body = body
		if err != nil {
			return next(req)
		}

		model, isStream, spanAttrs := parseMessagesRequest(bodyBytes)

		// An empty model means nothing meaningful to attach to a span.
		if model == "" {
			return next(req)
		}

		// Streaming responses need event accumulation before their spans
		// carry usage data; upstream passes streaming requests through
		// uninstrumented rather than emit incomplete spans.
		if isStream {
			return next(req)
		}

		spanName := opName + " " + model
		baseAttrs := []attribute.KeyValue{
			attrStr(GenAISystemKey, "anthropic"),
			attrStr(GenAIOperationNameKey, opName),
			attrStr(GenAIRequestModelKey, model),
			attrStr(GenAIProviderNameKey, provider),
		}
		spanAttrs = append(baseAttrs, spanAttrs...)

		ctx := req.Context()
		ctx, span := state.tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(spanAttrs...),
		)
		ctx = hooksupport.SuppressHTTPClientInstrumentation(ctx)
		req = req.WithContext(ctx)

		// Record the operation duration on every exit path below; errorAttrs
		// holds the error.type attribute on error paths and is empty on
		// success.
		var errorAttrs []attribute.KeyValue
		defer func() {
			if state.operationDuration != nil {
				attrs := concatAttrs(baseAttrs, errorAttrs)
				state.operationDuration.Record(ctx, time.Since(start).Seconds(),
					metric.WithAttributes(attrs...))
			}
		}()

		resp, err := next(req)
		if err != nil {
			errorTypeAttr := otelsemconv.ErrorType(err)
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			span.SetAttributes(errorTypeAttr)
			span.End()
			errorAttrs = []attribute.KeyValue{errorTypeAttr}
			return resp, err
		}

		if resp.StatusCode >= 400 {
			errorTypeAttr := otelsemconv.ErrorTypeKey.String(strconv.Itoa(resp.StatusCode))
			span.RecordError(errors.New(resp.Status))
			span.SetStatus(codes.Error, resp.Status)
			span.SetAttributes(errorTypeAttr)
			span.End()
			errorAttrs = []attribute.KeyValue{errorTypeAttr}
			return resp, nil
		}

		// Streaming requests were already passed through above; if the server
		// still answers with SSE, end the span without response attributes.
		contentType := strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
		if strings.EqualFold(contentType, "text/event-stream") {
			span.SetAttributes(attrBool(GenAIRequestIsStreamKey, true))
			span.End()
			return resp, nil
		}

		handleAnthropicNonStreamingResponse(resp, span, op)

		return resp, nil
	}
}

func handleAnthropicNonStreamingResponse(resp *http.Response, span trace.Span, op operationType) {
	defer span.End()

	if resp.Body == nil {
		return
	}

	bodyBytes, body, err := teeBody(resp.Body, maxResponseBodySize)
	resp.Body = body
	if err != nil {
		return
	}

	if op == opCountTokens {
		parseCountTokensResponse(bodyBytes, span)
		return
	}
	parseMessagesResponse(bodyBytes, span)
}

func parseCountTokensResponse(body []byte, span trace.Span) {
	var resp struct {
		InputTokens int64 `json:"input_tokens"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}
	span.SetAttributes(attrI64(GenAIUsageInputTokensKey, resp.InputTokens))
}

func parseMessagesRequest(body []byte) (string, bool, []attribute.KeyValue) {
	var req struct {
		Model       string   `json:"model"`
		Stream      bool     `json:"stream,omitempty"`
		MaxTokens   *int64   `json:"max_tokens,omitempty"`
		Temperature *float64 `json:"temperature,omitempty"`
		TopP        *float64 `json:"top_p,omitempty"`
		TopK        *int64   `json:"top_k,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", false, nil
	}

	var attrs []attribute.KeyValue
	if req.MaxTokens != nil {
		attrs = append(attrs, attrI64(GenAIRequestMaxTokensKey, *req.MaxTokens))
	}
	if req.Temperature != nil {
		attrs = append(attrs, attrF64(GenAIRequestTemperatureKey, *req.Temperature))
	}
	if req.TopP != nil {
		attrs = append(attrs, attrF64(GenAIRequestTopPKey, *req.TopP))
	}
	if req.TopK != nil {
		attrs = append(attrs, attrI64(GenAIRequestTopKKey, *req.TopK))
	}
	return req.Model, req.Stream, attrs
}

func parseMessagesResponse(body []byte, span trace.Span) {
	var resp struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}

	// Unlike OpenAI's prompt_tokens, Anthropic's input_tokens excludes cache
	// reads and creations, which are reported separately. Fold them back in so
	// gen_ai.usage.input_tokens reflects the full prompt per semconv.
	totalInput := resp.Usage.InputTokens +
		resp.Usage.CacheReadInputTokens +
		resp.Usage.CacheCreationInputTokens

	attrs := []attribute.KeyValue{
		attrStr(GenAIResponseIDKey, resp.ID),
		attrStr(GenAIResponseModelKey, resp.Model),
		attrI64(GenAIUsageInputTokensKey, totalInput),
		attrI64(GenAIUsageOutputTokensKey, resp.Usage.OutputTokens),
		// The Messages API reports no total_tokens field; derive it so the
		// span shape matches the other GenAI instrumentations.
		attrI64(GenAIUsageTotalTokensKey, totalInput+resp.Usage.OutputTokens),
	}
	if resp.StopReason != "" {
		attrs = append(attrs, GenAIResponseFinishReasonsKey.StringSlice([]string{resp.StopReason}))
	}
	// Prompt-cache usage is Anthropic-specific; only record it when the
	// request actually used the cache.
	if resp.Usage.CacheReadInputTokens > 0 {
		attrs = append(attrs, attrI64(attribute.Key("gen_ai.usage.cache_read.input_tokens"), resp.Usage.CacheReadInputTokens))
	}
	if resp.Usage.CacheCreationInputTokens > 0 {
		attrs = append(attrs, attrI64(attribute.Key("gen_ai.usage.cache_creation.input_tokens"), resp.Usage.CacheCreationInputTokens))
	}
	span.SetAttributes(attrs...)
}
