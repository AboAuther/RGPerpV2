#!/bin/sh
set -eu

: "${SERVICE_BIN:?SERVICE_BIN is required}"

if [ -n "${CHAIN_ENV_FILE:-}" ]; then
  echo "waiting for chain env file: ${CHAIN_ENV_FILE}"
  i=0
  while [ "$i" -lt 120 ]; do
    if [ -s "${CHAIN_ENV_FILE}" ]; then
      break
    fi
    i=$((i + 1))
    sleep 1
  done
  if [ ! -s "${CHAIN_ENV_FILE}" ]; then
    echo "chain env file not ready: ${CHAIN_ENV_FILE}" >&2
    exit 1
  fi
  set -a
  # shellcheck disable=SC1090
  . "${CHAIN_ENV_FILE}"
  set +a
  if [ -n "${ETH_RPC_URL_DOCKER:-}" ]; then
    export ETH_RPC_URL="${ETH_RPC_URL_DOCKER}"
  fi
  if [ -n "${ARB_RPC_URL_DOCKER:-}" ]; then
    export ARB_RPC_URL="${ARB_RPC_URL_DOCKER}"
  fi
  if [ -n "${BASE_RPC_URL_DOCKER:-}" ]; then
    export BASE_RPC_URL="${BASE_RPC_URL_DOCKER}"
  fi
fi

exec "/usr/local/bin/${SERVICE_BIN}"
