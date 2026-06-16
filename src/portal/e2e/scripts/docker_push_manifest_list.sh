#!/bin/bash
set -euo pipefail

if [[ $# -ne 6 ]]; then
    echo "Usage: $0 <registry_ip> <user> <password> <manifest_index> <image1> <image2>" >&2
    exit 1
fi

IP=$1
USER=$2
PASSWORD=$3
INDEX=$4
IMAGE1=$5
IMAGE2=$6

printf '%s\n' "$PASSWORD" | docker login "$IP" -u "$USER" --password-stdin
docker manifest create "$INDEX" "$IMAGE1" "$IMAGE2"
docker manifest push "$INDEX"
