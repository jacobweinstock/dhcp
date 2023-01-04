#!/usr/bin/env bash

# why this script you may ask?
# running `go run main.go server | jq` from the make target
# and then sending ctrl-c causes a broken pipe error and final log
# messages sent to stdout don't get displayed. This happens because
# ctrl-c kills both the `go run` and `jq` commands. And with the `jq` command
# dead there's is nothing on the other end of the pipe to read the stdout from `go run`

# trap ctrl-c and call ctrl_c()
trap ctrl_c INT

function ctrl_c() {
    kill "${RUN_PID}"
    kill "$(<pid)"
    rm -rf temp.in pid
    exit 0
}

(./bin/dhcp-linux -file-path ./backend/file/testdata/example.yaml -handlers reservation > temp.in 2>&1) &
RUN_PID=$!
sleep .5
ps -p "$RUN_PID" > /dev/null
if [ $? -ne 0 ]; then
    cat temp.in | jq
    rm -rf temp.in pid
    exit 1
fi
echo "Started dhcp-linux with pid: ${RUN_PID}"
( tail -F temp.in & echo $! >&3 ) 3>pid | jq -R 'fromjson? | select(type == "object")' &
cat -