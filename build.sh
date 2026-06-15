#!/usr/bin/sh

MITM_VERSION=$(git describe --tags)

go build -ldflags "-X main.version=${MITM_VERSION}" -o ./bin/mitm-collector-csv-xls main.go

cp bin/mitm-collector-csv-xls ../../scheduler/mitm_scheduler/bin/.