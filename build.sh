#!/bin/bash

go test || exit 1
go build

me=$(whoami)@$(hostname -f)
gitver=$(git rev-parse --short HEAD)
ver="2.1-${gitver}"

env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-X 'main.Version=v${ver}' -X 'main.Builder=${me}' -X 'main.BuildTime=$(date)'" \
    -o "icbm-linux-amd64"
