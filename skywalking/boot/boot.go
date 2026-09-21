// Package boot activates the SkyWalking Go tracing runtime for programs
// built with go-inject. Import it normally (import _ ".../skywalking/boot")
// so plugins/core enters the build graph, and let the injected main-package
// initializer call Init.
//
// Init links the goroutine-local storage that the skywalking/runtime rules
// injected into the runtime package, wires the operator entry points, and
// boots the tracer from SW_AGENT_* environment variables, mirroring the
// generated runtime_linker.go / tracer_init.go of the official go-agent.
package boot

import (
	"os"
	"strconv"
	"time"

	_ "unsafe"

	core "github.com/apache/skywalking-go/plugins/core"
	"github.com/apache/skywalking-go/plugins/core/operator"
	"github.com/apache/skywalking-go/plugins/core/reporter"
	grpc_reporter "github.com/apache/skywalking-go/plugins/core/reporter/grpc"
)

// The runtime accessors only exist in builds that inject the skywalking
// runtime rules: the runtime package self-linknames them to bare symbols,
// mirroring the official agent, because Go restricts direct runtime-package
// linkname pulls. The tracer constructor is added to plugins/core by the
// coreinit rule. go:linkname resolves everything at link time, so this
// package still compiles in plain builds where nothing imports it.
var (
	//go:linkname swGetGLS skywalking_get_gls
	swGetGLS func() interface{}
	//go:linkname swSetGLS skywalking_set_gls
	swSetGLS func(interface{})
	//go:linkname swGetGoID skywalking_get_goid
	swGetGoID func() int64
)

//go:linkname swNewTracer github.com/apache/skywalking-go/plugins/core.NewTracerForInject
func swNewTracer() *core.Tracer

var (
	tracer         *core.Tracer
	initNotify     []func()
	metricsPending []interface{}
	metricsHooks   []func()
	tracerReady    []func()
)

// OnTracerReady registers a callback invoked right after the tracer boots
// successfully. It mirrors the official //skywalking:init plugin hook:
// normal package initializers run before the injected main initializer, so
// registrations made here still precede tracer boot.
func OnTracerReady(f func()) { tracerReady = append(tracerReady, f) }

// Init is called from the injected application initializer. It must run
// before any instrumented request handling, which every Go main-package
// initializer guarantees.
func Init() {
	core.GetGLS = swGetGLS
	core.SetGLS = swSetGLS
	core.GetGoID = swGetGoID

	core.GetInitNotify = func() []func() { return initNotify }
	core.MetricsObtain = func() ([]interface{}, []func()) {
		pending, hooks := metricsPending, metricsHooks
		metricsPending, metricsHooks = nil, nil
		return pending, hooks
	}
	operator.AppendInitNotify = func(f func()) { initNotify = append(initNotify, f) }
	operator.MetricsAppender = func(v interface{}) { metricsPending = append(metricsPending, v) }
	operator.MetricsCollectAppender = func(f func()) { metricsHooks = append(metricsHooks, f) }

	tracer = swNewTracer()
	operator.GetOperator = func() operator.Operator {
		if tracer == nil {
			return nil
		}
		return tracer
	}

	rep, err := initReporter(tracer.Log)
	if err != nil {
		tracer.Log.Errorf("cannot initialize the reporter: %v", err)
		return
	}
	entity := core.NewEntity(envString("SW_AGENT_NAME", "Your_ApplicationName"), "SW_AGENT_INSTANCE_NAME")
	sampler := core.NewDynamicSampler(envFloat("SW_AGENT_SAMPLE", 1), tracer)
	meterCollectInterval := envInt("SW_AGENT_METER_COLLECT_INTERVAL", 20)
	correlation := &core.CorrelationConfig{
		MaxKeyCount:  envInt("SW_AGENT_CORRELATION_MAX_KEY_COUNT", 3),
		MaxValueSize: envInt("SW_AGENT_CORRELATION_MAX_VALUE_SIZE", 128),
	}
	ignoreSuffix := envString("SW_AGENT_IGNORE_SUFFIX",
		".jpg,.jpeg,.js,.css,.png,.bmp,.gif,.ico,.mp3,.mp4,.html,.svg")
	ignorePath := envString("SW_AGENT_TRACE_IGNORE_PATH", "")
	if err := tracer.Init(entity, rep, sampler, nil, meterCollectInterval, correlation, ignoreSuffix, ignorePath); err != nil {
		tracer.Log.Errorf("cannot initialize the SkyWalking Tracer: %v", err)
		return
	}
	for _, f := range tracerReady {
		f()
	}
}

func initReporter(logger operator.LogOperator) (reporter.Reporter, error) {
	if envBool("SW_AGENT_REPORTER_DISCARD", false) {
		return reporter.NewDiscardReporter(), nil
	}
	checkInterval := envSeconds("SW_AGENT_REPORTER_CHECK_INTERVAL", 20)
	backendService := envString("SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE", "127.0.0.1:11800")
	auth := envString("SW_AGENT_REPORTER_GRPC_AUTHENTICATION", "")
	connManager, err := reporter.NewConnectionManager(logger, checkInterval, backendService, auth, nil)
	if err != nil {
		return nil, err
	}
	cdsManager, err := reporter.NewCDSManager(logger, backendService,
		envSeconds("SW_AGENT_REPORTER_GRPC_CDS_FETCH_INTERVAL", 20), connManager)
	if err != nil {
		return nil, err
	}
	pprofManager, err := reporter.NewPprofTaskManager(logger, backendService,
		envSeconds("SW_AGENT_REPORTER_GRPC_PPROF_TASK_FETCH_INTERVAL", 20), connManager,
		envString("SW_AGENT_REPORTER_GRPC_PROFILE_PPROF_FILE_PATH", ""))
	if err != nil {
		return nil, err
	}
	return grpc_reporter.NewGRPCReporter(logger, backendService, checkInterval,
		envSeconds("SW_AGENT_REPORTER_GRPC_PROFILE_FETCH_INTERVAL", 20),
		connManager, cdsManager, pprofManager)
}

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envSeconds(key string, def int) time.Duration {
	return time.Duration(envInt(key, def)) * time.Second
}
