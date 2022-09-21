package cmd

import (
	"amorenoz/ovs-flowmon/pkg/netflow"
	"amorenoz/ovs-flowmon/pkg/ovn"
	"amorenoz/ovs-flowmon/pkg/stats"
	"amorenoz/ovs-flowmon/pkg/view"

	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

var ovnCmd = &cobra.Command{
	Use:   "ovn",
	Short: "Configure and visualize OVN debug-mode (experimental)",
	Long:  `In this mode ovs-flowmon connects to a OVN control plane. It configures OVN's debug-mode and drop sampling. Then it enriches each flow with OVN data extracted from the IPFIX sample`,
	Run:   run_ovn,
}

func run_ovn(cmd *cobra.Command, args []string) {
	app = tview.NewApplication()
	pages := tview.NewPages()

	statsViewer = stats.NewStatsView(app)
	flowTable = view.NewFlowTable().SetStatsBackend(statsViewer).SetOVN(true)
	menuConfig := view.MainPageConfig{
		FlowTable: flowTable,
		Stats:     statsViewer,
	}
	view.MainPage(app, pages, menuConfig, log)
	view.WelcomePage(pages, `OVN mode. Drop sampling has been enabled in the remote OVN cluster.
However, IPFIX configuration needs to be added to each chassis that you want to sample. To do that, run the following command on them:

ovs-vsctl --id=@br get Bridge br-int --
	  --id=@i create IPFIX targets=\"${HOST_IP}:2055\"
	  --  create Flow_Sample_Collector_Set bridge=@br id=1 ipfix=@i
`)

	app.SetRoot(pages, true).SetFocus(pages)

	nb, err := cmd.Flags().GetString("nbdb")
	if err != nil {
		log.Fatal(err)
	}
	sb, err := cmd.Flags().GetString("sbdb")
	if err != nil {
		log.Fatal(err)
	}

	ovnClient, err := ovn.NewOVNClient(nb, sb, log)
	if err != nil {
		log.Fatal(err)
	}
	err = ovnClient.Start()
	if err != nil {
		log.Fatal(err)
	}
	err = ovnClient.SetDebugMode()
	if err != nil {
		log.Fatal(err)
	}
	log.Info("OVN Client started")

	ipAddr := ""
	nf, err := netflow.NewNFReader(1,
		"netflow://"+ipAddr+":2055",
		&view.FlowConsumer{FlowTable: flowTable, App: app},
		[]netflow.Enricher{ovnClient},
		log)
	if err != nil {
		log.Fatal(err)
	}
	go nf.Listen()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
