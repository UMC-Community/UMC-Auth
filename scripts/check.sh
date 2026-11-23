#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

if [ -n "$(gofmt -l cmd)" ]; then
	echo "gofmt found unformatted files:"
	gofmt -l cmd
	exit 1
fi

go vet ./...
go test ./...
