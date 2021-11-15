#!/bin/bash

############### Get AWS Info ###############

# Credentials for usage of aws cli
if [ -z "${AWS_ACCESS_KEY_ID}" -o -z "${AWS_SECRET_ACCESS_KEY}" -o -z "${AWS_DEFAULT_REGION}" ]; then
   echo "AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_DEFAULT_REGION must be set via env"
   exit 1
fi

# Allocation ID of elastic IP
if [ -z "$ALLOCATION_ID" ]; then
   echo "Allocation ID of elastic IP must be set via env ALLOCATION_ID"
   exit 1
fi

# Get local instance ID
if [ -z "${INSTANCE_ID}" ]; then
   export INSTANCE_ID=$(curl -s http://instance-data/latest/meta-data/instance-id)
   echo "Get instance ID from aws ${INSTANCE_ID}"
fi

############### Start Keepalived ##############

pkill keepalived

check_file_exit() {
    if [ ! -f $1 ]; then
        echo "$1 is not a file or does not exists"
        exit 1
    fi
}

check_dir_create() {
    if [ ! -d $1 ]; then
        if [ -e $1 ]; then
            echo "$1 is not a directory"
	    exit 1
        fi
        echo "$1 does not exist, create it"
	mkdir -p $1
    fi
}

# cp input config from external
if [ ! -z ${CONFIG_PATH} ]; then
    if [ "${CONFIG_PATH}" == "/etc/keepalived/keepalived.conf" ]; then
        echo "${CONFIG_PATH} is not allowed, change to other path"
	exit 1
    fi
    check_file_exit ${CONFIG_PATH}
    echo "Use input config from ${CONFIG_PATH}"
    cp ${CONFIG_PATH} /etc/keepalived/keepalived.conf
    # make the right permission
    chmod 0644 /etc/keepalived/keepalived.conf
fi

# Log file recording status change
if [ -z "${STATE_LOG_DIR}" ]; then
    STATE_LOG_DIR=/tmp/
    echo "Use default path for state log dir ${STATE_LOG_DIR}"
fi

check_dir_create ${STATE_LOG_DIR}
export STATE_LOG_FILE=$(readlink -f "${STATE_LOG_DIR}")/state.log
echo "Use ${STATE_LOG_FILE} for state log"

# IPs of all keepalived instances must be set
if [ -z "$ALL_PEERS_IP" ]; then
    echo "IPs of all keepalived instances must be set via env ALL_PEERS_IP"
    exit 1
fi

# Local IP must be set from outside
if [ -z "$LOCAL_IP" ]; then
    echo "Local IP must be set via env LOCAL_IP"
    exit 1
fi

if ! echo "$ALL_PEERS_IP" | grep -q "^$LOCAL_IP,\|,$LOCAL_IP,\|,$LOCAL_IP$";then
    echo "Local IP ${LOCAL_IP} is not among peers ${ALL_PEERS_IP}"
    exit 1
fi

echo "Setting local IP to: $LOCAL_IP"
sed -i "s/{{LOCAL_IP}}/${LOCAL_IP}/g" /etc/keepalived/keepalived.conf

# Path to notify script
if [ ! -z "${NOTIFY_MASTER_PATH}" ]; then
    check_file_exit ${NOTIFY_MASTER_PATH}
    cp ${NOTIFY_MASTER_PATH} /etc/keepalived/notify-master-script.sh
    chmod 0755 /etc/keepalived/notify-master-script.sh
fi

if [ ! -z "${NOTIFY_BACKUP_PATH}" ]; then
    check_file_exit ${NOTIFY_BACKUP_PATH}
    cp ${NOTIFY_BACKUP_PATH} /etc/keepalived/notify-backup-script.sh
    chmod 0755 /etc/keepalived/notify-backup-script.sh
fi

# Backend check script
if [ ! -z "${CHECK_SCRIPT_PATH}" ]; then
    check_file_exit ${CHECK_SCRIPT_PATH}
    cp ${CHECK_SCRIPT_PATH} /etc/keepalived/check-script.sh
    chmod 0755 /etc/keepalived/check-script.sh
fi

# Set peers
echo "Setting peers"
IFS=','
read -ra ipsarr <<< "${ALL_PEERS_IP}"

peers=""
for ip in "${ipsarr[@]}"; do
  if [ "${ip}" != "${LOCAL_IP}" ]; then
    echo "${ip}"
    peers+="${ip}\r        "
  fi
done

sed "s/{{PEERS_IP}}/${peers::-10}/g" /etc/keepalived/keepalived.conf | tr '\r' '\n' > /etc/keepalived/tmp.conf
mv /etc/keepalived/tmp.conf /etc/keepalived/keepalived.conf

# Primary NIC must be set
if [ -z "$PRIMARY_NIC" ]; then
    echo "Primary NIC must be set via env PRIMARY_NIC"
    exit 1
fi
echo "Setting primary NIC to: $PRIMARY_NIC"
sed -i "s/{{PRIMARY_NIC}}/${PRIMARY_NIC}/g" /etc/keepalived/keepalived.conf

## Set node priority based on env variable
if [ -z "${NODE_PRIORITY}" ]; then
   NODE_PRIORITY=100
fi

echo "Setting priority to: ${NODE_PRIORITY}"
sed -i "s/{{NODE_PRIORITY}}/${NODE_PRIORITY}/g" /etc/keepalived/keepalived.conf

ROUTER_ID=${HOSTNAME}
echo "Setting Router ID to: $ROUTER_ID"
sed -i "s/{{ROUTER_ID}}/${ROUTER_ID}/g" /etc/keepalived/keepalived.conf

# Make sure we react to these signals by running stop() when we see them - for clean shutdown
# And then exiting
trap "stop; exit 0;" SIGTERM SIGINT

stop()
{
  # We're here because we've seen SIGTERM, likely via a Docker stop command or similar
  # Let's shutdown cleanly
  echo "SIGTERM caught, terminating keepalived process..."
  # Record PIDs
  pid=$(pidof keepalived)
  # Kill them
  kill -TERM $pid > /dev/null 2>&1
  # Wait till they have been killed
  wait $pid
  echo "Terminated."
  exit 0
}

# This loop runs till until we've started up successfully
while true; do

  # Check if Keepalived is running by recording it's PID (if it's not running $pid will be null):
  pid=$(pidof keepalived)

  # If $pid is null, do this to start or restart Keepalived:
  while [ -z "$pid" ]; do
    echo "Displaying resulting /etc/keepalived/keepalived.conf contents..."
    cat /etc/keepalived/keepalived.conf
    echo "Starting Keepalived in the background..."
    keepalived --dont-fork --log-console --log-detail --vrrp &
    # Check if Keepalived is now running by recording it's PID (if it's not running $pid will be null):
    pid=$(pidof keepalived)

    # If $pid is null, startup failed; log the fact and sleep for 2s
    # We'll then automatically loop through and try again
    if [ -z "$pid" ]; then
      echo "Startup of Keepalived failed, sleeping for 2s, then retrying..."
      sleep 2
    fi

  done

  # Break this outer loop once we've started up successfully
  # Otherwise, we'll silently restart and Rancher won't know
  break

done

# Wait until the Keepalived processes stop (for some reason)
wait $pid
echo "The Keepalived process is no longer running, exiting..."
# Exit with an error
exit 1
#courtesy: https://github.com/NeoAssist/docker-keepalived
