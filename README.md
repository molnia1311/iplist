# iplist

A small Go HTTP server that serves line-delimited IPv4 allowlists fetched from
external sources and kept in memory.

## Endpoints

- `GET /` – list available endpoints
- `GET /github` – IPv4 CIDRs from the GitHub meta API
- `GET /azure/teams` – IPv4 CIDRs for the Azure `AzureBotService` service tag
- `GET /azure/microsoftteams` – alias for `/azure/teams`
- `GET /metrics` – Prometheus metrics

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
curl http://localhost:8080/azure/teams
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
