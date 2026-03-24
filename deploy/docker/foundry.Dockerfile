FROM ghcr.io/foundry-rs/foundry:stable AS foundry

FROM debian:bookworm-slim

WORKDIR /workspace

RUN apt-get update \
  && apt-get install -y --no-install-recommends bash ca-certificates python3 \
  && rm -rf /var/lib/apt/lists/*

COPY --from=foundry /usr/local/bin/anvil /usr/local/bin/anvil
COPY --from=foundry /usr/local/bin/cast /usr/local/bin/cast
COPY --from=foundry /usr/local/bin/forge /usr/local/bin/forge
