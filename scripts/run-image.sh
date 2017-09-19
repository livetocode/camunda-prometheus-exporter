#!/bin/sh
SCRIPT_FOLDER="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source $SCRIPT_FOLDER/config.sh

if [[ "$(docker images -q $REPO_ORG/$REPO_NAME 2> /dev/null)" == "" ]]; then
    echo "Could not find image $REPO_ORG/$REPO_NAME. You must call build-image.sh first!"
else
    docker run -p 8080:8080 $REPO_ORG/$REPO_NAME $*
fi
