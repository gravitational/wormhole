#!/bin/sh
set -e
set -x

echo "Reverting changeset $RIG_CHANGESET"
rig revert
rig cs delete --force -c cs/$RIG_CHANGESET