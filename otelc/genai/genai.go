// Package genai hosts the GenAI (openai-go / anthropic-sdk-go) HTTP
// middlewares shared by the openaiv1/v2/v3 and anthropic rule templates.
// The middlewares are SDK-independent (net/http + otel only), so they live in
// this ordinary package and the injected NewClient templates reach them with
// a plain import — unlike net/http, importing back into these SDK packages
// closes no cycle.
//
// Ported from go.opentelemetry.io/otelc instrumentation/github.com/openai/openai-go
// (middleware.go + semconv/genai.go, identical across v1/v2/v3 except scope
// name and init log line) and instrumentation/github.com/anthropics/anthropic-sdk-go
// (middleware.go + semconv).
package genai

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/kakj-go/go-inject-trace-contrib/otelc/genai/streaming"
	hooklog "github.com/kakj-go/go-inject-trace-contrib/otelc/hooklog"
	hooksupport "github.com/kakj-go/go-inject-trace-contrib/otelc/hooksupport"
)

const (
	maxRequestBodySize  = 1 << 20 // 1 MB
	maxResponseBodySize = 4 << 20 // 4 MB
)

// ---- genai attribute keys (openai semconv/genai.go; anthropic adds TopK) ----

const (
	GenAISystemKey                   = attribute.Key("gen_ai.system")
	GenAIOperationNameKey            = attribute.Key("gen_ai.operation.name")
	GenAIRequestModelKey             = attribute.Key("gen_ai.request.model")
	GenAIResponseModelKey            = attribute.Key("gen_ai.response.model")
	GenAIResponseIDKey               = attribute.Key("gen_ai.response.id")
	GenAIResponseFinishReasonsKey    = attribute.Key("gen_ai.response.finish_reasons")
	GenAIUsageInputTokensKey         = attribute.Key("gen_ai.usage.input_tokens")
	GenAIUsageOutputTokensKey        = attribute.Key("gen_ai.usage.output_tokens")
	GenAIUsageTotalTokensKey         = attribute.Key("gen_ai.usage.total_tokens")
	GenAIProviderNameKey             = attribute.Key("gen_ai.provider.name")
	GenAIRequestMaxTokensKey         = attribute.Key("gen_ai.request.max_tokens")
	GenAIRequestTemperatureKey       = attribute.Key("gen_ai.request.temperature")
	GenAIRequestTopPKey              = attribute.Key("gen_ai.request.top_p")
	GenAIRequestFrequencyPenaltyKey  = attribute.Key("gen_ai.request.frequency_penalty")
	GenAIRequestPresencePenaltyKey   = attribute.Key("gen_ai.request.presence_penalty")
	GenAIRequestTopKKey              = attribute.Key("gen_ai.request.top_k")
	GenAIRequestIsStreamKey          = attribute.Key("gen_ai.request.is_stream")
	GenAIResponseTimeToFirstTokenKey = attribute.Key("gen_ai.response.time_to_first_token")
)

func attrStr(k attribute.Key, v string) attribute.KeyValue  { return k.String(v) }
func attrI64(k attribute.Key, v int64) attribute.KeyValue   { return k.Int64(v) }
func attrF64(k attribute.Key, v float64) attribute.KeyValue { return k.Float64(v) }
func attrBool(k attribute.Key, v bool) attribute.KeyValue   { return k.Bool(v) }

// ---- per-scope instrumentation state ----

type scopeState struct {
	tracer            trace.Tracer
	operationDuration metric.Float64Histogram
}

var (
	scopeMu     sync.Mutex
	scopes      = map[string]*scopeState{}
	captureOnce sync.Once
	captureOn   bool
)

// contentCaptureFromEnv mirrors upstream's parsing of
// OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT.
func contentCaptureFromEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}

func captureContentEnabled() bool {
	captureOnce.Do(func() {
		captureOn = contentCaptureFromEnv(os.Getenv("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT"))
	})
	return captureOn
}

// ensureScope lazily creates the tracer and duration histogram for a scope
// name (e.g. "go.opentelemetry.io/otelc/instrumentation/github.com/openai/openai-go").
func ensureScope(name string) *scopeState {
	scopeMu.Lock()
	defer scopeMu.Unlock()
	if s, ok := scopes[name]; ok {
		return s
	}
	s := &scopeState{}
	s.tracer = otel.GetTracerProvider().Tracer(
		name,
		trace.WithInstrumentationVersion(hooksupport.ModuleVersion()),
	)
	meter := otel.GetMeterProvider().Meter(
		name,
		metric.WithInstrumentationVersion(hooksupport.ModuleVersion()),
	)
	var err error
	s.operationDuration, err = meter.Float64Histogram(
		"gen_ai.client.operation.duration",
		metric.WithDescription("Duration of GenAI client operations."),
		metric.WithUnit("s"),
	)
	if err != nil {
		hooklog.Logger().Error("failed to create gen_ai.client.operation.duration histogram", "error", err)
	}
	scopes[name] = s
	return s
}

// concatAttrs replaces slices.Concat for older-lang targets.
func concatAttrs(a, b []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

// ---- shared helpers ----

type operationType int

const (
	opChat operationType = iota
	opCompletion
	opEmbedding
	opMessages
	opCountTokens
	opUnknown
)

func recordContentEvents(span trace.Span, name, key string, contents []string) {
	for _, content := range contents {
		if content == "" {
			continue
		}
		span.AddEvent(name, trace.WithAttributes(
			attribute.String(key, streaming.TruncateContent(content)),
		))
	}
}

func contentFromJSON(content json.RawMessage) string {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || bytes.Equal(content, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return text
	}

	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &parts); err == nil {
		texts := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		return strings.Join(texts, "\n")
	}

	marshaled, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(marshaled)
}

// teeBody reads up to limit bytes for parsing while reassembling the full
// body for the SDK/caller.
func teeBody(body io.ReadCloser, limit int64) ([]byte, io.ReadCloser, error) {
	var buf bytes.Buffer
	tee := io.TeeReader(body, &buf)
	bodyBytes, err := io.ReadAll(io.LimitReader(tee, limit))
	reassembled := struct {
		io.Reader
		io.Closer
	}{io.MultiReader(&buf, body), body}
	return bodyBytes, reassembled, err
}

// ---- openai middleware ----

// providerEntries is an ordered slice so keyword matching has deterministic
// priority, unlike map iteration.
var providerEntries = []struct{ keyword, provider string }{
	{"openai.com", "openai"},
	{"azure.com", "azure"},
	{"anthropic.com", "anthropic"},
	{"dashscope.aliyuncs", "qwen"},
	{"volces.com", "ark"},
	{"ark.cn", "ark"},
	{"hunyuan", "tencent"},
	{"tencentcloudapi", "tencent"},
	{"googleapis.com", "google"},
	{"generativelanguage", "google"},
	{"deepseek.com", "deepseek"},
	{"moonshot", "moonshot"},
	{"zhipuai.cn", "zhipu"},
	{"bigmodel.cn", "zhipu"},
	{"baidu.com", "baidu"},
	{"minimax", "minimax"},
	{"siliconflow", "siliconflow"},
	{"together", "together"},
	{"mistral", "mistral"},
	{"groq.com", "groq"},
	{"ollama", "ollama"},
	{"localhost", "local"},
	{"127.0.0.1", "local"},
}

func getProviderName(host string) string {
	for _, entry := range providerEntries {
		if strings.Contains(host, entry.keyword) {
			return entry.provider
		}
	}
	return "openai"
}

func classifyOpenAIOperation(path string) operationType {
	if strings.HasSuffix(path, "chat/completions") {
		return opChat
	}
	if strings.HasSuffix(path, "completions") {
		return opCompletion
	}
	if strings.HasSuffix(path, "embeddings") {
		return opEmbedding
	}
	return opUnknown
}

func openAIOperationName(op operationType) string {
	switch op {
	case opChat:
		return "chat"
	case opCompletion:
		return "text_completion"
	case opEmbedding:
		return "embeddings"
	default:
		return ""
	}
}

// OpenAIMiddleware returns the openai-go HTTP middleware for the given scope
// name. The scope determines the tracer/meter identity (span scope name), so
// each SDK major version passes its own.
func OpenAIMiddleware(scope string) func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	state := ensureScope(scope)
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		if req.Body == nil {
			return next(req)
		}

		op := classifyOpenAIOperation(req.URL.Path)
		if op == opUnknown {
			return next(req)
		}

		start := time.Now()
		provider := getProviderName(req.URL.Host)
		opName := openAIOperationName(op)

		bodyBytes, body, err := teeBody(req.Body, maxRequestBodySize)
		req.Body = body
		if err != nil {
			return next(req)
		}

		var model string
		var spanAttrs []attribute.KeyValue
		captureContent := captureContentEnabled()
		var prompts []string

		switch op {
		case opChat:
			model, spanAttrs, prompts = parseChatRequest(bodyBytes, captureContent)
		case opCompletion:
			model, spanAttrs, prompts = parseCompletionRequest(bodyBytes, captureContent)
		case opEmbedding:
			model, spanAttrs = parseEmbeddingRequest(bodyBytes)
		}

		if model == "" {
			return next(req)
		}

		spanName := opName + " " + model
		baseAttrs := []attribute.KeyValue{
			attrStr(GenAISystemKey, "openai"),
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

		if captureContent {
			recordContentEvents(span, "gen_ai.content.prompt", "gen_ai.prompt", prompts)
		}
		ctx = hooksupport.SuppressHTTPClientInstrumentation(ctx)
		req = req.WithContext(ctx)

		// For streaming responses, the duration metric is recorded when the
		// stream finishes (mirroring the span lifetime) via the streaming reader.
		var errorAttrs []attribute.KeyValue
		isStreaming := false
		defer func() {
			if isStreaming {
				return
			}
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
			span.RecordError(errors.New(resp.Status))
			span.SetStatus(codes.Error, resp.Status)
			span.SetAttributes(otelsemconv.ErrorTypeKey.String(strconv.Itoa(resp.StatusCode)))
			span.End()
			errorAttrs = []attribute.KeyValue{otelsemconv.ErrorTypeKey.String(strconv.Itoa(resp.StatusCode))}
			return resp, nil
		}

		contentType := strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0])
		isStreaming = strings.EqualFold(contentType, "text/event-stream")

		if isStreaming {
			span.SetAttributes(attrBool(GenAIRequestIsStreamKey, true))
			resp.Body = newOpenAIStreamingReader(resp.Body, span, start, op, captureContent, func() {
				if state.operationDuration != nil {
					attrs := concatAttrs(baseAttrs, errorAttrs)
					state.operationDuration.Record(ctx, time.Since(start).Seconds(),
						metric.WithAttributes(attrs...))
				}
			})
		} else {
			handleOpenAINonStreamingResponse(resp, span, op, captureContent)
		}

		return resp, nil
	}
}

func newOpenAIStreamingReader(
	body io.ReadCloser,
	span trace.Span,
	start time.Time,
	op operationType,
	captureContent bool,
	onDone func(),
) io.ReadCloser {
	var streamingOp streaming.OperationType
	switch op {
	case opChat:
		streamingOp = streaming.OpChat
	case opCompletion:
		streamingOp = streaming.OpCompletion
	}
	return streaming.NewStreamingReader(body, span, start, streamingOp, captureContent, streaming.ContentCaptureLimit, onDone)
}

func handleOpenAINonStreamingResponse(
	resp *http.Response,
	span trace.Span,
	op operationType,
	captureContent bool,
) {
	defer span.End()

	bodyBytes, body, err := teeBody(resp.Body, maxResponseBodySize)
	resp.Body = body
	if err != nil {
		return
	}

	switch op {
	case opChat:
		parseChatResponse(bodyBytes, span, captureContent)
	case opCompletion:
		parseCompletionResponse(bodyBytes, span, captureContent)
	case opEmbedding:
		parseEmbeddingResponse(bodyBytes, span)
	}
}

func parseChatRequest(body []byte, captureContent bool) (string, []attribute.KeyValue, []string) {
	var req struct {
		Model               string   `json:"model"`
		MaxTokens           *int64   `json:"max_tokens,omitempty"`
		MaxCompletionTokens *int64   `json:"max_completion_tokens,omitempty"`
		Temperature         *float64 `json:"temperature,omitempty"`
		TopP                *float64 `json:"top_p,omitempty"`
		FrequencyPenalty    *float64 `json:"frequency_penalty,omitempty"`
		PresencePenalty     *float64 `json:"presence_penalty,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", nil, nil
	}

	var attrs []attribute.KeyValue
	if req.MaxCompletionTokens != nil {
		attrs = append(attrs, attrI64(GenAIRequestMaxTokensKey, *req.MaxCompletionTokens))
	} else if req.MaxTokens != nil {
		attrs = append(attrs, attrI64(GenAIRequestMaxTokensKey, *req.MaxTokens))
	}
	if req.Temperature != nil {
		attrs = append(attrs, attrF64(GenAIRequestTemperatureKey, *req.Temperature))
	}
	if req.TopP != nil {
		attrs = append(attrs, attrF64(GenAIRequestTopPKey, *req.TopP))
	}
	if req.FrequencyPenalty != nil {
		attrs = append(attrs, attrF64(GenAIRequestFrequencyPenaltyKey, *req.FrequencyPenalty))
	}
	if req.PresencePenalty != nil {
		attrs = append(attrs, attrF64(GenAIRequestPresencePenaltyKey, *req.PresencePenalty))
	}
	if !captureContent {
		return req.Model, attrs, nil
	}
	return req.Model, attrs, parseChatPrompts(body)
}

func parseCompletionRequest(body []byte, captureContent bool) (string, []attribute.KeyValue, []string) {
	var req struct {
		Model            string   `json:"model"`
		MaxTokens        *int64   `json:"max_tokens,omitempty"`
		Temperature      *float64 `json:"temperature,omitempty"`
		TopP             *float64 `json:"top_p,omitempty"`
		FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
		PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", nil, nil
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
	if req.FrequencyPenalty != nil {
		attrs = append(attrs, attrF64(GenAIRequestFrequencyPenaltyKey, *req.FrequencyPenalty))
	}
	if req.PresencePenalty != nil {
		attrs = append(attrs, attrF64(GenAIRequestPresencePenaltyKey, *req.PresencePenalty))
	}
	if !captureContent {
		return req.Model, attrs, nil
	}
	return req.Model, attrs, parseCompletionPrompts(body)
}

func parseChatPrompts(body []byte) []string {
	var req struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}

	prompts := make([]string, 0, len(req.Messages))
	for _, message := range req.Messages {
		if content := contentFromJSON(message["content"]); content != "" {
			prompts = append(prompts, content)
		}
	}
	return prompts
}

func parseCompletionPrompts(body []byte) []string {
	var req struct {
		Prompt json.RawMessage `json:"prompt"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Prompt) == 0 {
		return nil
	}

	var prompt string
	if err := json.Unmarshal(req.Prompt, &prompt); err == nil {
		return []string{prompt}
	}

	var prompts []string
	if err := json.Unmarshal(req.Prompt, &prompts); err != nil {
		return nil
	}
	return prompts
}

func parseEmbeddingRequest(body []byte) (string, []attribute.KeyValue) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", nil
	}
	return req.Model, nil
}

func parseChatResponse(body []byte, span trace.Span, captureContent bool) {
	var resp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}

	var reasons []string
	for _, c := range resp.Choices {
		if c.FinishReason != "" {
			reasons = append(reasons, c.FinishReason)
		}
	}
	if captureContent {
		recordContentEvents(span, "gen_ai.content.completion", "gen_ai.completion", parseChatCompletions(body))
	}

	span.SetAttributes(
		attrStr(GenAIResponseIDKey, resp.ID),
		attrStr(GenAIResponseModelKey, resp.Model),
		GenAIResponseFinishReasonsKey.StringSlice(reasons),
		attrI64(GenAIUsageInputTokensKey, resp.Usage.PromptTokens),
		attrI64(GenAIUsageOutputTokensKey, resp.Usage.CompletionTokens),
		attrI64(GenAIUsageTotalTokensKey, resp.Usage.TotalTokens),
	)
}

func parseCompletionResponse(body []byte, span trace.Span, captureContent bool) {
	var resp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}

	var reasons []string
	for _, c := range resp.Choices {
		if c.FinishReason != "" {
			reasons = append(reasons, c.FinishReason)
		}
	}
	if captureContent {
		recordContentEvents(span, "gen_ai.content.completion", "gen_ai.completion", parseCompletionContents(body))
	}

	span.SetAttributes(
		attrStr(GenAIResponseIDKey, resp.ID),
		attrStr(GenAIResponseModelKey, resp.Model),
		GenAIResponseFinishReasonsKey.StringSlice(reasons),
		attrI64(GenAIUsageInputTokensKey, resp.Usage.PromptTokens),
		attrI64(GenAIUsageOutputTokensKey, resp.Usage.CompletionTokens),
		attrI64(GenAIUsageTotalTokensKey, resp.Usage.TotalTokens),
	)
}

func parseChatCompletions(body []byte) []string {
	var resp struct {
		Choices []struct {
			Message map[string]json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}

	completions := make([]string, 0, len(resp.Choices))
	for _, choice := range resp.Choices {
		if content := contentFromJSON(choice.Message["content"]); content != "" {
			completions = append(completions, content)
		}
	}
	return completions
}

func parseCompletionContents(body []byte) []string {
	var resp struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}

	completions := make([]string, 0, len(resp.Choices))
	for _, choice := range resp.Choices {
		if choice.Text != "" {
			completions = append(completions, choice.Text)
		}
	}
	return completions
}

func parseEmbeddingResponse(body []byte, span trace.Span) {
	var resp struct {
		Model string `json:"model"`
		Usage struct {
			PromptTokens int64 `json:"prompt_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}

	span.SetAttributes(
		attrStr(GenAIResponseModelKey, resp.Model),
		attrI64(GenAIUsageInputTokensKey, resp.Usage.PromptTokens),
		attrI64(GenAIUsageTotalTokensKey, resp.Usage.TotalTokens),
	)
}
