#!/usr/bin/sh

MITM_VERSION=$(git describe --tags)

CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${MITM_VERSION}" -o ./bin/mitm-collector-csv-xls main.go

cp bin/mitm-collector-csv-xls ../../scheduler/mitm_scheduler/bin/.