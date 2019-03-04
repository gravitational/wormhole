#!/bin/sh
set -e
set -x

echo "Upgrading wormhole"
echo "Changeset: $RIG_CHANGESET"

if rig status $RIG_CHANGESET --retry-attempts=1 --retry-period=1s --quiet; then exit 0; fi

rig upsert -f /var/lib/gravity/resources/dns.yaml
rig status $RIG_CHANGESET --retry-attempts=120 --retry-period=1s --debug
rig freeze