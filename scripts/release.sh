#!/bin/bash

cd ..

if [[ "$OSTYPE" == "linux-gnu" ]]; then
    OS="Linux"
elif [[ "$OSTYPE" == "darwin"* ]]; then
    OS="MacOS"
fi

rm biobtree 

rm -rf biobtree_*.tar.gz 

go build

tar -cvzf biobtree_${OS}_64bit.tar.gz biobtree