#!/usr/bin/env bash
# Convenience wrapper: run the agent from repo root without installing.
set -e
cd "$(dirname "$0")"
# NOTE: uses repo-local config.yaml, overriding ~/.mini-agent/config.yaml.
# Remove -config to use your installed config instead.
go run ./cmd/mini-agent -config ./config.yaml
