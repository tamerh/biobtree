#!/bin/bash

# gRPC Benchmark script to compare LMDB vs MDBX
# LMDB gRPC: localhost:7777
# MDBX gRPC: localhost:7778

echo "========================================="
echo "Biobtree LMDB vs MDBX - gRPC Performance Test"
echo "========================================="
echo ""

# Find grpcurl (check PATH, then common Go locations)
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
    echo "Then add ~/go/bin to your PATH or run this script with full path"
    exit 1
fi

echo "Using grpcurl: $GRPCURL"
echo ""

# Find proto files
PROTO_DIR="pbuf"
if [ ! -f "$PROTO_DIR/app.proto" ]; then
    echo "ERROR: Proto files not found in $PROTO_DIR/"
    exit 1
fi

# Test queries (same as HTTP tests but in gRPC format)
declare -a QUERIES=(
    '{"terms":["9606"],"query":"map(taxparent)"}'
    '{"terms":["9606"],"query":"map(taxparent).map(taxparent).map(taxparent)"}'
    '{"terms":["59201"],"query":"map(taxparent).map(taxparent).map(taxparent)"}'
    '{"terms":["9606"],"query":"map(taxchild)"}'
    '{"terms":["9606"],"query":"map(taxchild).map(taxchild)"}'
    '{"terms":["562"],"query":"map(taxchild).filter(taxonomy.taxonomic_division==\"BCT\")"}'
)

declare -a QUERY_NAMES=(
    "i=9606&m=map(taxparent)"
    "i=9606&m=map(taxparent).map(taxparent).map(taxparent)"
    "i=59201&m=map(taxparent).map(taxparent).map(taxparent)"
    "i=9606&m=map(taxchild)"
    "i=9606&m=map(taxchild).map(taxchild)"
    "i=562&m=map(taxchild).filter(taxonomy.taxonomic_division==\"BCT\")"
)

ITERATIONS=10
WARMUP=3

echo "Configuration:"
echo "  Warmup iterations: $WARMUP"
echo "  Measured iterations: $ITERATIONS"
echo ""

# Function to test gRPC query
test_grpc_query() {
    local port=$1
    local query=$2
    local iterations=$3
    local warmup=$4

    # Warmup phase
    for ((i=1; i<=$warmup; i++)); do
        $GRPCURL -plaintext -proto pbuf/app.proto -import-path pbuf -d "$query" localhost:${port} pbuf.BiobtreeService/Mapping > /dev/null 2>&1
    done

    # Actual measurement
    total_time=0

    for ((i=1; i<=$iterations; i++)); do
        start=$(date +%s.%N)
        $GRPCURL -plaintext -proto pbuf/app.proto -import-path pbuf -d "$query" localhost:${port} pbuf.BiobtreeService/Mapping > /dev/null 2>&1
        end=$(date +%s.%N)
        time_ms=$(echo "$end - $start" | bc)
        total_time=$(echo "$total_time + $time_ms" | bc)
    done

    # Calculate average
    avg_time=$(echo "scale=6; $total_time / $iterations" | bc)
    echo "$avg_time"
}

# Results arrays
declare -a LMDB_TIMES
declare -a MDBX_TIMES

# Run benchmarks
for idx in "${!QUERIES[@]}"; do
    query="${QUERIES[$idx]}"
    query_name="${QUERY_NAMES[$idx]}"

    echo "----------------------------------------"
    echo "Query $((idx+1)): $query_name"
    echo "----------------------------------------"

    # Test LMDB
    echo -n "Testing LMDB (gRPC port 7777)... "
    lmdb_time=$(test_grpc_query 7777 "$query" $ITERATIONS $WARMUP)
    LMDB_TIMES[$idx]=$lmdb_time
    echo "${lmdb_time}s avg"

    # Test MDBX
    echo -n "Testing MDBX (gRPC port 7778)... "
    mdbx_time=$(test_grpc_query 7778 "$query" $ITERATIONS $WARMUP)
    MDBX_TIMES[$idx]=$mdbx_time
    echo "${mdbx_time}s avg"

    # Calculate improvement
    if [ $(echo "$lmdb_time > 0" | bc) -eq 1 ]; then
        improvement=$(echo "scale=2; (($lmdb_time - $mdbx_time) / $lmdb_time) * 100" | bc)
        if [ $(echo "$improvement > 0" | bc) -eq 1 ]; then
            echo "MDBX improvement: ${improvement}% faster"
        else
            improvement_abs=$(echo "$improvement * -1" | bc)
            echo "MDBX is ${improvement_abs}% SLOWER"
        fi
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
    if [ $(echo "$overall_improvement > 0" | bc) -eq 1 ]; then
        echo "  Overall MDBX improvement: ${overall_improvement}% faster"
        echo "  Overall speedup: ${overall_speedup}x"
    else
        overall_improvement_abs=$(echo "$overall_improvement * -1" | bc)
        echo "  MDBX is ${overall_improvement_abs}% slower overall"
        echo "  Speedup: ${overall_speedup}x"
    fi
fi

echo "========================================="
