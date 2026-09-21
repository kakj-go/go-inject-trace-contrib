//go:build goinject

//inject:github.com/emicklei/go-restful/v3
package restfulv3

import (
	"fmt"
	"net/http"

	"github.com/apache/skywalking-go/plugins/core/tracing"
)

type Container struct {
	webServices []*WebService
	//inject:add
	swSkywalkingData interface{}
}

func (c *Container) Add(service *WebService) *Container {
	swAddFilterToContainer(c)
}

func (c *Container) Dispatch(httpWriter http.ResponseWriter, httpRequest *http.Request) {
	swAddFilterToContainer(c)
}

func (c *Container) HandleWithFilter(pattern string, handler http.Handler) {
	swAddFilterToContainer(c)
}

//inject:add
var swFilterInstance = func(request *Request, response *Response, chain *FilterChain) {
	s, err := tracing.CreateEntrySpan(request.Request.Method+":"+request.SelectedRoutePath(), func(k string) (string, error) {
		return request.HeaderParameter(k), nil
	}, tracing.WithLayer(tracing.SpanLayerHTTP),
		tracing.WithTag(tracing.TagHTTPMethod, request.Request.Method),
		tracing.WithTag(tracing.TagURL, request.Request.Host+request.Request.URL.Path),
		tracing.WithComponent(5004))
	if err != nil {
		chain.ProcessFilter(request, response)
		return
	}
	defer func() {
		code := response.StatusCode()
		if response.Error() != nil {
			s.Error(response.Error().Error())
		}
		s.Tag(tracing.TagStatusCode, fmt.Sprintf("%d", code))
		s.End()
	}()
	chain.ProcessFilter(request, response)
}

//inject:add
func swAddFilterToContainer(c *Container) {
	if c.swSkywalkingData == nil {
		c.swSkywalkingData = true
	} else {
		return
	}
	c.Filter(swFilterInstance)
}
