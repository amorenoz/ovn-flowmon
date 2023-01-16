#!/bin/bash
set -eux

OVS_FLOWMON_MODE=${OVS_FLOWMON_MODE:-ovs}
OVS_TARGET=${OVS_TARGET:-unix:/var/run/openvswitch/db.sock}

case ${OVS_FLOWMON_MODE} in
    ovs)
        cmd="ovs-flowmon ovs ${OVS_TARGET}"
        ;;
    ovn)
        cmd="ovs-flowmon ovn --nbdb tcp:${OVN_CENTRAL}:6641 --sbdb tcp:${OVN_CENTRAL}:6642 --ovs ${OVS_TARGET}"
        ;;
    *)
        echo "unkown mode"
        exit 1
esac

cat > /root/run <<EOF 
#!/bin/sh

exec $cmd

EOF

chmod +x /root/run
exec sleep infinity 

