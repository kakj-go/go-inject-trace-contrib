# skywalking

[English](README.md) | [中文](README_CN.md)

Ports every [plugin](https://github.com/apache/skywalking-go/tree/main/plugins)
of [skywalking-go](https://github.com/apache/skywalking-go) onto go-inject
rule templates. See the [main README](../README.md) for usage; this file
documents runtime configuration.

## Configuration

Runtime configuration is read from `SW_AGENT_*` environment variables.
Required (defaults exist but are only useful for local smoke tests):

| Variable | Meaning | Default if unset |
|---|---|---|
| `SW_AGENT_NAME` | service name | `Your_ApplicationName` |
| `SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE` | OAP backend address | `127.0.0.1:11800` |

Everything else — sampling (`SW_AGENT_SAMPLE`), reporter intervals and
authentication (`SW_AGENT_REPORTER_*`), instance name, path exclusion, and
per-plugin options (`SW_AGENT_PLUGIN_CONFIG_*`) — uses the same variable
set as the [official skywalking-go agent][swcfg] and is compatible with it;
see the official docs for the full list and defaults. Note that plugin
options collect **nothing** sensitive (parameters, request headers, …) by
default — enable them by their official names when needed (e.g.
`SW_AGENT_PLUGIN_CONFIG_SQL_COLLECT_PARAMETER=true`).

Typical launch:

```bash
SW_AGENT_NAME=demo-frontend \
SW_AGENT_REPORTER_GRPC_BACKEND_SERVICE=oap:11800 \
./frontend
```

[swcfg]: https://skywalking.apache.org/docs/skywalking-go/next/en/setup/configurations/
