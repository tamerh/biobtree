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


waitJobsForCompletion() {
    sleep 600
    echo "Starting check if jobs are finished with following command--> bjobs -P $@ | wc -l"
    while [ true ]
    do
        BJOBS_RESULT=`bjobs -P $@ | wc -l`
        
        if [ "$BJOBS_RESULT" == 0  ]
        then
            echo "All jobs are now finished"
            break
        fi
        sleep 120
    done
    echo "Check on jobs completed"
}
