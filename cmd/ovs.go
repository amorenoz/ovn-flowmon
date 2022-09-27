package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"amorenoz/ovs-flowmon/pkg/netflow"
	"amorenoz/ovs-flowmon/pkg/ovs"
	"amorenoz/ovs-flowmon/pkg/stats"
	"amorenoz/ovs-flowmon/pkg/view"

	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var ovsCmd = &cobra.Command{
	Use:   "ovs [target]",
	Short: "Configure a local or remote ovs-vswitchd",
	Long: `Configure per-bridge IPFIX sampling on a local or remote ovs-vswitchd daemon. This mode allows you to also visualize life OvS statistics.
The target must be specified in "Connection Methods" (man(7) ovsdb). Default is: unix:/var/run/openvswitch/db.sock`,
	Run:  runOvs,
	Args: cobra.MaximumNArgs(1),
}

func ovsStart(bridge string, sampling, cacheMax, cacheTimeout int) {
	if ovsdb == "" {
		log.Error("OVSDB not configured")
		return
	}
	if !ovsClient.Started() {
		err := ovsClient.Start()
		if err != nil {
			log.Error("Failed to start Ovs Client")
			return
		}
		err = ovsClient.EnableStatistics()
		if err != nil {
			log.Fatal(err)
		}
	}
	ipAddr, err := ipAddressFromOvsdb(ovsdb)
	if err != nil {
		log.Fatalf("Bad OvS target %s", err.Error())
	}
	err = ovsClient.SetIPFIX(bridge, ipAddr+":2055", sampling, cacheMax, cacheTimeout)
	if err != nil {
		log.Error("Failed to set OVS configuration")
		log.Error(err)
	} else {
		log.Info("OVS configuration changed")
	}
}

func ovsStop() {
	if ovsClient != nil {
		log.Info("Stopping IPFIX exporter")
		err := ovsClient.ClearIPFIX()
		if err != nil {
			log.Error(err)
		}
		err = ovsClient.Close()
		if err != nil {
			log.Error(err)
		}
	}
}

func ovsConfigPage(pages *tview.Pages) {
	var err error
	if ovsdb == "" {
		return
	}
	// Initialize OVS Configuration client
	ovsClient, err = ovs.NewOVSClient(ovsdb, statsViewer, log)
	if err != nil {
		log.Fatal(err)
	}

	sampling := 400
	bridge := "br-int"
	form := tview.NewForm()
	form.AddDropDown("Bridge", []string{"br-int"}, 0, func(option string, _ int) {
		bridge = option
	}). // TODO: Add more
		AddInputField("Sampling", "400", 5, func(textToCheck string, _ rune) bool {
			_, err := strconv.ParseInt(textToCheck, 0, 32)
			return err == nil
		}, func(text string) {
			intVal, err := strconv.ParseInt(text, 0, 32)
			if err == nil {
				sampling = int(intVal)
			}
		}).
		AddButton("Save", func() {
			ovsStart(bridge, sampling, ovs.DefaultCacheMax, ovs.DefaultActiveTimeout)
			pages.HidePage("config")
		}).
		AddButton("Cancel", func() {
			pages.HidePage("config")
		})
	// TODO add cache size, etc
	configMenu := tview.NewFlex()
	configMenu.SetTitle("OVS Configuration").SetBorder(true)
	configMenu.SetDirection(tview.FlexRow).AddItem(tview.NewTextView().SetText(`Configure OvS IPFIX Exporter

Use <Tab> to move around the form
Press <Save> to save the configuration
Press <Cancel> to go back to the main menu


`), 0, 1, false).
		AddItem(form, 0, 2, true)
	pages.AddPage("config", view.Center(configMenu, 60, 20), true, false)
}

func runOvs(cmd *cobra.Command, args []string) {
	if len(args) == 1 {
		ovsdb = args[0]
	} else {
		ovsdb = "unix:/var/run/openvswitch/db.sock"
	}
	ipAddr, err := ipAddressFromOvsdb(ovsdb)
	if err != nil {
		log.Fatalf("Bad OvS target %s", err.Error())
	}
	app = tview.NewApplication()
	pages := tview.NewPages()
	statsViewer = stats.NewStatsView(app)
	flowTable = view.NewFlowTable().SetStatsBackend(statsViewer)
	menuConfig := view.MainPageConfig{
		FlowTable: flowTable,
		Stats:     statsViewer,
		OnExit:    ovsStop,
		ExtraMenu: func(menu *tview.List, log *logrus.Logger) error {
			menu.AddItem("Start OvS IPFIX Exporter", "", 's', func() {
				ovsStart("br-int", ovs.DefaultSampling, ovs.DefaultCacheMax, ovs.DefaultActiveTimeout)
			})
			menu.AddItem("(Re)Configure OvS IPFIX Exporter", "", 'c', func() {
				pages.ShowPage("config")
			})
			menu.AddItem("Stop OvS IPFIX Exporter", "", 't', func() {
				ovsStop()
			})
			return nil
		},
	}

	view.MainPage(app, pages, menuConfig, log)
	ovsConfigPage(pages)
	view.WelcomePage(pages, `In "ovs" mode you'll be able to configure OvS IPFIX sampling as well as to visualize live OvS statistics`)

	app.SetRoot(pages, true).SetFocus(pages)

	nf, err := netflow.NewNFReader(1,
		"netflow://"+ipAddr+":2055",
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

// ipAddressFromOvsdb returns the local IP address we use to listen based on an ovsdb string.
// If it's unix, we use localhost. If it's a remote connection, we use the source IP address that
// would be used to contact such remote host.
func ipAddressFromOvsdb(ovsdb string) (string, error) {
	parts := strings.Split(ovsdb, ":")
	switch parts[0] {
	case "tcp":
		conn, err := net.Dial("tcp", strings.Join(parts[1:], ":"))
		if err != nil {
			return "", fmt.Errorf("Failed to connect to remote OvS at %s", ovsdb)
		}
		return strings.Split(conn.LocalAddr().String(), ":")[0], nil
	case "unix":
		if _, err := os.Stat(parts[1]); errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("OvS socket file does not exist: %s", parts[1])
		}
		return "127.0.0.1", nil
	default:
		return "", fmt.Errorf("Unsupported OvS target. Only unix and tcp are supported %s", ovsdb)
	}
}
