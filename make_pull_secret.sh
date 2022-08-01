#!/bin/bash

if [ -z "$1" ]; then
    CONFIGFILE="$HOME/.docker/config.json"
else
    CONFIGFILE="$1"
fi

kubectl create secret generic pull-secret --from-file=.dockerconfigjson="$CONFIGFILE" --type=kubernetes.io/dockerconfigjson
