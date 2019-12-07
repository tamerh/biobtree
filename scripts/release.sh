#!/bin/bash

set -e

cd ..

if [[ "$OSTYPE" == "linux-gnu" ]]; then
    OS="Linux"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    OS="MacOS"
fi

if [ -f biobtree ]; then
    rm biobtree
fi

if [ -f biobtree_*.tar.gz ]; then
    rm -rf biobtree_*.tar.gz
fi

go build

tar -cvzf biobtree_${OS}_64bit.tar.gz biobtree