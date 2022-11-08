package cmd

import (
	"amorenoz/ovs-flowmon/pkg/netflow"
	"amorenoz/ovs-flowmon/pkg/ovn"
	"amorenoz/ovs-flowmon/pkg/ovs"
	"amorenoz/ovs-flowmon/pkg/view"

	"github.com/spf13/cobra"
)

var ovnCmd = &cobra.Command{
	Use:   "ovn",
	Short: "Configure and visualize OVN debug-mode (experimental)",
	Long:  `In this mode ovs-flowmon connects to a OVN control plane. It configures OVN's debug-mode and drop sampling. Then it enriches each flow with OVN data extracted from the IPFIX sample`,
	Run:   runOvn,
}

func runOvn(cmd *cobra.Command, args []string) {
	var ovsClient *ovs.OVSClient = nil

	app := view.NewApp(log)
	app.FlowTable().SetMode(view.OVN)
	app.WelcomePage(`OVN mode. Drop sampling has been enabled in the remote OVN cluster.
However, IPFIX configuration needs to be added to each chassis that you want to sample. To do that, run the following command on them:

ovs-vsctl --id=@br get Bridge br-int --
	  --id=@i create IPFIX targets=\"${HOST_IP}:2055\"
	  --  create Flow_Sample_Collector_Set bridge=@br id=1 ipfix=@i
`)

	ovsdb, err := cmd.Flags().GetString("ovs")
	if err != nil {
		log.Fatal(err)
	}
	if ovsdb != "" {
		log.Infof("Starting OVS client: %s", ovsdb)
		ovsClient, err = ovs.NewOVSClient(ovsdb, app.Stats(), log)
	}

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
		&view.FlowConsumer{FlowTable: app.FlowTable(), App: app.App()},
		[]netflow.Enricher{ovnClient.OVNEnricher()},
		log)
	if err != nil {
		log.Fatal(err)
	}
	go nf.Listen()

	if ovsClient != nil {
		ovnOvsStartAndConfig(ovsClient, ovsdb)
	}

	if err := app.Run(); err != nil {
		panic(err)
	}
}

func ovnOvsStart(ovs *ovs.OVSClient) {
	err := ovs.Start()
	if err != nil {
		log.Error("Failed to start Ovs Client")
		return
	}
	err = ovs.EnableStatistics()
	if err != nil {
		log.Fatal(err)
	}
}

func ovnOvsStartAndConfig(ovs *ovs.OVSClient, ovsdb string) {
	if !ovs.Started() {
		ovnOvsStart(ovs)
	}
	ipAddr, err := ipAddressFromOvsdb(ovsdb)
	if err != nil {
		log.Fatalf("Bad OvS target %s", err.Error())
	}
	err = ovs.SetFlowSampling(ipAddr + ":2055")
	if err != nil {
		log.Fatalf("Failed to configure OVS Flow sampling: %s", err.Error())
	}
}
