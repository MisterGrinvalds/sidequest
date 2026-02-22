# sidequest

A multi-tool Swiss Army knife container for Kubernetes and container runtimes. Single Go binary, Alpine-based image, packed with HTTP/REST/gRPC/GraphQL/DNS servers, an OIDC identity provider, network diagnostics, container inspection, and metrics.

## Quick start

```sh
# Run locally
go build -o sidequest ./cmd/sidequest
./sidequest serve

# Run in Docker
docker build -t sidequest .
docker run --rm -p 8080:8080 -p 8081:8081 -p 8082:8082 -p 9090:9090 sidequest
```

Open `http://localhost:8080` for the interactive docs landing page.

## Servers

All servers share an in-memory item store. Items created via REST are queryable via gRPC and GraphQL.

| Server | Default Port | Default | Protocol | Explorer |
|--------|-------------|---------|----------|----------|
| HTTP Echo | 8080 | enabled | HTTP | Landing page at `/` |
| REST API | 8081 | enabled | HTTP | REST explorer at `/` |
| GraphQL | 8082 | enabled | HTTP | GraphiQL at `/playground` |
| gRPC | 9090 | enabled | gRPC | Server reflection enabled |
| DNS | 5353 | disabled | DNS | -- |
| Identity (OIDC) | 8443 | disabled | HTTP | OIDC explorer at `/` |

### HTTP Echo (`:8080`)

```sh
curl http://localhost:8080/echo           # echo request details
curl http://localhost:8080/headers        # request headers
curl http://localhost:8080/ip             # client IP
curl http://localhost:8080/delay/3        # 3-second delayed response
curl http://localhost:8080/status/418     # specific status code
curl http://localhost:8080/health         # health check
curl http://localhost:8080/ready          # readiness check
```

### REST API (`:8081`)

Full CRUD with pagination, sorting, label filtering, ETags, and conditional requests.

```sh
# Create
curl -X POST http://localhost:8081/api/v1/items \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo","data":{"hello":"world"},"labels":{"env":"dev"}}'

# List with filters
curl 'http://localhost:8081/api/v1/items?labels=env:dev&sort=name&limit=10'

# Conditional GET (304 if unchanged)
curl -H 'If-None-Match: "abc123"' http://localhost:8081/api/v1/items/ID
```

### gRPC (`:9090`)

Protobuf-defined ItemService with 6 RPCs including server-side streaming.

```sh
# List services via reflection
grpcurl -plaintext localhost:9090 list

# Create item
grpcurl -plaintext -d '{"name":"test"}' localhost:9090 sidequest.v1.ItemService/CreateItem
```

### GraphQL (`:8082`)

Queries, mutations, pagination, and label filtering. GraphiQL playground at `/playground`.

```sh
curl http://localhost:8082/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ items { items { id name } } }"}'
```

### DNS (`:5353`)

Configurable zones with upstream forwarding to `8.8.8.8`.

```sh
dig @localhost -p 5353 sidequest.local
```

### Identity Provider (`:8443`)

Lightweight OIDC-compatible provider with JWT issuance and validation.

```sh
# OIDC discovery
curl http://localhost:8443/.well-known/openid-configuration

# Get a token
curl -X POST http://localhost:8443/token \
  -u sidequest-client:sidequest-secret \
  -d 'grant_type=client_credentials'

# Introspect
curl -X POST http://localhost:8443/introspect \
  -d 'token=YOUR_TOKEN'
```

Default client: `sidequest-client` / `sidequest-secret`.

## CLI

```
sidequest serve              Start all enabled servers
sidequest http serve          Start HTTP echo server only
sidequest rest serve          Start REST API server only
sidequest grpc serve          Start gRPC server only
sidequest graphql serve       Start GraphQL server only
sidequest dns serve           Start DNS server only
sidequest identity serve      Start OIDC provider only

sidequest http get URL        HTTP client
sidequest rest list           REST client (list/get/create/update/delete)
sidequest grpc list           gRPC client (list/get/create/update/delete/watch)
sidequest graphql query       GraphQL client
sidequest dns lookup HOST     DNS client
sidequest identity token      Request OIDC token
sidequest identity validate   Validate JWT against JWKS
sidequest identity inspect    Decode JWT claims

sidequest net ping HOST       Ping
sidequest net trace HOST      Traceroute (mtr fallback)
sidequest net ports HOST      Port scan
sidequest net interfaces      Network interfaces

sidequest container info      Runtime detection
sidequest container cgroups   Cgroup inspection
sidequest container caps      Capabilities
sidequest container ns        Namespaces
sidequest container ps        Process listing

sidequest storage df          Disk usage
sidequest storage mounts      Mount points
sidequest storage bench       I/O benchmark

sidequest k8s pods            kubectl get pods
sidequest k8s logs POD        kubectl logs
sidequest k8s events          kubectl get events
sidequest k8s exec POD        kubectl exec
sidequest k8s info            kubectl cluster-info
sidequest k8s top             kubectl top
sidequest k8s k9s             Launch k9s

sidequest metrics             Launch btop++
sidequest deploy k8s          Deploy to Kubernetes
sidequest undeploy k8s        Remove from Kubernetes
sidequest version             Build info
```

## Configuration

All configuration is via environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `SIDEQUEST_HTTP_ENABLED` | `true` | Enable HTTP echo server |
| `SIDEQUEST_HTTP_PORT` | `8080` | HTTP echo server port |
| `SIDEQUEST_REST_ENABLED` | `true` | Enable REST API server |
| `SIDEQUEST_REST_PORT` | `8081` | REST API server port |
| `SIDEQUEST_GRPC_ENABLED` | `true` | Enable gRPC server |
| `SIDEQUEST_GRPC_PORT` | `9090` | gRPC server port |
| `SIDEQUEST_GRAPHQL_ENABLED` | `true` | Enable GraphQL server |
| `SIDEQUEST_GRAPHQL_PORT` | `8082` | GraphQL server port |
| `SIDEQUEST_DNS_ENABLED` | `false` | Enable DNS server |
| `SIDEQUEST_DNS_PORT` | `5353` | DNS server port |
| `SIDEQUEST_IDENTITY_ENABLED` | `false` | Enable OIDC identity provider |
| `SIDEQUEST_IDENTITY_PORT` | `8443` | OIDC provider port |
| `SIDEQUEST_IDENTITY_ISSUER` | `http://localhost:8443` | OIDC issuer URL |

## Building

```sh
make build          # Build binary to bin/sidequest
make test           # Run all tests
make test-cover     # Tests with coverage report
make docker-build   # Build container image (local arch)
make docker-push    # Build and push multi-arch image via lazyoci
```

### KIND workflow

```sh
make kind-load      # Build image and load into KIND cluster
make kind-deploy    # Build, load, and deploy to KIND
make kind-undeploy  # Remove from KIND cluster
```

Override the cluster name with `KIND_CLUSTER=my-cluster make kind-load`.

## Container image

Alpine 3.21 base with bundled tools:

- **sidequest** binary
- **kubectl** 1.31
- **k9s** 0.32.7
- **btop++** for system metrics
- **Network tools**: curl, dig, mtr, nmap, tcpdump, iproute2
- **Process tools**: procps
- **Storage tools**: util-linux

```sh
# Run with all servers + DNS + OIDC
docker run --rm \
  -e SIDEQUEST_DNS_ENABLED=true \
  -e SIDEQUEST_IDENTITY_ENABLED=true \
  -p 8080:8080 -p 8081:8081 -p 8082:8082 -p 9090:9090 -p 5353:5353/udp -p 8443:8443 \
  sidequest
```

## Kubernetes deployment

```sh
# Deploy with port-forward (cleans up on Ctrl+C)
sidequest deploy k8s --image sidequest:latest

# Deploy without port-forward
sidequest deploy k8s --image sidequest:latest --port-forward=false

# Custom namespace and replicas
sidequest deploy k8s --image sidequest:latest -n my-ns --replicas 3

# Remove
sidequest undeploy k8s
```

## Project structure

```
cmd/sidequest/          Entrypoint
internal/
  cli/                  Cobra commands (root, serve, deploy, per-server CLIs)
  config/               Environment variable configuration
  store/                Thread-safe in-memory item store with events
  server/
    http/               HTTP echo server
    rest/               REST CRUD server
    grpc/               gRPC server (protobuf-based)
    graphql/            GraphQL server
    dns/                DNS server
    identity/           OIDC identity provider
    ui/                 Embedded HTML templates and static assets
  client/
    http/               HTTP client
    grpc/               gRPC client
    graphql/            GraphQL client
proto/sidequest/v1/     Protobuf definitions and generated code
```
