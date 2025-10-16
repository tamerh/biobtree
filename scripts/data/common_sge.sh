#!/bin/bash

echo ostpe is $OSTYPE

if [[ "$OSTYPE" == "linux-gnu" || "$OSTYPE" == "linux" ]]; then
  GNUSORT="sort"
  echo "Using linux sort"
elif [[ "$OSTYPE" == "darwin"* ]]; then
  GNUSORT="/usr/local/opt/coreutils/libexec/gnubin/sort"
fi


if [[ ! -f biobtree ]]; then

    if [[ "$OSTYPE" == "linux-gnu" || "$OSTYPE" == "linux" ]]; then
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
       sleep 60
    fi

    echo "waiting for the job $1 to be finished"

    while [ true ]
    do
        # Check if job exists in qstat
        QSTAT_RESULT=`qstat -u $(whoami) | grep "$1" | wc -l`

        if [ "$QSTAT_RESULT" == 0  ]
        then
            echo "job $1 is finished"
            break
        fi
        sleep 120
    done
}
