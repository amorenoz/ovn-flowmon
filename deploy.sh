#!/bin/bash
set -eu

SCRIPT=`realpath -s $0`
SCRIPT_PATH=`dirname $SCRIPT`

TEMPLATE=${SCRIPT_PATH}/dist/ovs-flowmon.yaml.j2
POD_SPEC=${SCRIPT_PATH}/build/ovs-flowmon.yaml
IMAGE=quay.io/amorenoz/ovs-flowmon
OVN_K8S_NAMESPACE="ovn-kubernetes"
OVN_CENTRAL="unknown"
MODE="ovs"
mkdir -p ${SCRIPT_PATH}/build

usage() {
    echo "$0 [OPTIONS]"
    echo ""
    echo "Deploy the ovs-flowmon to debug the OVS running on a k8s NODE"
    echo ""
    echo "Options"
    echo "  -i IMAGE        Optional: Use a different container image"
    echo "  -l NODE         Listen mode on a NODE"
    echo "  -o NODE         OVN drop-sampling mode on a NODE"
    echo "  -a NAMESPACE    OVN ACL mode on a namespace"
    echo ""
}

error() {
    echo $@ > %2
    exit 1
}
is_command_fail() {
	set +e
	local cmd=$1
	if ! command -v $cmd &> /dev/null; then
		echo "ERROR - command '$cmd' not found, exiting."
		exit 1
	fi
	set -e
}

KUBECTL=""
get_kubectl_binary() {
	set +e
	KUBECTL=$(which oc 2>/dev/null)
	ret_code="$?"
	if [ $ret_code -eq 0 ]; then
		set -e
		return
	fi

	# Note: "local OC_LOCATION=" will set the return code to 0, always
	# so must keep this in public scope
	KUBECTL=$(which kubectl 2>/dev/null)
	ret_code="$?"
	if [ $ret_code -eq 0 ]; then
		set -e
		return
	fi

	echo "ERROR - could not find oc/kubectl binary, exiting."
	exit 1
}

set_ovn_central() {
    IFS=" " read -a ovn_db_hosts <<<"$(${KUBECTL} get ep -n ${OVN_K8S_NAMESPACE} ovnkube-db -o=jsonpath='{range .subsets[0].addresses[*]}{.ip}{" "}')"
    if [[ ${#ovn_db_hosts[@]} == 0 ]]; then
        error "Cannot determine ovn endpoint"
    fi
    OVN_CENTRAL=${ovn_db_hosts[0]}
}

if [ $# -lt 1 ]; then
    usage
    exit 1
fi

while getopts "hl:o:i:a:" opt; do
    case ${opt} in
        h)
            usage
            exit 0
            ;;
        i)
            IMAGE=$OPTARG
            ;;
        o)
            MODE="ovn"
            NODE=$OPTARG
            ;;
        l)
            MODE="ovs"
            NODE=$OPTARG
            ;;
        a)
            MODE="acl"
            NAMESPACE=$OPTARG
            ;;
    esac
done

get_kubectl_binary
is_command_fail pip
is_command_fail grep

# Ensure j2 installed
pip freeze | grep j2cli || pip install j2cli[yaml] --user

# j2 may be installed, but the pip user path might not be added to PATH
is_command_fail j2

if [[ ${MODE} == "ovn" ]]; then
    set_ovn_central
    $KUBECTL get nodes &>/dev/null || "kubectl cannot access cluster node $NODE. Ensure the node name is correct and you have access to the cluster (KUBECONFIG)"

elif [[ ${MODE} == "ovs" ]]; then
    $KUBECTL get nodes &>/dev/null || "kubectl cannot access cluster node $NODE. Ensure the node name is correct and you have access to the cluster (KUBECONFIG)"

elif [[ ${MODE} == "acl" ]]; then
    set_ovn_central
    echo "Configuring ACL sampling for namespace $NAMESPACE"
    ovn_master_pod=$(${KUBECTL} get pods -o name -n ${OVN_K8S_NAMESPACE} | grep ovnkube-master | cut -d "/" -f 2)

    cmd=$(cat <<EOF
acls=\$(echo \$( ovn-nbctl --columns=_uuid find ACL external_ids:namespace=$NAMESPACE | cut -d ":" -f 2 | xargs) \$(ovn-nbctl --columns=_uuid find ACL name=${NAMESPACE}_egressDefaultDeny | cut -d ":" -f 2 | xargs) \$(ovn-nbctl --columns=_uuid find ACL name=${NAMESPACE}_ingressDefaultDeny | cut -d ":" -f 2 | xargs))
for acl in \$acls; do
    ovn-nbctl clear ACL \$acl sample;
    ovn-nbctl --id=@s create Sample probability=65535 collector_set_id=2 obs_domain_id=2 obs_point=\$(( 16#\$(echo \$acl | cut -d "-" -f 1) )) -- set ACL \$acl sample=@s;
done
EOF
)
    echo $cmd
    ${KUBECTL} exec -it -n ${OVN_K8S_NAMESPACE} $ovn_master_pod --container ovn-northd -- bash -c "$cmd"
    NODE=$(${KUBECTL} get nodes -l node-role.kubernetes.io/control-plane="" -o name | cut -d "/" -f 2)
fi


deployment=$(uuidgen | cut -d "-" -f 1)
node=${NODE} \
    image=${IMAGE} \
    ovn_central=${OVN_CENTRAL} \
    mode=${MODE} \
    deployment=${deployment} \
    j2 ${TEMPLATE} -o ${POD_SPEC}

$KUBECTL label nodes --overwrite ${NODE} flowmon=${deployment}

$KUBECTL delete -f ${POD_SPEC} || true
$KUBECTL apply -f ${POD_SPEC}

echo "Waiting for pod to switch to ready condition. This can take a while (timeout 300s) ..."
$KUBECTL wait pod --for=condition=ready --timeout=300s -l app=ovs-flowmon


if [[ ${MODE} == "acl" ]]; then
    listen_addr="$(${KUBECTL} get node $NODE -o jsonpath='{ $.status.addresses[?(@.type=="InternalIP")].address }'):2055"
    (sleep 10 && 
    ovs_pods=$(${KUBECTL} get pods -A -o name | grep ovs-node)
    for pod in $ovs_pods; do
        ${KUBECTL} -n ${OVN_K8S_NAMESPACE} exec -it ${pod} -- ovs-vsctl destroy Flow_Sample_Collector_Set . &>/dev/null || true
        ${KUBECTL} -n ${OVN_K8S_NAMESPACE} exec -it ${pod} -- ovs-vsctl --id=@i get bridge br-int -- --id=@ipfix create IPFIX target=\"${listen_addr}\" -- create Flow_Sample_Collector_Set bridge=@i id=2 ipfix=@ipfix &>/dev/null
    done
    )&
fi

$KUBECTL exec -it ovs-flowmon-${NODE} -- /root/run
