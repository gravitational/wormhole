#!/bin/sh
set -e
set -x

echo "Installing wormhole"
echo "Changeset: $RIG_CHANGESET"

rig upsert -f /gravity/wormhole.yaml