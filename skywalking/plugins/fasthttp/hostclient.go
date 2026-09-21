//go:build goinject

//inject:github.com/valyala/fasthttp
package fasthttp

import (
	"fmt"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type HostClient struct {
}

func (hc *HostClient) Do(req *Request, resp *Response) (err error) {
	swSpan := tracing.Span(nil)
	s, swErr := tracing.CreateExitSpan(fmt.Sprintf("%s:%s", string(req.Header.Method()), req.URI().String()),
		string(req.Host()), func(headerKey, headerValue string) error {
			req.Header.Add(headerKey, headerValue)
			return nil
		}, tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, string(req.Header.Method())),
		tracing.WithTag(tracing.TagURL, req.URI().String()),
		tracing.WithComponent(5019))
	if swErr == nil {
		swSpan = s
	}
	defer func() {
		if swSpan == nil {
			return
		}
		if resp != nil {
			if resp.StatusCode() >= 400 {
				swSpan.Error()
			}
			swSpan.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", resp.StatusCode()))
		}
		if err != nil {
			swSpan.Error(err.Error())
		}
		swSpan.End()
	}()
	return
}
