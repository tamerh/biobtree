#!/bin/bash

# Test Query 6 specifically - the filter query that should take 3 seconds

echo "========================================="
echo "Testing Query 6 (Filter Query)"
echo "i=562&m=map(taxchild).filter(taxonomy.taxonomic_division==\"BCT\")"
echo "========================================="
echo ""

echo "This query should take ~3 seconds (CPU-bound filter evaluation)"
echo ""

echo "REST API (LMDB - port 9292):"
echo "----------------------------"
time curl -s "http://localhost:9292/ws/map/?i=562&m=map(taxchild).filter(taxonomy.taxonomic_division==\"BCT\")" -o /tmp/rest_q6.json
echo "Response size: $(wc -c < /tmp/rest_q6.json) bytes"
echo ""

if [ -x "$HOME/go/bin/grpcurl" ]; then
    GRPCURL="$HOME/go/bin/grpcurl"

    echo "gRPC API (LMDB - port 7777):"
    echo "----------------------------"
    QUERY='{"terms":["562"],"query":"map(taxchild).filter(taxonomy.taxonomic_division==\"BCT\")"}'
    time $GRPCURL -plaintext -proto pbuf/app.proto -import-path pbuf -d "$QUERY" localhost:7777 pbuf.BiobtreeService/Mapping -o /tmp/grpc_q6.json 2>&1 | grep -v "^$"
    echo "Response size: $(wc -c < /tmp/grpc_q6.json) bytes"
    echo ""

    echo "Comparison:"
    echo "----------------------------"
    echo "REST response size:  $(wc -c < /tmp/rest_q6.json) bytes"
    echo "gRPC response size:  $(wc -c < /tmp/grpc_q6.json) bytes"

    rest_lines=$(cat /tmp/rest_q6.json | grep -o '"identifier"' | wc -l)
    grpc_lines=$(cat /tmp/grpc_q6.json | grep -o '"identifier"' | wc -l)

    echo "REST result count:   ~$rest_lines results"
    echo "gRPC result count:   ~$grpc_lines results"

    if [ "$rest_lines" != "$grpc_lines" ]; then
        echo ""
        echo "⚠️  WARNING: Result counts differ! gRPC might not be filtering correctly."
    fi
else
    echo "grpcurl not found, skipping gRPC test"
fi
