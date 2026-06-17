#!/bin/bash
set -euo pipefail

if [[ $# -ne 5 ]]; then
    echo "Usage: $0 <registry_ip> <user> <manifest_index> <image1> <image2>" >&2
    exit 1
fi

IP=$1
USER=$2
INDEX=$3
IMAGE1=$4
IMAGE2=$5

docker login "$IP" -u "$USER" --password-stdin
docker manifest create "$INDEX" "$IMAGE1" "$IMAGE2"
docker manifest push "$INDEX"
