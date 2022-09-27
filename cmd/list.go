package cmd

import (
	"amorenoz/ovs-flowmon/pkg/netflow"
	"amorenoz/ovs-flowmon/pkg/stats"
	"amorenoz/ovs-flowmon/pkg/view"

	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

var listenCmd = &cobra.Command{
	Use:   "listen [host:port]",
	Short: "Listen to exisiting IPFIX traffic",
	Long:  "An IPFIX exporter muxt be configured manually. Default listen address is: *:2055.",
	Run:   runListen,
	Args:  cobra.MaximumNArgs(1),
}

func runListen(cmd *cobra.Command, args []string) {
	ipPort := ":2055"
	if len(args) == 1 {
		ipPort = args[0]
	}
	app = tview.NewApplication()
	pages := tview.NewPages()

	statsViewer = stats.NewStatsView(app)
	flowTable = view.NewFlowTable().SetStatsBackend(statsViewer)
	menuConfig := view.MainPageConfig{
		FlowTable: flowTable,
		Stats:     statsViewer,
	}
	view.MainPage(app, pages, menuConfig, log)
	view.WelcomePage(pages, `In "listen" mode you must manually start an IPFIX exporter to send flows to this host.
In OpenvSwitch you can run something like:
"ovs-vsctl -- set Bridge br-int ipfix=@i \
           -- --id=@i create IPFIX targets=\"${HOST_IP}:2055\"

Note that if you had already started the IPFIX exporter, it might take some time (e.g: 10mins in OvS) before it sends us the Templates, without which we cannot
decode the IPFIX Flow Records. It is possible that re-starting the exporter helps.`)

	app.SetRoot(pages, true).SetFocus(pages)

	nf, err := netflow.NewNFReader(1,
		"netflow://"+ipPort,
		&view.FlowConsumer{FlowTable: flowTable, App: app},
		[]netflow.Enricher{},
		log)

	if err != nil {
		log.Fatal(err)
	}
	go nf.Listen()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
