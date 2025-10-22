#!/bin/bash

# Benchmark script to compare LMDB vs MDBX query performance
# LMDB server: localhost:9292
# MDBX server: localhost:9293

echo "========================================="
echo "Biobtree LMDB vs MDBX Performance Test"
echo "========================================="
echo ""

# Test queries (taxonomy dataset)
declare -a QUERIES=(
    "i=9606&m=map(taxparent)"
    "i=9606&m=map(taxparent).map(taxparent).map(taxparent)"
    "i=59201&m=map(taxparent).map(taxparent).map(taxparent)"
    "i=9606&m=map(taxchild)"
    "i=9606&m=map(taxchild).map(taxchild)"
    "i=562&m=map(taxchild).filter(taxonomy.taxonomic_division==\"BCT\")"
)

# Number of iterations for each query
ITERATIONS=10

# Function to test a single query on a server
test_query() {
    local port=$1
    local query=$2
    local iterations=$3

    total_time=0

    for ((i=1; i<=$iterations; i++)); do
        # Use curl with timing, capture only time_total
        time_ms=$(curl -s -o /dev/null -w "%{time_total}\n" "http://localhost:${port}/ws/map/?${query}" 2>/dev/null)
        total_time=$(echo "$total_time + $time_ms" | bc)
    done

    # Calculate average
    avg_time=$(echo "scale=6; $total_time / $iterations" | bc)
    echo "$avg_time"
}

echo "Running $ITERATIONS iterations per query..."
echo ""

# Results arrays
declare -a LMDB_TIMES
declare -a MDBX_TIMES

# Run benchmarks
for idx in "${!QUERIES[@]}"; do
    query="${QUERIES[$idx]}"

    echo "----------------------------------------"
    echo "Query $((idx+1)): $query"
    echo "----------------------------------------"

    # Test LMDB
    echo -n "Testing LMDB (port 9292)... "
    lmdb_time=$(test_query 9292 "$query" $ITERATIONS)
    LMDB_TIMES[$idx]=$lmdb_time
    echo "${lmdb_time}s avg"

    # Test MDBX
    echo -n "Testing MDBX (port 9293)... "
    mdbx_time=$(test_query 9293 "$query" $ITERATIONS)
    MDBX_TIMES[$idx]=$mdbx_time
    echo "${mdbx_time}s avg"

    # Calculate improvement
    if [ $(echo "$lmdb_time > 0" | bc) -eq 1 ]; then
        improvement=$(echo "scale=2; (($lmdb_time - $mdbx_time) / $lmdb_time) * 100" | bc)
        speedup=$(echo "scale=2; $lmdb_time / $mdbx_time" | bc)
        echo "MDBX improvement: ${improvement}% faster (${speedup}x speedup)"
    fi
    echo ""
done

# Summary
echo "========================================="
echo "SUMMARY"
echo "========================================="
echo ""

total_lmdb=0
total_mdbx=0

for idx in "${!QUERIES[@]}"; do
    lmdb=${LMDB_TIMES[$idx]}
    mdbx=${MDBX_TIMES[$idx]}
    total_lmdb=$(echo "$total_lmdb + $lmdb" | bc)
    total_mdbx=$(echo "$total_mdbx + $mdbx" | bc)

    echo "Query $((idx+1)):"
    echo "  LMDB: ${lmdb}s"
    echo "  MDBX: ${mdbx}s"
done

echo ""
echo "----------------------------------------"
echo "OVERALL RESULTS:"
echo "  Total LMDB time: ${total_lmdb}s"
echo "  Total MDBX time: ${total_mdbx}s"

if [ $(echo "$total_lmdb > 0" | bc) -eq 1 ]; then
    overall_improvement=$(echo "scale=2; (($total_lmdb - $total_mdbx) / $total_lmdb) * 100" | bc)
    overall_speedup=$(echo "scale=2; $total_lmdb / $total_mdbx" | bc)
    echo "  Overall MDBX improvement: ${overall_improvement}% faster"
    echo "  Overall speedup: ${overall_speedup}x"
fi

echo "========================================="
