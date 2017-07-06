#!/bin/sh
goimports -w .
go fmt
go install
#go test
