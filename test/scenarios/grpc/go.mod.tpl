module test/plugins/scenarios/grpc

go 1.20

require (
	google.golang.org/grpc {{FRAMEWORK_VERSION}}
)

require (
	github.com/cncf/xds/go v0.0.0-20230607035331-e9ce68804cb4 // indirect
	github.com/envoyproxy/protoc-gen-validate v0.10.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	skywalking.apache.org/repo/goapi v0.0.0-20230314034821-0c5a44bb767a // indirect
)
require (
	github.com/apache/skywalking-go v0.7.0
	github.com/kakj-go/go-inject-trace-contrib v0.0.0
	google.golang.org/genproto v0.0.0-20240213162025-012b6fc9bca9
	dubbo.apache.org/dubbo-go/v3 v3.0.1
	github.com/apache/pulsar-client-go v0.12.0
	github.com/apache/rocketmq-client-go/v2 v2.1.2
	github.com/elastic/go-elasticsearch/v8 v8.11.1
	github.com/emicklei/go-restful/v3 v3.10.2
	github.com/go-kratos/kratos/v2 v2.6.2
	github.com/go-sql-driver/mysql v1.7.1
	github.com/gofiber/fiber/v2 v2.50.0
	github.com/gogf/gf/v2 v2.7.3
	github.com/gorilla/mux v1.8.0
	github.com/jackc/pgx/v5 v5.5.5
	github.com/kataras/iris/v12 v12.2.0
	github.com/labstack/echo/v4 v4.11.4
	github.com/rabbitmq/amqp091-go v1.9.0
	github.com/redis/go-redis/v9 v9.0.5
	github.com/segmentio/kafka-go v0.4.47
	github.com/valyala/fasthttp v1.51.0
	go-micro.dev/v4 v4.9.0
	go.mongodb.org/mongo-driver v1.11.7
	gorm.io/driver/mysql v1.5.1
	gorm.io/driver/postgres v1.5.2
	gorm.io/gorm v1.25.1
	github.com/apache/skywalking-go/toolkit v0.7.0
)

replace github.com/kakj-go/go-inject-trace-contrib => /contrib

replace google.golang.org/grpc => google.golang.org/grpc {{FRAMEWORK_VERSION}}
replace go.opentelemetry.io/otel => go.opentelemetry.io/otel v1.24.0
replace go.opentelemetry.io/otel/trace => go.opentelemetry.io/otel/trace v1.24.0
replace go.opentelemetry.io/otel/metric => go.opentelemetry.io/otel/metric v1.24.0
replace go.opentelemetry.io/otel/sdk => go.opentelemetry.io/otel/sdk v1.24.0
replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.19.1
replace k8s.io/klog/v2 => k8s.io/klog/v2 v2.110.1
replace github.com/go-logr/logr => github.com/go-logr/logr v1.3.0
