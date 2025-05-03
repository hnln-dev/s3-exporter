#!/bin/bash

set -euo pipefail

export CGO_ENABLED=0

while IFS=/ read -r GOOS GOARCH; do
  echo Building s3-exporter for $GOOS on $GOARCH...
  GOOS=$GOOS GOARCH=$GOARCH go build -a -trimpath \
    -ldflags '-buildid= -extldflags "-static" -X main.Version="'"$CI_COMMIT_REF_NAME"'"' \
    -o bin/s3-exporter.$GOOS-$GOARCH ./cmd/s3-exporter
done << EOF
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
aix/ppc64
EOF
