#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
go run ./cmd/mini-agent -config ./config.yaml
