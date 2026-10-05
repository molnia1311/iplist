# iplist

A small Go HTTP server that serves line-delimited IPv4 allowlists fetched from
external sources and kept in memory.

## Endpoints

- `GET /` – list available endpoints
- `GET /github` – IPv4 CIDRs from the GitHub meta API
- `GET /azure/{tag}` – IPv4 CIDRs for the official Azure Service Tag `{tag}`
  (e.g. `/azure/AzureBotService`, `/azure/MicrosoftTeams`)
- `GET /metrics` – Prometheus metrics

## Metrics

The `/metrics` endpoint exposes the following Prometheus-format metrics:

| Name | Type | Labels | Description |
|---|---|---|---|
| `iplist_prefixes_total` | gauge | `list` | Number of IPv4 prefixes cached for each allowlist. |
| `iplist_fetch_total` | counter | `list`, `status` | Total number of fetch attempts per allowlist and outcome. |
| `iplist_last_success_timestamp` | gauge | `list` | Unix timestamp of the last successful fetch for each allowlist. |
| `iplist_staleness_seconds` | gauge | `list` | Seconds since the last successful fetch for each allowlist. |

The `list` label is the full list key `source:list` (e.g. `github:github`,
`azure:AzureBotService`); `status` is `success` or `failure`.

Example output:

```text
# HELP iplist_prefixes_total Number of IPv4 prefixes cached for each allowlist.
# TYPE iplist_prefixes_total gauge
iplist_prefixes_total{list="azure:AzureBotService"} 267
iplist_prefixes_total{list="github:github"} 20

# HELP iplist_fetch_total Total number of fetch attempts per allowlist and outcome.
# TYPE iplist_fetch_total counter
iplist_fetch_total{list="azure:AzureBotService",status="success"} 12
iplist_fetch_total{list="github:github",status="success"} 12
iplist_fetch_total{list="github:github",status="failure"} 1

# HELP iplist_last_success_timestamp Unix timestamp of the last successful fetch for each allowlist.
# TYPE iplist_last_success_timestamp gauge
iplist_last_success_timestamp{list="azure:AzureBotService"} 1759937500
iplist_last_success_timestamp{list="github:github"} 1759937500

# HELP iplist_staleness_seconds Seconds since the last successful fetch for each allowlist.
# TYPE iplist_staleness_seconds gauge
iplist_staleness_seconds{list="azure:AzureBotService"} 47
iplist_staleness_seconds{list="github:github"} 47
```

## Running

```bash
go run .
```

The server listens on `:8080` by default. Use `-addr` or `LISTEN_ADDR` to change it.

## Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | Address to listen on |
| `REFRESH_INTERVAL` | `1h` | How often to refresh allowlists |
| `FETCH_TIMEOUT` | `30s` | Upstream HTTP fetch timeout |

## Example

```bash
curl http://localhost:8080/github
curl http://localhost:8080/azure/AzureBotService
curl http://localhost:8080/metrics
```

## Building

```bash
make check    # fmt, vet, build, test
make lint     # requires golangci-lint
make tidy     # ensure go.mod/go.sum are clean
```

## Container

```bash
docker build -t iplist .
docker run -p 8080:8080 iplist
```

## Sources

- Azure Service Tags – Public Cloud: https://www.microsoft.com/en-us/download/details.aspx?id=56519
- GitHub meta API: https://api.github.com/meta

If an upstream source is unavailable, the last successfully fetched list is served.
