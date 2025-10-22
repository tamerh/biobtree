#!/bin/bash

# Simple script to test a single gRPC query and compare responses

# Find grpcurl
GRPCURL=""
if command -v grpcurl &> /dev/null; then
    GRPCURL="grpcurl"
elif [ -x "$HOME/go/bin/grpcurl" ]; then
    GRPCURL="$HOME/go/bin/grpcurl"
elif [ -x "~/go/bin/grpcurl" ]; then
    GRPCURL="~/go/bin/grpcurl"
elif [ -x "$GOPATH/bin/grpcurl" ]; then
    GRPCURL="$GOPATH/bin/grpcurl"
else
    echo "ERROR: grpcurl is not installed or not found"
    echo "Install with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    exit 1
fi

echo "Using grpcurl: $GRPCURL"

QUERY='{"terms":["9606"],"query":"map(taxparent)"}'

echo "========================================="
echo "Testing gRPC Query: i=9606&m=map(taxparent)"
echo "========================================="
echo ""

echo "LMDB Response (port 7777):"
echo "----------------------------"
$GRPCURL -plaintext -proto pbuf/app.proto -import-path pbuf -d "$QUERY" localhost:7777 pbuf.BiobtreeService/Mapping | tee /tmp/grpc_lmdb.json
echo ""

echo "MDBX Response (port 7778):"
echo "----------------------------"
$GRPCURL -plaintext -proto pbuf/app.proto -import-path pbuf -d "$QUERY" localhost:7778 pbuf.BiobtreeService/Mapping | tee /tmp/grpc_mdbx.json
echo ""

echo "Comparison:"
echo "----------------------------"
if diff /tmp/grpc_lmdb.json /tmp/grpc_mdbx.json > /dev/null 2>&1; then
    echo "✓ Responses are IDENTICAL"
else
    echo "✗ Responses DIFFER!"
    echo ""
    echo "Differences:"
    diff -u /tmp/grpc_lmdb.json /tmp/grpc_mdbx.json
fi
