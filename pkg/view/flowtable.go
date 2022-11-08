package view

import (
	"amorenoz/ovs-flowmon/pkg/flowmon"
	"amorenoz/ovs-flowmon/pkg/stats"
	"fmt"
	"sort"
	"sync"

	"github.com/gdamore/tcell/v2"
	flowmessage "github.com/netsampler/goflow2/pb"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

type SelectMode int

const ModeRows SelectMode = 0     // Only Rows are selectable
const ModeColsAll SelectMode = 1  // All Columns are selectable
const ModeColsKeys SelectMode = 2 // Only Flow Key columns are selectable

// TableMode defines different modes or layouts
type TableMode int

// Normal mode: Columns with basic traffic headers are shown.
const Normal TableMode = 0

// OVN Drop mode: Columns show Logical Flow and Datapath information.
const OVN TableMode = 1

// OVNAcl mode: Columns show ACL name, direction and veredict.
const OVNAcl TableMode = 2

const ProcessedMessagesStat string = "Processed Messages"

var fieldList []string = []string{
	"InIf",
	"OutIf",
	"SrcMac",
	"DstMac",
	"VlanID",
	"Etype",
	"SrcAddr",
	"DstAddr",
	"Proto",
	"SrcPort",
	"DstPort",
	"SvcPort",
	"FlowDirection"}

var ovnFieldList []string = []string{
	"LFUUID",
	"LFMatch",
	"LFActions",
	"LFPipeline",
	"LFStage",
	"DPType",
	"DPName",
	"OFTable",
}

var ovnAclFieldList []string = []string{
	"ACLName",
	"ACLDirection",
	"ACLAction",
}

// FlowConsumer implementes the netflow.Consumer interface and adds the flowmessages
// to a FlowTable
type FlowConsumer struct {
	FlowTable *FlowTable
	App       *tview.Application
}

// Consume adds the flowmessage to the FlowTable
func (fc *FlowConsumer) Consume(msg *flowmessage.FlowMessage, extra map[string]interface{}, log *logrus.Logger) {
	fc.FlowTable.ProcessMessage(msg, extra)
	fc.App.QueueUpdateDraw(func() {
		fc.FlowTable.Draw()
	})
}

// FlowTable is in charge of managing a table of flows with aggregates.
type FlowTable struct {
	View  *tview.Table
	stats stats.StatsBackend
	// mutex to protect aggregates (not flows)
	mutex sync.RWMutex

	// data
	flows      []*flowmon.FlowInfo
	aggregates []*flowmon.FlowAggregate
	lessFunc   func(one, other *flowmon.FlowAggregate) bool

	// configuration
	// Keeping both the list and the map for efficiency
	aggregateKeyList []string
	aggregateKeyMap  map[string]bool
	keys             []string
	mode             SelectMode

	// Stats
	nMessages int
}

func NewFlowTable() *FlowTable {
	fields := fieldList
	tableView := tview.NewTable().
		SetSelectable(true, false). // Allow flows to be selected
		SetFixed(1, 1).             // Make it always focus the top left
		SetSelectable(true, false)  // Start in RowMode

	ft := &FlowTable{
		View:             tableView,
		stats:            nil,
		mutex:            sync.RWMutex{},
		flows:            make([]*flowmon.FlowInfo, 0),
		aggregates:       make([]*flowmon.FlowAggregate, 0),
		aggregateKeyList: nil,
		aggregateKeyMap:  nil,
		keys:             fields,
		lessFunc: func(one, other *flowmon.FlowAggregate) bool {
			return one.LastTimeReceived < other.LastTimeReceived
		},
	}
	ft.updateFieldsLocked()
	return ft
}

func (ft *FlowTable) SetStatsBackend(statsBackend stats.StatsBackend) *FlowTable {
	statsBackend.RegisterStat(ProcessedMessagesStat)
	ft.stats = statsBackend
	return ft
}

func (ft *FlowTable) SetMode(mode TableMode) *FlowTable {
	needsUpdate := false
	switch mode {
	case Normal:
		break
	case OVN:
		for _, field := range ovnFieldList {
			ft.keys = append(ft.keys, field)
		}
		needsUpdate = true
	case OVNAcl:
		for _, field := range ovnAclFieldList {
			ft.keys = append(ft.keys, field)
		}
		needsUpdate = true
	}
	if needsUpdate {
		ft.mutex.Lock()
		defer ft.mutex.Unlock()
		ft.updateFieldsLocked()
	}
	return ft
}

func (ft *FlowTable) GetAggregates() map[string]bool {
	return ft.aggregateKeyMap
}

func (ft *FlowTable) UpdateKeys(aggregates map[string]bool) {
	aggregateKeyList := []string{}
	for _, field := range ft.keys {
		if aggregates[field] {
			aggregateKeyList = append(aggregateKeyList, field)
		}
	}
	ft.mutex.Lock()
	defer ft.mutex.Unlock()
	ft.aggregateKeyList = aggregateKeyList
	ft.aggregateKeyMap = aggregates
	// Need to recompute all aggregations
	ft.recompute()
}

func (ft *FlowTable) ToggleAggregate(index int) {
	colName := ft.View.GetCell(0, index).Text
	// Toggle aggregate and update flow table
	newAggregates := make(map[string]bool)
	for k, v := range ft.aggregateKeyMap {
		newAggregates[k] = v
	}
	newAggregates[colName] = !newAggregates[colName]
	ft.UpdateKeys(newAggregates)
	ft.View.Clear()
	ft.Draw()
}

func (ft *FlowTable) SetSelectMode(mode SelectMode) {
	switch mode {
	case ModeRows:
		ft.View.SetSelectable(true, false)
	case ModeColsAll, ModeColsKeys:
		ft.View.SetSelectable(false, true)
	}
	ft.mode = mode
	ft.Draw()
}

func (ft *FlowTable) Draw() {
	var cell *tview.TableCell
	// Draw Key
	for col, key := range ft.keys {
		cell = tview.NewTableCell(key).SetTextColor(tcell.ColorWhite).SetAlign(tview.AlignLeft).SetSelectable(ft.mode != ModeRows)
		ft.View.SetCell(0, col, cell)
	}

	col := len(ft.keys)

	cell = tview.NewTableCell("TotalBytes").
		SetTextColor(tcell.ColorWhite).
		SetAlign(tview.AlignLeft).
		SetSelectable(ft.mode == ModeColsAll)
	ft.View.SetCell(0, col, cell)
	col += 1
	cell = tview.NewTableCell("TotalPackets").
		SetTextColor(tcell.ColorWhite).
		SetAlign(tview.AlignLeft).
		SetSelectable(ft.mode == ModeColsAll)
	ft.View.SetCell(0, col, cell)
	col += 1
	cell = tview.NewTableCell("Rate(kbps)").
		SetTextColor(tcell.ColorWhite).
		SetAlign(tview.AlignLeft).
		SetSelectable(ft.mode == ModeColsAll)
	ft.View.SetCell(0, col, cell)
	col += 1

	ft.mutex.RLock()
	defer ft.mutex.RUnlock()
	for i, agg := range ft.aggregates {
		for col, key := range ft.keys {
			var fieldStr string
			var err error
			if !ft.aggregateKeyMap[key] {
				fieldStr = "-"
			} else {
				fieldStr, err = agg.GetFieldString(key)
				if err != nil {
					log.Error(err)
					fieldStr = "err"
				}
			}
			cell = tview.NewTableCell(fieldStr).SetTextColor(tcell.ColorWhite).SetAlign(tview.AlignLeft).SetSelectable(ft.mode == ModeRows)
			ft.View.SetCell(1+i, col, cell)
		}
		col := len(ft.keys)

		cell = tview.NewTableCell(fmt.Sprintf("%d", int(agg.TotalBytes))).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		ft.View.SetCell(1+i, col, cell)
		col += 1
		cell = tview.NewTableCell(fmt.Sprintf("%d", int(agg.TotalPackets))).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)
		ft.View.SetCell(1+i, col, cell)
		col += 1

		delta := "="
		if agg.LastDeltaBps > 0 {
			delta = "↑"
		} else if agg.LastDeltaBps < 0 {
			delta = "↓"
		}
		cell = tview.NewTableCell(fmt.Sprintf("%.1f %s", float64(agg.LastBps)/1000, delta)).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignLeft).
			SetSelectable(false)

		ft.View.SetCell(1+i, col, cell)
		col += 1
	}
	ft.stats.UpdateStat(ProcessedMessagesStat, fmt.Sprintf("%d", ft.nMessages))
	ft.stats.Draw()
}

func (ft *FlowTable) ProcessMessage(msg *flowmessage.FlowMessage, extra map[string]interface{}) {
	log.Debugf("Processing Flow Message: %+v", msg)

	flowInfo := flowmon.NewFlowInfo(msg, extra)
	ft.flows = append(ft.flows, flowInfo)

	ft.mutex.Lock()
	defer ft.mutex.Unlock()
	ft.ProcessFlow(flowInfo)
	ft.nMessages += 1
}

// Caller must hold mutex
func (ft *FlowTable) ProcessFlow(flowInfo *flowmon.FlowInfo) {
	var matched bool = false
	var err error = nil
	for i, agg := range ft.aggregates {
		matched, err = agg.AppendIfMatches(flowInfo)
		if err != nil {
			log.Error(err)
			return
		}
		if matched {
			// Re-insert the matched aggregate
			ft.aggregates = append(ft.aggregates[:i], ft.aggregates[i+1:]...)
			ft.insertSortedAggregate(agg)
			break
		}
	}
	if !matched {
		// Create new Aggregate for this flow
		newAgg := flowmon.NewFlowAggregate(ft.aggregateKeyList)
		if match, err := newAgg.AppendIfMatches(flowInfo); !match || err != nil {
			log.Fatal(err)
		}

		// Sorted insertion
		ft.insertSortedAggregate(newAgg)
	}
}

// SetSortingKey sets the field that will be used for sorting the aggregates
func (ft *FlowTable) SetSortingColumn(index int) error {
	colName := ft.View.GetCell(0, index).Text
	return ft.SetSortingKey(colName)
}

// SetSortingKey sets the field that will be used for sorting the aggregates
func (ft *FlowTable) SetSortingKey(key string) error {
	switch key {
	case "LastTimeReceived":
		ft.lessFunc = func(one, other *flowmon.FlowAggregate) bool {
			return one.LastTimeReceived < other.LastTimeReceived
		}
	case "Rate(kbps)":
		ft.lessFunc = func(one, other *flowmon.FlowAggregate) bool {
			return one.LastBps < other.LastBps
		}
	case "TotalBytes":
		ft.lessFunc = func(one, other *flowmon.FlowAggregate) bool {
			return one.TotalBytes < other.TotalBytes
		}
	case "TotalPackets":
		ft.lessFunc = func(one, other *flowmon.FlowAggregate) bool {
			return one.TotalBytes < other.TotalBytes
		}

	default:
		found := false
		for _, k := range ft.keys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Cannot set sorting key to %s", key)
		}
		if !ft.aggregateKeyMap[key] {
			return fmt.Errorf("Cannot set sorting key to %s since it's not part of the aggregate", key)
		}
		ft.lessFunc = func(one, other *flowmon.FlowAggregate) bool {
			res, _ := one.Less(key, other)
			return res
		}
	}

	ft.mutex.Lock()
	ft.recompute()
	ft.mutex.Unlock()
	// Do not hold the lock while redrawing

	ft.View.Clear()
	ft.Draw()
	return nil
}

func (ft *FlowTable) insertSortedAggregate(agg *flowmon.FlowAggregate) {
	insertionPoint := sort.Search(len(ft.aggregates), func(i int) bool {
		return ft.lessFunc(ft.aggregates[i], agg)
	})
	if insertionPoint == len(ft.aggregates) {
		ft.aggregates = append(ft.aggregates, agg)
	} else {
		ft.aggregates = append(ft.aggregates[0:insertionPoint],
			append([]*flowmon.FlowAggregate{agg}, ft.aggregates[insertionPoint:]...)...)
	}
}

// Recompute all aggregates
func (ft *FlowTable) recompute() {
	ft.aggregates = make([]*flowmon.FlowAggregate, 0)
	for _, flow := range ft.flows {
		ft.ProcessFlow(flow)
	}
}

func (ft *FlowTable) updateFieldsLocked() {
	aggregateKeyList := []string{}
	aggregates := map[string]bool{}
	for _, field := range ft.keys {
		aggregateKeyList = append(aggregateKeyList, field)
		if _, ok := aggregates[field]; !ok {
			aggregates[field] = true
		}
	}
	ft.aggregateKeyList = aggregateKeyList
	ft.aggregateKeyMap = aggregates
}
