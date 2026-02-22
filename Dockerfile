# ============================================================
# Build stage
# ============================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w \
    -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev) \
    -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) \
    -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /sidequest ./cmd/sidequest/

# ============================================================
# Runtime stage
# ============================================================
FROM alpine:3.21

# Enable community repo for btop.
RUN echo "https://dl-cdn.alpinelinux.org/alpine/v3.21/community" >> /etc/apk/repositories

RUN apk add --no-cache \
    # Metrics TUI
    btop \
    # DNS tools
    bind-tools \
    # Network tools
    curl wget \
    iputils \
    mtr \
    iproute2 \
    net-tools \
    nmap \
    tcpdump \
    # Process tools
    procps \
    # Storage tools
    util-linux \
    # General
    ca-certificates \
    bash \
    jq

# Install kubectl.
ARG KUBECTL_VERSION=1.31.0
ARG TARGETARCH
RUN wget -q "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
    -O /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl

# Install k9s.
ARG K9S_VERSION=0.32.7
RUN wget -q "https://github.com/derailed/k9s/releases/download/v${K9S_VERSION}/k9s_Linux_${TARGETARCH}.tar.gz" \
    -O /tmp/k9s.tar.gz && tar -xzf /tmp/k9s.tar.gz -C /usr/local/bin k9s && rm /tmp/k9s.tar.gz

# Copy sidequest binary.
COPY --from=builder /sidequest /usr/local/bin/sidequest

# Set locale for btop.
ENV LANG=C.UTF-8

# Default: start all configured servers.
ENTRYPOINT ["sidequest"]
CMD ["serve"]

EXPOSE 8080 8081 8082 8443 9090 5353/udp
