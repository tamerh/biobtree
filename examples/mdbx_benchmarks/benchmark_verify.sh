#!/bin/bash

# Benchmark script that also verifies response content
# LMDB server: localhost:9292
# MDBX server: localhost:9293

echo "========================================="
echo "Biobtree LMDB vs MDBX - Response Verification"
echo "========================================="
echo ""

# Test queries
declare -a QUERIES=(
    "i=9606&m=map(taxparent)"
    "i=9606&m=map(taxparent).map(taxparent).map(taxparent)"
    "i=59201&m=map(taxparent).map(taxparent).map(taxparent)"
)

ITERATIONS=20  # More iterations to reduce variance
WARMUP=5       # Warmup iterations (discarded)

echo "Configuration:"
echo "  Warmup iterations: $WARMUP"
echo "  Measured iterations: $ITERATIONS"
echo ""

# Function to test and capture response
test_query_with_response() {
    local port=$1
    local query=$2
    local iterations=$3
    local warmup=$4

    # Warmup phase
    for ((i=1; i<=$warmup; i++)); do
        curl -s "http://localhost:${port}/ws/map/?${query}" > /dev/null 2>&1
    done

    # Actual measurement
    total_time=0
    local response_file="/tmp/response_${port}.json"

    for ((i=1; i<=$iterations; i++)); do
        time_ms=$(curl -s -o "$response_file" -w "%{time_total}\n" "http://localhost:${port}/ws/map/?${query}" 2>/dev/null)
        total_time=$(echo "$total_time + $time_ms" | bc)
    done

    # Calculate average
    avg_time=$(echo "scale=6; $total_time / $iterations" | bc)
    echo "$avg_time"
}

# Function to compare JSON responses (ignoring whitespace differences)
compare_responses() {
    local lmdb_file=$1
    local mdbx_file=$2

    # Use jq if available for proper JSON comparison, otherwise use diff
    if command -v jq &> /dev/null; then
        diff <(jq -S . "$lmdb_file" 2>/dev/null) <(jq -S . "$mdbx_file" 2>/dev/null) > /dev/null 2>&1
        return $?
    else
        diff "$lmdb_file" "$mdbx_file" > /dev/null 2>&1
        return $?
    fi
}

# Run benchmarks with verification
for idx in "${!QUERIES[@]}"; do
    query="${QUERIES[$idx]}"

    echo "========================================="
    echo "Query $((idx+1)): $query"
    echo "========================================="

    # Test LMDB
    echo "Testing LMDB (port 9292) with warmup..."
    lmdb_time=$(test_query_with_response 9292 "$query" $ITERATIONS $WARMUP)
    lmdb_response="/tmp/response_9292.json"

    # Test MDBX
    echo "Testing MDBX (port 9293) with warmup..."
    mdbx_time=$(test_query_with_response 9293 "$query" $ITERATIONS $WARMUP)
    mdbx_response="/tmp/response_9293.json"

    echo ""
    echo "Results:"
    echo "  LMDB avg: ${lmdb_time}s"
    echo "  MDBX avg: ${mdbx_time}s"

    # Compare responses
    echo ""
    echo "Response Comparison:"
    if compare_responses "$lmdb_response" "$mdbx_response"; then
        echo "  ✓ Responses are IDENTICAL"
    else
        echo "  ✗ WARNING: Responses DIFFER!"
        echo ""
        echo "  LMDB response preview:"
        head -c 500 "$lmdb_response" 2>/dev/null | jq -C . 2>/dev/null || head -c 500 "$lmdb_response"
        echo ""
        echo "  MDBX response preview:"
        head -c 500 "$mdbx_response" 2>/dev/null | jq -C . 2>/dev/null || head -c 500 "$mdbx_response"
        echo ""
    fi

    # Response metadata
    lmdb_size=$(wc -c < "$lmdb_response" 2>/dev/null)
    mdbx_size=$(wc -c < "$mdbx_response" 2>/dev/null)
    echo "  LMDB response size: ${lmdb_size} bytes"
    echo "  MDBX response size: ${mdbx_size} bytes"

    # Calculate improvement
    if [ $(echo "$lmdb_time > 0" | bc) -eq 1 ]; then
        improvement=$(echo "scale=2; (($lmdb_time - $mdbx_time) / $lmdb_time) * 100" | bc)
        if [ $(echo "$improvement > 0" | bc) -eq 1 ]; then
            echo "  Performance: MDBX is ${improvement}% faster"
        else
            improvement_abs=$(echo "$improvement * -1" | bc)
            echo "  Performance: MDBX is ${improvement_abs}% SLOWER"
        fi
    fi

    echo ""
    echo "Full LMDB Response:"
    cat "$lmdb_response" | jq -C . 2>/dev/null || cat "$lmdb_response"
    echo ""
    echo "Full MDBX Response:"
    cat "$mdbx_response" | jq -C . 2>/dev/null || cat "$mdbx_response"
    echo ""
    echo ""
done

# Cleanup
rm -f /tmp/response_*.json

echo "========================================="
echo "Test Complete"
echo "========================================="
