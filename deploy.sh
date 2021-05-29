#!/bin/bash
# shellcheck disable=SC2154

set -e  # exit on error
trap '_last_command=$_current_command; _current_command=$BASH_COMMAND' debug
trap 'echo "\"${_last_command}\" command failed with exit code $?."' exit

./build.sh
scp icbm-linux-amd64 icbm.api.evq.io:
ssh api.evq.io "sudo ./icbm-linux-amd64 -install"

./test-icbm.sh

trap '' exit
exit 0
