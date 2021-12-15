

#!/bin/bash

# uploads large builtin files to biobtree-conf github release

set -e


if [[ -z $1 ]]; then
    echo "release version is required"
    exit 1
fi

if [[ -z $2 ]]; then
    echo "builtin files folder location is required"
    exit 1
fi

if [[ -z $3 ]]; then
    echo "github release id is required"
    # curl -H "Accept: application/vnd.github.v3+json"   https://api.github.com/repos/tamerh/biobtree-conf/releases
    exit 1
fi

BUILTINPATH=$1
GITHUB_TOKEN=$2 #TODO read some other way
GITHUB_RELEASE=$3

cd $BUILTINPATH

# todo github rejects but better to exclude set4r here
for filename in *.gz; do
 
    curl -H "Authorization: token $GITHUB_TOKEN" -H "Content-Type: application/gzip" --data-binary @$filename "https://uploads.github.com/repos/tamerh/biobtree-conf/releases/$GITHUB_RELEASE/assets?name=$filename"

     echo "curl -H 'Authorization: token $GITHUB_TOKEN' \
        -H 'Content-Type: application/gzip' \
       --data-binary @$filename \
        'https://uploads.github.com/repos/tamerh/biobtree-conf/releases/$GITHUB_RELEASE/assets?name=$filename'"

   echo $filename
done

echo "All done."

