module test/plugins/scenarios/irisv12

go 1.19

require (
	github.com/kataras/iris/v12 {{FRAMEWORK_VERSION}}
)

require (
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/CloudyKit/fastprinter v0.0.0-20200109182630-33d98a066a53 // indirect
	github.com/CloudyKit/jet/v6 v6.2.0 // indirect
	github.com/Joker/jade v1.1.3 // indirect
	github.com/Shopify/goreferrer v0.0.0-20220729165902-8cddb4f5de06 // indirect
	github.com/andybalholm/brotli v1.0.5 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/eknkc/amber v0.0.0-20171010120322-cdade1c07385 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/flosch/pongo2/v4 v4.0.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/iris-contrib/schema v0.0.6 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/kataras/blocks v0.0.7 // indirect
	github.com/kataras/golog v0.1.9 // indirect
	github.com/kataras/pio v0.0.12 // indirect
	github.com/kataras/sitemap v0.0.6 // indirect
	github.com/kataras/tunnel v0.0.4 // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/mailgun/raymond/v2 v2.0.48 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/microcosm-cc/bluemonday v1.0.25 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/schollz/closestmatch v2.1.0+incompatible // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/tdewolff/minify/v2 v2.12.8 // indirect
	github.com/tdewolff/parse/v2 v2.6.7 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/yosssi/ace v0.0.5 // indirect
	golang.org/x/crypto v0.12.0 // indirect
	golang.org/x/net v0.14.0 // indirect
	golang.org/x/sys v0.11.0 // indirect
	golang.org/x/text v0.12.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/grpc v1.55.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
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
	github.com/labstack/echo/v4 v4.11.4
	github.com/rabbitmq/amqp091-go v1.9.0
	github.com/redis/go-redis/v9 v9.0.5
	github.com/segmentio/kafka-go v0.4.47
	github.com/valyala/fasthttp v1.51.0
	go-micro.dev/v4 v4.9.0
	go.mongodb.org/mongo-driver v1.11.7
	google.golang.org/grpc v1.56.2
	gorm.io/driver/mysql v1.5.1
	gorm.io/driver/postgres v1.5.2
	gorm.io/gorm v1.25.1
	github.com/apache/skywalking-go/toolkit v0.7.0
)

replace github.com/kakj-go/go-inject-trace-contrib => /contrib
replace golang.org/x/net => golang.org/x/net v0.33.0
replace golang.org/x/crypto => golang.org/x/crypto v0.31.0
replace golang.org/x/sys => golang.org/x/sys v0.28.0
replace golang.org/x/text => golang.org/x/text v0.20.0
replace google.golang.org/grpc => google.golang.org/grpc v1.70.0
replace google.golang.org/genproto/googleapis/rpc => google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237
replace go.opentelemetry.io/otel => go.opentelemetry.io/otel v1.24.0
replace go.opentelemetry.io/otel/trace => go.opentelemetry.io/otel/trace v1.24.0
replace go.opentelemetry.io/otel/metric => go.opentelemetry.io/otel/metric v1.24.0
replace go.opentelemetry.io/otel/sdk => go.opentelemetry.io/otel/sdk v1.24.0
replace github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.19.1
replace k8s.io/klog/v2 => k8s.io/klog/v2 v2.110.1
replace github.com/go-logr/logr => github.com/go-logr/logr v1.3.0
