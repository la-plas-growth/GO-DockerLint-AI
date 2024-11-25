#!/bin/bash
go build .
./go-docker-ai dockerlint -f Dockerfile          
