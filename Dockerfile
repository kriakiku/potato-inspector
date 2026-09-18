# syntax=docker/dockerfile:1

FROM oven/bun:1-alpine AS webbuild
WORKDIR /web
COPY web/package.json web/bun.lock* web/package-lock.json* ./
RUN bun install
COPY web/ ./
COPY docs/ /docs/
RUN bun run build

FROM golang:1.23-bookworm AS gobuild
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=webbuild /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/potatoinspector ./cmd/potatoinspector

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    iproute2 \
    iptables \
    iputils-ping \
    python3 \
    python3-pip \
    python3-venv \
    libcap2-bin \
    && rm -rf /var/lib/apt/lists/* \
    && python3 -m venv /opt/mitm \
    && /opt/mitm/bin/pip install --no-cache-dir mitmproxy \
    && ln -sf /opt/mitm/bin/mitmdump /usr/local/bin/mitmdump \
    && ln -sf /opt/mitm/bin/mitmproxy /usr/local/bin/mitmproxy

COPY --from=gobuild /out/potatoinspector /usr/local/bin/potatoinspector
COPY mitmaddon /app/mitmaddon
COPY profiles /app/profiles

ENV POTATOINSPECTOR_DATA=/data \
    POTATOINSPECTOR_WG_SUBNET=10.8.0.0/24 \
    POTATOINSPECTOR_WG_PORT=51820 \
    POTATOINSPECTOR_PANEL=8443 \
    POTATOINSPECTOR_ADDON=/app/mitmaddon \
    PATH="/opt/mitm/bin:${PATH}"

VOLUME ["/data"]
EXPOSE 51820/udp 8443 80

ENTRYPOINT ["/usr/local/bin/potatoinspector"]
