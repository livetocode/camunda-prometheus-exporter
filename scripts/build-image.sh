#!/bin/sh
# Inspired by https://blog.alexellis.io/mutli-stage-docker-builds/
SCRIPT_FOLDER="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source $SCRIPT_FOLDER/config.sh

rm -rf $OUTPUT
mkdir $OUTPUT
docker rm -f extract-$REPO_NAME 2>/dev/null 

docker build -t $REPO_ORG/$REPO_NAME:build . -f ./Dockerfile.build
if [ $? -ne 0 ]; then
    echo "Could not build compiler image"
    exit 1
fi

docker create --name extract-$REPO_NAME $REPO_ORG/$REPO_NAME:build
docker cp extract-$REPO_NAME:/go/src/$REPO_ORG/$REPO_NAME/app $OUTPUT/app
docker rm -f extract-$REPO_NAME
if [ ! -f $OUTPUT/app ]; then
    echo "Could not extract executable"
    exit 1
fi

docker build --no-cache -t $REPO_ORG/$REPO_NAME .
if [ $? -ne 0 ]; then
    echo "Could not build final image"
    exit 1
fi

