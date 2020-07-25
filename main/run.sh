#!/bin/bash
if [ $1 = "start" ]; then
    for (( i=0; i<4; i++ )) do
        ./main server $i --tag=hotstuff-server &
    done
    for (( i=0; i<3; i++ )) do
        ./main client $i --tag=hotstuff-client &
    done
elif [ $1 = "kill" ]; then
    pgrep -f hotstuff-server | xargs kill
    pgrep -f hotstuff-client | xargs kill
elif [ $1 = "build" ]; then
    go build
fi