#!/usr/bin/env bash

Version=$1
Hash="`git rev-parse --short HEAD`"
Tag="`git describe --tags`"

go build  -ldflags "-X main.Version=${Version:-"$Tag"} -X main.Revision=${Hash}" .