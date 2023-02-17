#!/bin/env bash

dirname=holder
gosum=./go.sum
gomod=./go.mod

if [ ! -f "$gosum" ]
then
    echo "go.sum not exist"
    if [ ! -f "$gomod" ]; then
        echo "go.mod not exist" 
        go mod init $dirname
        go mod tidy
    fi
    # echo "go.sum not exist"
    go mod tidy
       
fi

if [ ! -d "./bin" ]
then
    mkdir bin
fi

main=oss_cache_main
bin=bin/$main
rm $bin

if [ ! -f "$bin" ]; then
    echo "./$bin not exist"
    go build -o bin src/$main.go
else
    echo "./$bin exist"
fi

if [ ! -n "$2" ]; then
    echo "[shell] run $bin"
    echo ""
    ./$bin $1
else
    echo "[shell] run $bin"
    echo ""
    ./$bin $1 $2
fi
