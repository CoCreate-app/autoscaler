#!/bin/bash

touch $STATE_LOG_FILE

logf() {
    echo "$(date) * * * * $*" >> $STATE_LOG_FILE
}

checkerr() {
    if [ $? -ne 0 ]; then
        logf "Error in $1"
        exit 1
    fi
}

logf "$INSTANCE_ID becomes backup or fault"
logf "Allocation ID is $ALLOCATION_ID"

exit 0
