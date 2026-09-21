#!/bin/sh
# Regenerate the grpc stubs. protoc is not assumed to be installed on the
# host; run it in a throwaway golang container instead.
set -e
cd "$(dirname "$0")"
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W 2>/dev/null || pwd)":/api -w /api golang:1.26-bookworm bash -c '
  set -e
  apt-get update -qq >/dev/null && apt-get install -y -qq protobuf-compiler >/dev/null
  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
  export PATH=$PATH:$(go env GOPATH)/bin
  protoc -I=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api.proto
'
