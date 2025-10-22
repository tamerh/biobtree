#!/bin/bash

echo "========================================="
echo "Testing Build Tags"
echo "========================================="
echo ""

# Build LMDB-only version
echo "1. Building LMDB-only version (default)..."
go build -o biobtree_lmdb_only 2>&1 | grep -v "warning:" | grep -v "^$" || echo "   ✓ Build successful"
SIZE_LMDB=$(ls -lh biobtree_lmdb_only 2>/dev/null | awk '{print $5}')
echo "   Binary size: $SIZE_LMDB"
echo ""

# Build with MDBX support
echo "2. Building with MDBX support (-tags mdbx)..."
go build -tags mdbx -o biobtree_with_mdbx 2>&1 | grep -v "warning:" | grep -v "^$" || echo "   ✓ Build successful"
SIZE_MDBX=$(ls -lh biobtree_with_mdbx 2>/dev/null | awk '{print $5}')
echo "   Binary size: $SIZE_MDBX"
echo ""

echo "========================================="
echo "Summary:"
echo "  LMDB-only:     $SIZE_LMDB"
echo "  With MDBX:     $SIZE_MDBX"
echo ""
echo "Size increase with MDBX support:"
SIZE_LMDB_BYTES=$(ls -l biobtree_lmdb_only 2>/dev/null | awk '{print $5}')
SIZE_MDBX_BYTES=$(ls -l biobtree_with_mdbx 2>/dev/null | awk '{print $5}')
DIFF=$((SIZE_MDBX_BYTES - SIZE_LMDB_BYTES))
DIFF_MB=$((DIFF / 1024 / 1024))
echo "  +${DIFF_MB}MB"
echo "========================================="

# Cleanup
rm -f biobtree_lmdb_only biobtree_with_mdbx
