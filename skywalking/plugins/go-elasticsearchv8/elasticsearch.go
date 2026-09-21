//go:build goinject

//inject:github.com/elastic/go-elasticsearch/v8
package goelasticsearchv8

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/apache/skywalking-go/plugins/core/tracing"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
)

type BaseClient struct {
}

// Perform mirrors the official go-elasticsearchv8 ESV8Interceptor.
func (c *BaseClient) Perform(req *http.Request) (resp *http.Response, err error) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateExitSpan("Elasticsearch/"+req.Method, swClientURLs(c), func(headerKey, headerValue string) error {
		return nil
	},
		tracing.WithLayer(tracing.SpanLayerDatabase),
		tracing.WithTag(tracing.TagDBType, "Elasticsearch"),
		tracing.WithTag(tracing.TagDBStatement, strings.TrimPrefix(req.URL.Path, "/")),
		tracing.WithComponent(47),
	)
	if swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if resp != nil {
			swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", resp.StatusCode))
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}

//inject:add
func swClientURLs(c *BaseClient) (urls string) {
	defer func() {
		// The official agent recovers any before-invoke panic; in particular a
		// transport that is not *elastictransport.Client is ignored silently.
		_ = recover()
	}()
	var addresses []string
	for _, u := range c.Transport.(*elastictransport.Client).URLs() {
		addresses = append(addresses, u.String())
	}
	return strings.Join(addresses, ",")
}
