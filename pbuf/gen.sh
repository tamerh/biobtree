#!/bin/bash

export GOPATH=/Users/tgur/go

currentdir=`pwd`

mkdir -p $GOPATH/src/biobtree/pbuf
mkdir -p biobtree
rm biobtree/*
rm attr.pb.go
rm attr.pb_ffjson.go
protoc -I=. --go_out=plugins=grpc:biobtree app.proto
protoc -I=. --go_out=. attr.proto 
mv biobtree/* .

mv attr.pb.go $GOPATH/src/biobtree/pbuf
mv app.pb.go $GOPATH/src/biobtree/pbuf

cd $GOPATH/src/biobtree/pbuf/
ffjson attr.pb.go 
ffjson app.pb.go

cd $currentdir
mv $GOPATH/src/biobtree/pbuf/* .

rmdir biobtree