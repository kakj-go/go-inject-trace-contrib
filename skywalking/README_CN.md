# skywalking

[English](README.md) | [中文](README_CN.md)

将 [skywalking-go](https://github.com/apache/skywalking-go) 的全部
[plugin](https://github.com/apache/skywalking-go/tree/main/plugins) 转化为
go-inject 规则模板。用法见[主 README](../README_CN.md);本文件记录
运行时配置。

## 配置

运行时从 `SW_AGENT_*` 环境变量读取配置。必配项(不配也有默认值,但只
适合本地冒烟测试):

| 变量 | 含义 | 不设置时的默认值 |
|---|---|---|
| `SW_AGENT_NAME` | 服务名 | `Your_ApplicationName` |
| `SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE` | OAP 后端地址 | `127.0.0.1:11800` |

其余变量——采样率(`SW_AGENT_SAMPLE`)、reporter 各类间隔与认证
(`SW_AGENT_REPORTER_*`)、实例名、路径忽略、以及各插件选项
(`SW_AGENT_PLUGIN_CONFIG_*`)——变量集与
[官方 skywalking-go agent][swcfg] 完全一致且兼容,完整列表与默认值参考
官方文档。注意插件选项默认**不采集**参数/请求头等敏感内容,需要时按
官方变量名打开(如 `SW_AGENT_PLUGIN_CONFIG_SQL_COLLECT_PARAMETER=true`)。

典型启动:

```bash
SW_AGENT_NAME=demo-frontend \
SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE=oap:11800 \
./frontend
```

[swcfg]: https://skywalking.apache.org/docs/skywalking-go/next/en/setup/configurations/
