# ovn-flowmon
Terminal-based NetFlow/sFlow/IPFIX flow monitoring for OpenvSwitch

Implemented using [tview](https://github.com/rivo/tview) (for graphics) and [goflow2](https://github.com/netsampler/goflow2) (for IPFIX collection).

** Important Note: the word "flow" here means an IPFIX Flow Record not an OpenFlow flow or a datapath flow. For a more verbose definition, [please read the RFC](https://datatracker.ietf.org/doc/html/rfc7011#section-2)


## Building

    make

## Usage

    ovs-flowmon help

### OVS mode: Built-in OvS Exporter configuration
There is a built-in OvS configuration module. You can connect to a running OvS by using the `ovs` subcommand.
For locally running OvS:

    ./build/ovs-flowmon ovs unix:/var/run/openvswitch/db.sock

For a remote OvS, you can expose the OVSDB on a specific port:

    Where the ovsdb-server is running, execute:

    ovs-appctl -t ovsdb-server ovsdb-server/add-remote ptcp:6999

    Then run `ovs-flowmon` with:

    ./build/ovs-flowmon ovs tcp:172.18.0.5:6999

In the UI, you'll see a button to Start, Stop and (re)Configure the OvS IPFIX Exporter.

### Listen mode: Manual configuration of the exporter
If you are using an exporter other than OvS or it is not trivial how the exporter will access the collector, you can start the Flow Monitor and manually configure the exporter.

In that case, start the ovs-flowmon daemon without any option

    ./build/ovs-flowmon listen

The collector will listen on ::2055. Now you can configure your exporter to export flows to that address. For OvS, you can run something like:

    ovs-vsctl -- set Bridge br-int ipfix=@i -- --id=@i create IPFIX targets=\"OVS_FLOWMON_IP:2055\" sampling=200

Where OVS_FLOWMON_IP is the IP Address where ovs-flowmon can be reached.


### OVN mode (Experimental): Sample OVN drops
OVN mode interacts with a running OVN cluster and configures drop-sampling mode. Also, it can add the correspondent per-flow IPFIX sampling configuration a running OvS.

The result is the ability to visualize the packets that are dropped by OVN as well as the LogicalFlows, stages, etc that caused the drop.
Example:

     ./build/ovs-flowmon ovn --nbdb tcp:172.18.0.4:6641 --sbdb  tcp:172.18.0.4:6642 --ovs unix:/var/run/openvswitch/db.sock


## Aggregates
The flow table supports aggregation. Aggregation is a useful tool to visualize exactly the flows you're looking for.

If you add a column to the aggregates it means it gets wildcarded. If two flows have the same value in the rest of the columns they will get collapsed into one aggregated flow and their Bytes/Packets/Rate will be summed up.

For example, imagine you have the following flows:

| SrcAddr|  DstAddr|  SrcPort| DstPort| RATE (kbps)|
|--------|---------|---------|--------|------------|
|  IP_A  |  IP_B   |   65432 |  80    |      10    |
|  IP_B  |  IP_A   |   80    |  65432 |      10    |
|  IP_C  |  IP_B   |   76543 |  80    |      5     |
|  IP_A  |  IP_B   |   65433 |  80    |      3     |
|  IP_B  |  IP_A   |   80    |  65433 |      3     |



Clearly IP_A and IP_B are exchanging packets. If you just want to see the total IP traffic that the hosts exchange, you can add SrcPort and DstPort to the aggregate and you'll get:

| SrcAddr|  DstAddr|  SrcPort| DstPort| RATE (kbps)|
|--------|---------|---------|--------|------------|
|  IP_A  |  IP_B   |   *     |  *     |      13    |
|  IP_B  |  IP_A   |   *     |  *     |      13    |
|  IP_C  |  IP_B   |   *     |  *     |      5     |



If you just want to see how much traffic is hitting IP_B you can add SrcAddr to the aggregate and get:

| SrcAddr|  DstAddr|  SrcPort| DstPort| RATE (kbps)|
|--------|---------|---------|--------|------------|
|  *     |  IP_B   |   *     |  *     |      18    |
|  *     |  IP_A   |   *     |  *     |      13    |


# Deployment

### Kubernetes / Openshift

Use the deploy script to deploy the monitor in the node you want to monitor:

    ./deploy.sh [ -i NON_DEFAULT_IMAGE ] [-o] NODE_NAME

#### OVN mode

To deploy the flow monitor in OVN drop-sampling mode, run

    ./deploy.sh -o NODE_NAME


To clean the deployment simply delete the ovs-flowmon pod:

    kubectl delete pod -l app=ovs-flowmon


### Podman

Use podman to start the monitor container on the server where OvS is running:

    podman run --detach  --network=host --volume /var/run/openvswitch:/var/run/openvswitch --name ovs-flowmon quay.io/amorenoz/ovs-flowmon

Run the monitoring app:

    podman exec -it ovs-flowmon /root/run


## Demo
[![asciicast](https://asciinema.org/a/440615.svg)](https://asciinema.org/a/440615)
