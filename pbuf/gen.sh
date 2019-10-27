#!/bin/bash

#requirments
# go get -u github.com/pquerna/ffjson
# go get -u github.com/golang/protobuf/proto
# go get -u github.com/golang/protobuf/protoc-gen-go
# go get -u google.golang.org/grpc

mkdir -p biobtree
rm biobtree/*

rm -rf ffjson-inception*
rm *.go
rm *.go

protoc -I=. --go_out=plugins=grpc:biobtree app.proto
protoc -I=. --go_out=. attr.proto 

mv biobtree/* .

ffjson attr.pb.go 
ffjson app.pb.go

rmdir biobtree