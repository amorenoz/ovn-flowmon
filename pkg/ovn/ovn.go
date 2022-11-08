package ovn

import (
	"context"
	"fmt"
	"strings"

	"amorenoz/ovs-flowmon/pkg/netflow"

	"github.com/bombsimon/logrusr/v2"
	flowmessage "github.com/netsampler/goflow2/pb"
	"github.com/ovn-org/libovsdb/client"
	"github.com/ovn-org/libovsdb/model"
	"github.com/ovn-org/libovsdb/ovsdb"
	"github.com/sirupsen/logrus"
)

const (
	OVNDebugDomain = 1
	OVNACLDomain   = 2
)

// NBGlobal defines an object in NB_Global table
type NBGlobal struct {
	UUID    string            `ovsdb:"_uuid"`
	Options map[string]string `ovsdb:"options"`
}

type (
	// LogicalFlowPipeline is the type used for the Pipeline enum.
	LogicalFlowPipeline = string
)

var (
	// LogicalFlowPipelineIngress represents the "ingress" pipeline.
	LogicalFlowPipelineIngress LogicalFlowPipeline = "ingress"
	// LogicalFlowPipelineEgress represents the "egress" pipeline.
	LogicalFlowPipelineEgress LogicalFlowPipeline = "egress"
)

// LogicalFlow defines an object in Logical_Flow table.
type LogicalFlow struct {
	UUID    string `ovsdb:"_uuid"`
	Actions string `ovsdb:"actions"`
	//	ControllerMeter *string             `ovsdb:"controller_meter"`
	ExternalIDs     map[string]string   `ovsdb:"external_ids"`
	LogicalDatapath *string             `ovsdb:"logical_datapath"`
	LogicalDpGroup  *string             `ovsdb:"logical_dp_group"`
	Match           string              `ovsdb:"match"`
	Pipeline        LogicalFlowPipeline `ovsdb:"pipeline"`
	Priority        int                 `ovsdb:"priority"`
	TableID         int                 `ovsdb:"table_id"`
	Tags            map[string]string   `ovsdb:"tags"`
}

// DatapathBinding defines an object in Datapath_Binding table
type DatapathBinding struct {
	UUID        string            `ovsdb:"_uuid"`
	ExternalIDs map[string]string `ovsdb:"external_ids"`
	//LoadBalancers []string          `ovsdb:"load_balancers"`
	TunnelKey int `ovsdb:"tunnel_key"`
}

type (
	// DatapathType is an enum used to indicate the type of datapath
	// that originated thes sample.
	DatapathType = string
)

var (
	// DatapathTypeSwitch indicates the datapath is a Logical Switch.
	DatapathTypeSwitch DatapathType = "logical_switch"
	// DatapathTypeRouter indicates the datapath is a Logical Router.
	DatapathTypeRouter DatapathType = "logical_router"
	// DatapathTypePhysical indicates the Sample does not correspond to
	// a logical datapath but to a physical table.
	DatapathTypePhysical DatapathType = "physical"
)

type (
	ACLAction    = string
	ACLDirection = string
	ACLSeverity  = string
)

var (
	ACLActionAllow          ACLAction    = "allow"
	ACLActionAllowRelated   ACLAction    = "allow-related"
	ACLActionAllowStateless ACLAction    = "allow-stateless"
	ACLActionDrop           ACLAction    = "drop"
	ACLActionReject         ACLAction    = "reject"
	ACLDirectionFromLport   ACLDirection = "from-lport"
	ACLDirectionToLport     ACLDirection = "to-lport"
	//ACLSeverityAlert        ACLSeverity  = "alert"
	//ACLSeverityWarning      ACLSeverity  = "warning"
	//ACLSeverityNotice       ACLSeverity  = "notice"
	//ACLSeverityInfo         ACLSeverity  = "info"
	//ACLSeverityDebug        ACLSeverity  = "debug"
)

// ACL defines an object in ACL table
type ACL struct {
	UUID        string            `ovsdb:"_uuid"`
	Action      ACLAction         `ovsdb:"action"`
	Direction   ACLDirection      `ovsdb:"direction"`
	ExternalIDs map[string]string `ovsdb:"external_ids"`
	//Label       int               `ovsdb:"label"`
	//Log         bool              `ovsdb:"log"`
	//Match	      string		`ovsdb:"match"`
	//Meter       *string           `ovsdb:"meter"`
	Name    *string           `ovsdb:"name"`
	Options map[string]string `ovsdb:"options"`
	//Priority    int               `ovsdb:"priority"`
	//Severity    *ACLSeverity      `ovsdb:"severity"`
}

// SampleInfo represents the OVN information associated with a sample.
type SampleInfo struct {
	// Flow information.
	LogicalFlow   *LogicalFlow
	OpenFlowTable int
	// Datapath Information.
	DatapathType DatapathType
	DatapathName string
}

// OVNClient is the main object that configures and retrieves information from OVN.
type OVNClient struct {
	nb  client.Client
	sb  client.Client
	log *logrus.Logger
}

func NewOVNClient(nbStr string, sbStr string, log *logrus.Logger) (*OVNClient, error) {
	logr := logrusr.New(log)
	var err error

	// Connect NB.
	dbmodel, err := model.NewDBModel("OVN_Northbound",
		map[string]model.Model{
			"NB_Global": &NBGlobal{},
			"ACL":       &ACL{},
		})
	if err != nil {
		return nil, err
	}
	nb, err := client.NewOVSDBClient(dbmodel, client.WithEndpoint(nbStr), client.WithLogger(&logr))
	if err != nil {
		return nil, err
	}

	// Connect SB.
	dbmodel, err = model.NewDBModel("OVN_Southbound",
		map[string]model.Model{
			"Logical_Flow":     &LogicalFlow{},
			"Datapath_Binding": &DatapathBinding{},
		})
	sb, err := client.NewOVSDBClient(dbmodel, client.WithEndpoint(sbStr), client.WithLogger(&logr))
	if err != nil {
		return nil, err
	}
	return &OVNClient{
		nb:  nb,
		sb:  sb,
		log: log,
	}, nil
}
func (o *OVNClient) Close() error {
	nbs := []NBGlobal{}
	if !o.nb.Connected() {
		return nil
	}

	if err := o.nb.List(&nbs); err != nil {
		return err
	}
	nb := nbs[0]
	delete(nb.Options, "debug_drop_collector_set")
	delete(nb.Options, "debug_drop_domain_id")
	clearOps, err := o.nb.Where(&nb).Update(&nb, &nb.Options)
	if err != nil {
		o.log.Error(err)
	} else {
		response, err := o.nb.Transact(context.TODO(), clearOps...)
		if err != nil {
			o.log.Error(err)
		}
		if opErr, err := ovsdb.CheckOperationResults(response, clearOps); err != nil {
			o.log.Errorf("%s: %+v", err.Error(), opErr)
		}
	}

	o.nb.Close()
	o.sb.Close()
	return nil
}

func (o *OVNClient) Started() bool {
	return o.nb.Connected() && o.sb.Connected()
}

func (o *OVNClient) Start() error {
	var err error
	for _, db := range []client.Client{o.nb, o.sb} {
		if db.Connected() {
			return nil
		}
		err := db.Connect(context.Background())
		if err != nil {
			return err
		}
	}
	_, err = o.nb.MonitorAll(context.TODO())
	if err != nil {
		return err
	}
	_, err = o.sb.Monitor(context.TODO(), o.sb.NewMonitor(
		client.WithTable(&LogicalFlow{}), client.WithTable(&DatapathBinding{})))
	if err != nil {
		return err
	}
	return nil
}

func (o *OVNClient) SetDebugMode() error {
	if !o.nb.Connected() {
		return fmt.Errorf("Client not connected")
	}

	nbs := []NBGlobal{}
	if !o.nb.Connected() {
		return nil
	}

	if err := o.nb.List(&nbs); err != nil {
		return err
	}
	nb := nbs[0]

	mutateOps, err := o.nb.Where(&nb).Mutate(&nb,
		model.Mutation{
			Field:   &nb.Options,
			Mutator: ovsdb.MutateOperationInsert,
			Value:   map[string]string{"debug_drop_collector_set": "1"},
		},
		model.Mutation{
			Field:   &nb.Options,
			Mutator: ovsdb.MutateOperationInsert,
			Value:   map[string]string{"debug_drop_domain_id": "1"},
		})
	if err != nil {
		return err
	}
	response, err := o.nb.Transact(context.TODO(), mutateOps...)
	logFields := logrus.Fields{
		"operation": mutateOps,
		"response":  response,
		"err":       err,
	}
	if err != nil {
		o.log.WithFields(logFields).Error(err)
	}
	if opErr, err := ovsdb.CheckOperationResults(response, mutateOps); err != nil {
		o.log.WithFields(logFields).Errorf("%s: %+v", err.Error(), opErr)
	}
	o.log.WithFields(logFields).Info("OVN Drop sampling: Enabled")
	return nil
}

/* Extract OVN Information from the sample.

ObservationDomainID has the following format
| 31 ---- 24 | 23 ----------- 0|
  DomainID       Datapath Key

If DomainID is the OVN Drop Debugging ID = 1 then, the ObservationPoitID is:

- The cookie (first 32 bits of the Logical Flow's UUID) if Datapath Key corresponds
to an existing datapath.
- The table number if the Datapath Key is zero.
*/

func (o *OVNClient) extractObservationIds(sample *flowmessage.FlowMessage) (uint32, uint32, uint32, error) {
	obsDomainID := sample.ObservationDomainID

	domain := (obsDomainID & 0xFF000000) >> 24
	tunnelKey := obsDomainID & 0x00FFFFFF

	if domain != OVNDebugDomain && domain != OVNACLDomain {
		return 0, 0, 0, fmt.Errorf("DomainID %d not supported for OVN Data extraction", domain)
	}
	obsPointID := sample.ObservationPointID
	return domain, obsPointID, tunnelKey, nil
}

func (o *OVNClient) getSampleOVNInfo(sample *flowmessage.FlowMessage) (*SampleInfo, error) {
	_, point, tunnel, err := o.extractObservationIds(sample)
	if err != nil {
		return nil, err
	}
	return o.getOVNDebugSampleInfo(tunnel, point)
}

func (o *OVNClient) getSampleACLInfo(sample *flowmessage.FlowMessage) (*ACL, error) {
	_, point, _, err := o.extractObservationIds(sample)
	if err != nil {
		return nil, err
	}
	return o.getACL(point)
}

func (o *OVNClient) getOVNDebugSampleInfo(tunnelKey, obsPointID uint32) (*SampleInfo, error) {
	var lflow *LogicalFlow
	var table int
	var dpName string
	var dpType DatapathType

	if tunnelKey == 0 {
		dpType = DatapathTypePhysical
		table = int(obsPointID)
	} else {
		table = -1
		dp, err := o.getDatapath(tunnelKey)
		if err != nil {
			return nil, err
		}
		if ls, ok := dp.ExternalIDs["logical-switch"]; ok && ls != "" {
			dpType = DatapathTypeSwitch
			dpName = dp.ExternalIDs["name"]
		} else if lr, ok := dp.ExternalIDs["logical-router"]; ok && lr != "" {
			dpType = DatapathTypeRouter
			dpName = dp.ExternalIDs["name"]
		} else {
			return nil, fmt.Errorf("Datapath Binding from unsuported type: %#v", *dp)
		}
		lflow, err = o.getLFlow(obsPointID)
		if err != nil {
			return nil, err
		}
	}

	return &SampleInfo{
		LogicalFlow:   lflow,
		OpenFlowTable: table,
		DatapathType:  dpType,
		DatapathName:  dpName,
	}, nil

}

/* Look if there is a Logical Flow whose UUID starts with the hexadecimal
representation of the ObservationPointID.*/
func (o *OVNClient) getACL(observationPointID uint32) (*ACL, error) {
	acls := []ACL{}
	obsString := fmt.Sprintf("%08x", int(observationPointID))
	err := o.nb.WhereCache(
		func(ls *ACL) bool {
			return strings.HasPrefix(ls.UUID, obsString)
		}).List(&acls)
	if err != nil {
		return nil, err
	}
	if len(acls) == 0 {
		return nil, fmt.Errorf("No ACL found with observationPointID %s", obsString)
	}
	if len(acls) > 1 {
		o.log.Warningf("Duplicated ACL found with observationPointID %s", obsString)
	}
	return &acls[0], nil
}

/* Look if there is a Logical Flow whose UUID starts with the hexadecimal
representation of the ObservationPointID.*/
func (o *OVNClient) getLFlow(observationPointID uint32) (*LogicalFlow, error) {
	lf := []LogicalFlow{}
	obsString := fmt.Sprintf("%08x", int(observationPointID))
	err := o.sb.WhereCache(
		func(ls *LogicalFlow) bool {
			return strings.HasPrefix(ls.UUID, obsString)
		}).List(&lf)
	if err != nil {
		return nil, err
	}
	if len(lf) == 0 {
		return nil, fmt.Errorf("No LogicalFlow found with observationPointID %s", obsString)
	}
	if len(lf) > 1 {
		o.log.Warningf("Duplicated LogicalFlow found with observationPointID %s", obsString)
	}
	return &lf[0], nil
}

// Get the DatapathBinding object associated with the given tunnel_key
func (o *OVNClient) getDatapath(tunnelKey uint32) (*DatapathBinding, error) {
	dps := []DatapathBinding{}
	err := o.sb.WhereCache(
		func(dp *DatapathBinding) bool {
			return dp.TunnelKey == int(tunnelKey)
		}).List(&dps)

	if err != nil {
		return nil, err
	}
	if len(dps) == 0 {
		return nil, fmt.Errorf("No DatapathBinding found with TunnelKey %d", tunnelKey)
	}
	if len(dps) > 1 {
		o.log.Warningf("Duplicated Logicadplow found with TunnelKey %d", tunnelKey)
	}
	return &dps[0], nil
}

// OVNEnricher returns an Enricher that augments the flow data with generic OVN information
func (o *OVNClient) OVNEnricher() netflow.Enricher {
	return &OVNEnricher{o}
}

// ACLEnricher returns an Enricher that augments the flow data with ACL information
func (o *OVNClient) ACLEnricher() netflow.Enricher {
	return &OVNACLEnricher{o}
}

func (o *OVNClient) EnrichOVN(msg *flowmessage.FlowMessage, extra map[string]interface{}, log *logrus.Logger) map[string]interface{} {
	sampleInfo, err := o.getSampleOVNInfo(msg)
	if err != nil {
		log.Error(err)
		return extra
	}
	if sampleInfo.LogicalFlow != nil {
		extra["LFUUID"] = sampleInfo.LogicalFlow.UUID
		extra["LFMatch"] = sampleInfo.LogicalFlow.Match
		extra["LFActions"] = sampleInfo.LogicalFlow.Actions
		extra["LFPipeline"] = string(sampleInfo.LogicalFlow.Pipeline)
		extra["LFStage"] = string(sampleInfo.LogicalFlow.ExternalIDs["stage-name"])
	}
	extra["DPType"] = string(sampleInfo.DatapathType)
	extra["DPName"] = string(sampleInfo.DatapathName)
	extra["OFTable"] = sampleInfo.OpenFlowTable
	return extra
}

func (o *OVNClient) EnrichOVNACL(msg *flowmessage.FlowMessage, extra map[string]interface{}, log *logrus.Logger) map[string]interface{} {
	acl, err := o.getSampleACLInfo(msg)
	if err != nil {
		log.Error(err)
		return extra
	}
	if acl != nil {
		if acl.Name != nil {
			extra["ACLName"] = string(*acl.Name)
		} else {
			extra["ACLName"] = ""
		}
		//extra["ACLLabel"] = string(acl.Label)
		extra["ACLDirection"] = string(acl.Direction)
		extra["ACLAction"] = string(acl.Action)
	}
	return extra
}

// OVNEnricher implements the enricher interface for a OVNClient in OVN mode.
type OVNEnricher struct {
	client *OVNClient
}

func (o *OVNEnricher) Enrich(msg *flowmessage.FlowMessage, extra map[string]interface{}, log *logrus.Logger) map[string]interface{} {
	return o.client.EnrichOVN(msg, extra, log)
}

// OVNACLEnricher implements the enricher interface for a OVNClient in ACL mode.
type OVNACLEnricher struct {
	client *OVNClient
}

func (o *OVNACLEnricher) Enrich(msg *flowmessage.FlowMessage, extra map[string]interface{}, log *logrus.Logger) map[string]interface{} {
	return o.client.EnrichOVNACL(msg, extra, log)
}
