OUTPUT=./build
REPO_ORG=livetocode
REPO_NAME=camunda-prometheus-exporter
IMG_VERSION=`cat charts/camunda-prometheus-exporter/Chart.yaml |grep version: | awk '{print $2}'`

if [ -z "$IMG_VERSION" ]; then
    echo "Could not extract Chart version"
    exit 1
fi
