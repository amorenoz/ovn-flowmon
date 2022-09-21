package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"amorenoz/ovs-flowmon/pkg/netflow"
	"amorenoz/ovs-flowmon/pkg/ovn"
	"amorenoz/ovs-flowmon/pkg/ovs"
	"amorenoz/ovs-flowmon/pkg/stats"
	"amorenoz/ovs-flowmon/pkg/view"

	"github.com/gdamore/tcell/v2"
	_ "github.com/netsampler/goflow2/format/protobuf"
	flowmessage "github.com/netsampler/goflow2/pb"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	app         *tview.Application
	flowTable   *view.FlowTable
	statsViewer *stats.StatsView
	ovsClient   *ovs.OVSClient
	log         = logrus.New()
	logLevel    string
	ovsdb       string

	rootCmd = &cobra.Command{
		Use:   "ovs-flowmon",
		Short: "ovs-flowmon is an interactive IPFIX flow visualizer specially supporting OVS/OVN",
		Long: `An interactive IPFIX collector and visualization tool. Although it can work with any IPFIX exporter,
it supports configuring OVS IPFIX sampling and statistics.`,
		Run: func(cmd *cobra.Command, args []string) {
		},
	}

	listenCmd = &cobra.Command{
		Use:   "listen [host:port]",
		Short: "Listen to exisiting IPFIX traffic",
		Long:  "An IPFIX exporter muxt be configured manually. Default listen address is: *:2055.",
		Run:   run_listen,
		Args:  cobra.MaximumNArgs(1),
	}

	ovsCmd = &cobra.Command{
		Use:   "ovs [target]",
		Short: "Configure a local or remote ovs-vswitchd",
		Long: `Configure per-bridge IPFIX sampling on a local or remote ovs-vswitchd daemon. This mode allows you to also visualize life OvS statistics.
The target must be specified in "Connection Methods" (man(7) ovsdb). Default is: unix:/var/run/openvswitch/db.sock`,
		Run:  run_ovs,
		Args: cobra.MaximumNArgs(1),
	}

	ovnCmd = &cobra.Command{
		Use:   "ovn",
		Short: "Configure and visualize OVN debug-mode (experimental)",
		Long:  `In this mode ovs-flowmon connects to a OVN control plane. It configures OVN's debug-mode and drop sampling. Then it enriches each flow with OVN data extracted from the IPFIX sample`,
		Run:   run_ovn,
	}
)

type Listener interface {
	OnNewFlow(flow *flowmessage.FlowMessage)
}

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
		return "", fmt.Errorf("Unsupported OvS target. Only unix and tcp are supported")
	}
}

func welcomePage(pages *tview.Pages, message string) {
	welcome := tview.NewModal().SetText(`

Welcome to OvS Flow Monitor!

` + message + `


`).AddButtons([]string{"Start"}).SetDoneFunc(func(index int, label string) {
		pages.HidePage("welcome")
		pages.ShowPage("main")
	})
	pages.AddPage("welcome", welcome, true, true)
}

func mainPage(pages *tview.Pages, ovn bool) {
	statsViewer = stats.NewStatsView(app)
	flowTable = view.NewFlowTable(statsViewer, ovn)
	status := tview.NewTextView().SetText("Stopped. Press Start to start capturing\n")
	log.SetOutput(status)

	// Top Menu
	menu := tview.NewFlex()
	menuList := tview.NewList().
		ShowSecondaryText(false)
	menu.AddItem(menuList, 0, 2, true)
	menu.AddItem(statsViewer.View(), 0, 2, false)

	exit := func() {
		exit()
	}
	logs := func() {
		app.SetFocus(status)
	}
	flows := func() {
		app.SetFocus(flowTable.View)
	}

	show_aggregate := func() {
		// Make columns selectable
		flowTable.SetSelectMode(view.ModeColsKeys)
		app.SetFocus(flowTable.View)
		flowTable.View.SetSelectedFunc(func(row, col int) {
			flowTable.ToggleAggregate(col)
			flowTable.SetSelectMode(view.ModeRows)
			flowTable.View.SetSelectedFunc(func(row, col int) {
				app.SetFocus(menuList)
			})
			app.SetFocus(menuList)
		})
	}

	sort_by := func() {
		// Make columns selectable
		flowTable.SetSelectMode(view.ModeColsAll)
		app.SetFocus(flowTable.View)
		flowTable.View.SetSelectedFunc(func(row, col int) {
			err := flowTable.SetSortingColumn(col)
			if err != nil {
				log.Error(err)
			}
			flowTable.SetSelectMode(view.ModeRows)
			flowTable.View.SetSelectedFunc(func(row, col int) {
				app.SetFocus(menuList)
			})
			app.SetFocus(menuList)
		})
	}

	if ovsdb != "" {
		menuList.AddItem("Start OvS IPFIX Exporter", "", 's', func() {
			ovs_start("br-int", ovs.DefaultSampling, ovs.DefaultCacheMax, ovs.DefaultActiveTimeout)
		})
		menuList.AddItem("(Re)Configure OvS IPFIX Exporter", "", 'c', func() {
			pages.ShowPage("config")
		})
		menuList.AddItem("Stop OvS IPFIX Exporter", "", 't', func() {
			ovs_stop()
		})
	}
	menuList.AddItem("Flows", "", 'f', flows).
		AddItem("Add/Remove Fields from aggregate", "", 'a', show_aggregate).
		AddItem("Sort by ", "", 's', sort_by).
		AddItem("Logs", "", 'l', logs).
		AddItem("Exit", "", 'e', exit)
	flowTable.View.SetDoneFunc(func(key tcell.Key) {
		app.SetFocus(menuList)
	})
	flowTable.View.SetSelectedFunc(func(row, col int) {
		app.SetFocus(menuList)
	})
	status.SetDoneFunc(func(key tcell.Key) {
		app.SetFocus(menuList)
	})
	menuList.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Menu")
	flowTable.View.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Flows")
	status.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Logs")

	flex := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(menu, 0, 2, true).AddItem(flowTable.View, 0, 5, false).AddItem(status, 0, 1, false)

	pages.AddPage("main", flex, true, false)
}

func center(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}
func configPage(pages *tview.Pages) {
	var err error
	if ovsdb == "" {
		return
	}
	// Initialize OVS Configuration client
	ovsClient, err = ovs.NewOVSClient(ovsdb, statsViewer, log)
	if err != nil {
		fmt.Print(err)
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
			ovs_start(bridge, sampling, ovs.DefaultCacheMax, ovs.DefaultActiveTimeout)
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
	pages.AddPage("config", center(configMenu, 60, 20), true, false)
}

func ovs_stop() {
	if ovsClient != nil {
		log.Info("Stopping IPFIX exporter")
		err := ovsClient.Close()
		if err != nil {
			log.Error(err)
		}
	}
}

func ovs_start(bridge string, sampling, cacheMax, cacheTimeout int) {
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
			fmt.Print(err)
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

func exit() {
	log.Info("Stopping")
	ovs_stop()
	if app != nil {
		app.Stop()
	}
}

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

//func main() {
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&logLevel, "loglevel", "l", "info", "Log level")

	// listen
	rootCmd.AddCommand(listenCmd)

	// OVS
	rootCmd.AddCommand(ovsCmd)

	// OVN
	rootCmd.AddCommand(ovnCmd)
	ovnCmd.Flags().StringP("nbdb", "n", "unix:/var/run/ovn/ovnnb_db.sock", "OVN NB database connection")
	ovnCmd.Flags().StringP("sbdb", "s", "unix:/var/run/ovn/ovnsb_db.sock", "OVN SB database connection") // TODO Override with OVN_NB_DB and OVN_SB_DB and OVN_RUNDIR
	//ovnCmd.Flags().StringP("ovs", "o", "", "Optional OVS DB to configure")
}

func initConfig() {
	lvl, _ := logrus.ParseLevel(logLevel)
	log.SetLevel(lvl)
}

func run_ovn(cmd *cobra.Command, args []string) {
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

	app = tview.NewApplication()
	pages := tview.NewPages()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			exit()
		}
		return event
	})

	mainPage(pages, true)
	//configPage(pages)
	welcomePage(pages, `OVN mode. Drop sampling has been enabled in the remote OVN cluster.
However, IPFIX configuration needs to be added to each chassis that you want to sample. To do that, run the following command on them:

ovs-vsctl --id=@br get Bridge br-int --
	  --id=@i create IPFIX targets=\"${HOST_IP}:2055\"
	  --  create Flow_Sample_Collector_Set bridge=@br id=1 ipfix=@i
`)

	app.SetRoot(pages, true).SetFocus(pages)
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

func run_ovs(cmd *cobra.Command, args []string) {
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
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			exit()
		}
		return event
	})

	mainPage(pages, false)
	configPage(pages)
	welcomePage(pages, `In "ovs" mode you'll be able to configure OvS IPFIX sampling as well as to visualize live OvS statistics`)

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

func run_listen(cmd *cobra.Command, args []string) {
	ipPort := ":2055"
	if len(args) == 1 {
		ipPort = args[0]
	}
	app = tview.NewApplication()
	pages := tview.NewPages()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			exit()
		}
		return event
	})

	mainPage(pages, false)
	//configPage(pages)
	welcomePage(pages, `In "listen" mode you must manually start an IPFIX exporter to send flows to this host.
In OpenvSwitch you can run something like:
"ovs-vsctl -- set Bridge br-int ipfix=@i \
           -- --id=@i create IPFIX targets=\"${HOST_IP}:2055\"

Note that if you had already started the IPFIX exporter, it might take some time (e.g: 10mins in OvS) before it sends us the Templates, without which we cannot
decode the IPFIX Flow Records. It is possible that re-starting the exporter helps.`)

	app.SetRoot(pages, true).SetFocus(pages)

	do_listen(ipPort)
}

func do_listen(address string) {
	nf, err := netflow.NewNFReader(1,
		"netflow://"+address,
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
