#!/bin/bash

# Simple script to compare a single query response from both servers

if [ -z "$1" ]; then
    echo "Usage: $0 <query>"
    echo "Example: $0 'i=9606&m=map(taxparent)'"
    exit 1
fi

QUERY="$1"

echo "========================================="
echo "Comparing Query: $QUERY"
echo "========================================="
echo ""

echo "LMDB Response (port 9292):"
echo "----------------------------"
curl -s "http://localhost:9292/ws/map/?${QUERY}" | tee /tmp/lmdb_resp.json | jq . 2>/dev/null || cat /tmp/lmdb_resp.json
echo ""
echo ""

echo "MDBX Response (port 9293):"
echo "----------------------------"
curl -s "http://localhost:9293/ws/map/?${QUERY}" | tee /tmp/mdbx_resp.json | jq . 2>/dev/null || cat /tmp/mdbx_resp.json
echo ""
echo ""

echo "Comparison:"
echo "----------------------------"
if diff /tmp/lmdb_resp.json /tmp/mdbx_resp.json > /dev/null 2>&1; then
    echo "✓ Responses are IDENTICAL (byte-for-byte)"
elif command -v jq &> /dev/null && diff <(jq -S . /tmp/lmdb_resp.json 2>/dev/null) <(jq -S . /tmp/mdbx_resp.json 2>/dev/null) > /dev/null 2>&1; then
    echo "✓ Responses are IDENTICAL (JSON content, different formatting)"
else
    echo "✗ Responses DIFFER!"
    echo ""
    echo "Differences:"
    diff -u /tmp/lmdb_resp.json /tmp/mdbx_resp.json || echo "(diff failed)"
fi

echo ""
echo "Sizes:"
echo "  LMDB: $(wc -c < /tmp/lmdb_resp.json) bytes"
echo "  MDBX: $(wc -c < /tmp/mdbx_resp.json) bytes"
