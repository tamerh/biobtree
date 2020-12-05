#!/bin/bash

if [[ "$OSTYPE" == "linux-gnu" ]]; then
  GNUSORT="sort"
elif [[ "$OSTYPE" == "darwin"* ]]; then
  GNUSORT="/usr/local/opt/coreutils/libexec/gnubin/sort"
fi


if [[ ! -f biobtree ]]; then

    if [[ "$OSTYPE" == "linux-gnu" ]]; then
        OS="Linux"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        OS="MacOS"
    fi

    rm -f biobtree_*_64bit.tar.gz

    bbLatestVersion=`curl -Ls -o /dev/null -w %{url_effective} https://github.com/tamerh/biobtree/releases/latest | rev | cut -d '/' -f 1 | rev`

    curl -OL https://github.com/tamerh/biobtree/releases/download/$bbLatestVersion/biobtree_${OS}_64bit.tar.gz 

    tar -xzvf biobtree_${OS}_64bit.tar.gz

fi


waitJob() { 

    if [[ -z $2 ]]; then
       # sleep is to make sure job is registered
       sleep 600
    fi

    echo "wating the job to be finished"

    while [ true ]
    do  
        BJOBS_RESULT=`bjobs -P $1 | wc -l`

        if [ "$BJOBS_RESULT" == 0  ]
        then
            echo "job is finished"
            break
        fi
        sleep 120
    done
}
