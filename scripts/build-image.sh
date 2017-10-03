#!/bin/sh
# Inspired by https://blog.alexellis.io/mutli-stage-docker-builds/
SCRIPT_FOLDER="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source $SCRIPT_FOLDER/config.sh

rm -rf $OUTPUT
mkdir $OUTPUT
docker rm -f extract-$REPO_NAME 2>/dev/null

docker build -t $REPO_ORG/$REPO_NAME:build . -f ./Dockerfile
if [ $? -ne 0 ]; then
    echo "Could not build compiler image"
    exit 1
fi
