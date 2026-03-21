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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/migrator ./cmd/migrator

FROM debian:bookworm-slim

WORKDIR /workspace

COPY --from=builder /out/api-server /usr/local/bin/api-server
COPY --from=builder /out/indexer /usr/local/bin/indexer
COPY --from=builder /out/migrator /usr/local/bin/migrator
COPY deploy /workspace/deploy

RUN chmod +x /workspace/deploy/scripts/*.sh

ENTRYPOINT ["/workspace/deploy/scripts/start-backend-service.sh"]
