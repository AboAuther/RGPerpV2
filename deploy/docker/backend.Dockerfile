FROM golang:1.24.2 AS builder

WORKDIR /workspace

COPY backend/go.mod backend/go.sum /workspace/backend/
WORKDIR /workspace/backend
RUN for i in 1 2 3 4 5; do go mod download && exit 0; sleep 2; done; exit 1

WORKDIR /workspace
COPY backend /workspace/backend
COPY deploy /workspace/deploy

WORKDIR /workspace/backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/api-server ./cmd/api-server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/indexer ./cmd/indexer
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/market-data ./cmd/market-data
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/order-executor-worker ./cmd/order-executor-worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/risk-engine-worker ./cmd/risk-engine-worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/funding-worker ./cmd/funding-worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/liquidator-worker ./cmd/liquidator-worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/hedger-worker ./cmd/hedger-worker
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/migrator ./cmd/migrator

FROM debian:bookworm-slim

WORKDIR /workspace

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates python3 python3-pip \
  && python3 -m pip install --break-system-packages --no-cache-dir hyperliquid-python-sdk eth-account \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/api-server /usr/local/bin/api-server
COPY --from=builder /out/indexer /usr/local/bin/indexer
COPY --from=builder /out/market-data /usr/local/bin/market-data
COPY --from=builder /out/order-executor-worker /usr/local/bin/order-executor-worker
COPY --from=builder /out/risk-engine-worker /usr/local/bin/risk-engine-worker
COPY --from=builder /out/funding-worker /usr/local/bin/funding-worker
COPY --from=builder /out/liquidator-worker /usr/local/bin/liquidator-worker
COPY --from=builder /out/hedger-worker /usr/local/bin/hedger-worker
COPY --from=builder /out/migrator /usr/local/bin/migrator
COPY deploy /workspace/deploy
COPY backend/scripts /workspace/backend/scripts

RUN chmod +x /workspace/deploy/scripts/*.sh

ENTRYPOINT ["/workspace/deploy/scripts/start-backend-service.sh"]
