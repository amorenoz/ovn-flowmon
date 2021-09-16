package flowmon

import (
	"encoding/binary"
	"fmt"
	"net"
	"reflect"

	flowmessage "github.com/netsampler/goflow2/pb"
)

// FlowDirection is an enum for Flow directions
type FlowDirection uint32

const (
	FlowDirectionIngress FlowDirection = 0x0
	FlowDirectionEgress  FlowDirection = 0x1
)

func (fd FlowDirection) String() string {
	switch fd {
	case FlowDirectionIngress:
		return "INGRESS"
	case FlowDirectionEgress:
		return "EGRESS"
	default:
		return "unknown"
	}
}

// Etype is an enum for EtherTypes
type Etype uint32

const (
	EtypeIPv4 Etype = 0x800
	EtypeARP  Etype = 0x806
	EtypeIPv6 Etype = 0x86DD
)

func (e Etype) String() string {
	switch e {
	case EtypeIPv4:
		return "IPv4"
	case EtypeIPv6:
		return "IPv6"
	case EtypeARP:
		return "ARP"
	default:
		return fmt.Sprintf("0x%x", int(e))
	}
}

// Proto is an enum for Network Protocols
type Proto uint32

const (
	ProtoICMP   Proto = 0x1
	ProtoTCP    Proto = 0x6
	ProtoUDP    Proto = 0x11
	ProtoICMPv6 Proto = 0x3A
)

func (p Proto) String() string {
	switch p {
	case ProtoICMP:
		return "ICMP"
	case ProtoTCP:
		return "TCP"
	case ProtoUDP:
		return "UDP"
	case ProtoICMPv6:
		return "ICMPv6"
	default:
		return fmt.Sprintf("0x%x", int(p))
	}
}

// HexUint32 is an integer that prefers to be printed in hexadecimal format
type HexUint32 uint32

func (h HexUint32) String() string {
	return fmt.Sprintf("0x%x", int(h))
}

// DecUint32 is an integer that prefers to be printed in hexadecimal format
type DecUint32 uint32

func (h DecUint32) String() string {
	return fmt.Sprintf("%d", int(h))
}

// DecUint32 is an integer that prefers to be printed in hexadecimal format
type DecUint64 uint64

func (h DecUint64) String() string {
	return fmt.Sprintf("%d", int(h))
}

// FlowKey is the struct of common fields that conform a flow
type FlowKey struct {
	FlowDirection FlowDirection

	// Interfaces
	InIf  DecUint32
	OutIf DecUint32
	// Ethernet Header
	SrcMac net.HardwareAddr
	DstMac net.HardwareAddr
	Etype  Etype
	// VLAN
	VlanID DecUint32
	// Network Header
	SrcAddr net.IP
	DstAddr net.IP
	Proto   Proto
	// TODO: Fragments
	// TODO MPLS

	// Transport Header
	SrcPort  DecUint32
	DstPort  DecUint32
	TCPFlags HexUint32
	ICMPType HexUint32
	ICMPCode HexUint32
}

// GetFieldString returns the string representation of the given fieldName
func (fk *FlowKey) GetFieldString(fieldName string) (string, error) {

	flowKeyV := reflect.ValueOf(fk).Elem()
	field := flowKeyV.FieldByName(fieldName)
	if !field.IsValid() {
		return "", fmt.Errorf("Failed to get Field %s from FlowKey", fieldName)
	}
	return fmt.Sprintf("%s", field.Interface()), nil
}

// Matches returns whether another FlowKey is equal to this one
// mask can be provided with a list of fields to compare
func (fk *FlowKey) Matches(other *FlowKey, mask []string) (bool, error) {
	if len(mask) == 0 {
		return reflect.DeepEqual(fk, other), nil
	}
	thisV := reflect.ValueOf(fk).Elem()
	otherV := reflect.ValueOf(other).Elem()
	for _, fieldName := range mask {
		thisField := thisV.FieldByName(fieldName)
		otherField := otherV.FieldByName(fieldName)
		if !thisField.IsValid() || !otherField.IsValid() {
			return false, fmt.Errorf("Comparison error. Field %s is not present in FlowKey", fieldName)
		}
		if !reflect.DeepEqual(thisField.Interface(), otherField.Interface()) {
			return false, nil
		}
	}
	return true, nil
}

func macFromUint64(uintMac uint64) net.HardwareAddr {
	mac := make([]byte, 8)
	binary.BigEndian.PutUint64(mac, uintMac)
	return net.HardwareAddr(mac[2:])
}

func ipFromBytes(ipBytes []byte) net.IP {
	return net.IP(ipBytes)
}

// FlowInfo contains a FlowKey and it's metadta
type FlowInfo struct {
	Key *FlowKey

	Bytes   DecUint64
	Packets DecUint64

	TimeReceived DecUint64

	TimeFlowStart DecUint64
	TimeFlowEnd   DecUint64

	ForwardingStatus uint32
}

func NewFlowInfo(msg *flowmessage.FlowMessage) *FlowInfo {
	return &FlowInfo{
		Key: &FlowKey{
			FlowDirection: FlowDirection(msg.FlowDirection),
			InIf:          DecUint32(msg.InIf),
			OutIf:         DecUint32(msg.OutIf),
			SrcMac:        macFromUint64(msg.SrcMac),
			DstMac:        macFromUint64(msg.DstMac),
			Etype:         Etype(msg.Etype),
			VlanID:        DecUint32(msg.VlanId),
			SrcAddr:       ipFromBytes(msg.SrcAddr),
			DstAddr:       ipFromBytes(msg.DstAddr),
			Proto:         Proto(msg.Proto),
			SrcPort:       DecUint32(msg.SrcPort),
			DstPort:       DecUint32(msg.DstPort),
			TCPFlags:      HexUint32(msg.TCPFlags),
			ICMPType:      HexUint32(msg.IcmpType),
			ICMPCode:      HexUint32(msg.IcmpCode),
		},
		TimeReceived:     DecUint64(msg.TimeReceived),
		TimeFlowStart:    DecUint64(msg.TimeFlowStart),
		TimeFlowEnd:      DecUint64(msg.TimeFlowEnd),
		Bytes:            DecUint64(msg.Bytes),
		Packets:          DecUint64(msg.Packets),
		ForwardingStatus: msg.ForwardingStatus,
	}
}

// FlowAggregate is a list of flows aggregated by a set of keys
type FlowAggregate struct {
	Keys  []string
	Flows []*FlowInfo

	TotalBytes   DecUint64
	TotalPackets DecUint64

	LastTimeReceived   DecUint64
	FirstTimeReceived  DecUint64
	FirstTimeFlowStart DecUint64
	LastTimeFlowEnd    DecUint64

	LastBps      DecUint64
	LastDeltaBps int

	LastForwardingStatus uint32
}

func NewFlowAggregate(keys []string) *FlowAggregate {
	return &FlowAggregate{
		Keys:  keys,
		Flows: make([]*FlowInfo, 0),
	}
}

// Append appends the FlowInfo to the current Aggregate
func (fa *FlowAggregate) AppendIfMatches(flowInfo *FlowInfo) (bool, error) {
	match, err := fa.matches(flowInfo)
	if err != nil {
		return false, err
	}
	if !match {
		return false, nil
	}
	fa.LastForwardingStatus = flowInfo.ForwardingStatus

	fa.TotalBytes += DecUint64(flowInfo.Bytes)
	fa.TotalPackets += DecUint64(flowInfo.Packets)

	if fa.FirstTimeReceived == 0 {
		fa.FirstTimeReceived = DecUint64(flowInfo.TimeReceived)
	}
	if fa.FirstTimeFlowStart == 0 {
		fa.FirstTimeFlowStart = DecUint64(flowInfo.TimeFlowStart)
	}

	fa.LastTimeReceived = DecUint64(flowInfo.TimeReceived)
	fa.LastTimeFlowEnd = DecUint64(flowInfo.TimeFlowEnd)

	var newBps DecUint64 = 0
	if fa.LastTimeFlowEnd != fa.FirstTimeFlowStart {
		newBps = DecUint64(fa.TotalBytes / (fa.LastTimeFlowEnd - fa.FirstTimeFlowStart))
	}

	fa.LastDeltaBps = int(newBps) - int(fa.LastBps)
	fa.LastBps = newBps

	fa.Flows = append(fa.Flows, flowInfo)
	return true, nil
}

func (fa *FlowAggregate) matches(flowInfo *FlowInfo) (bool, error) {
	if len(fa.Flows) == 0 {
		// Accept new members to the aggregate if emtpy
		return true, nil
	}
	return fa.Flows[0].Key.Matches(flowInfo.Key, fa.Keys)
}

// GetFieldString returns the string representation of the given fieldName of the first flow
// Since only the first flow is used, it is assumed that only fields within the key list are
// used
func (fa *FlowAggregate) GetFieldString(fieldName string) (string, error) {
	if len(fa.Flows) == 0 {
		return "", fmt.Errorf("Empty Aggregate")
	}
	return fa.Flows[0].Key.GetFieldString(fieldName)
}
