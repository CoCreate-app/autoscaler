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

logf "$INSTANCE_ID becomes master"
logf "Allocation ID is $ALLOCATION_ID"

ASSOCIATION_ID=`aws ec2 describe-addresses --allocation-id $ALLOCATION_ID | /usr/bin/python -c 'import json,sys;obj=json.load(sys.stdin); print obj["Addresses"][0].get("AssociationId", "")'`
checkerr "Get current association ID"

if [ -z "$ASSOCIATION_ID" ]; then
    logf "Elastic IP is not associated, associate to $INSTANCE_ID"
    aws ec2 associate-address --allocation-id $ALLOCATION_ID --instance-id $INSTANCE_ID
    checkerr "Associate to $INSTANCE_ID"
    exit 0
fi

EIP_INSTANCE=`aws ec2 describe-addresses --allocation-id $ALLOCATION_ID | /usr/bin/python -c 'import json,sys;obj=json.load(sys.stdin); print obj["Addresses"][0]["InstanceId"]'`
checkerr "Get current associated instance ID"

logf "Current association ID: $ASSOCIATION_ID, associated instance ID: $EIP_INSTANCE"

if [ "$INSTANCE_ID" != "$EIP_INSTANCE" ]; then
    aws ec2 disassociate-address --association-id $ASSOCIATION_ID
    checkerr "Disassociate $ASSOCIATION_ID"

    aws ec2 associate-address --allocation-id $ALLOCATION_ID --instance-id $INSTANCE_ID
    checkerr "Associate to $INSTANCE_ID"

    logf "Assocaite elastic IP to $INSTANCE_ID"
fi

exit 0
