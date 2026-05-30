#!/usr/bin/env bash
# Convenience wrapper: run the agent from repo root without installing.
set -e
cd "$(dirname "$0")"
go run ./cmd/mini-agent -config ./config.yaml
