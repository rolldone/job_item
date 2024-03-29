#!/bin/bash


CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on \
 GOOS=linux go build main.go