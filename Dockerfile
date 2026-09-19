# syntax=docker/dockerfile:1

FROM golang:1.27-bookworm AS gobuild
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
    && (apt-get install -y --no-install-recommends upx-ucl || true) \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/potatonetwork ./cmd/potatonetwork \
    && (command -v upx >/dev/null && upx --best --lzma /out/potatonetwork || true)

# Smallest suitable runtime: scratch + single static binary.
# Catalog is go:embed'd; Mozilla roots via breml/rootcerts; netlink/nftables need no userland helpers.
# Require Docker NET_ADMIN (+ ifb available/loadable on the host kernel).
FROM scratch
COPY --from=gobuild /out/potatonetwork /usr/local/bin/potatonetwork

ENV POTATONETWORK_DATA=/data \
    POTATONETWORK_API_ADDR=:7783

VOLUME ["/data"]
EXPOSE 53/udp 53/tcp 7783

ENTRYPOINT ["/usr/local/bin/potatonetwork"]
