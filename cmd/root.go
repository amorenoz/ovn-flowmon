package cmd

import (
	"amorenoz/ovs-flowmon/pkg/ovs"
	"amorenoz/ovs-flowmon/pkg/stats"
	"amorenoz/ovs-flowmon/pkg/view"

	_ "github.com/netsampler/goflow2/format/protobuf"
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
)

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
	ovnCmd.Flags().StringP("ovs", "o", "", "Optional OVS DB to configure")
}

func initConfig() {
	lvl, _ := logrus.ParseLevel(logLevel)
	log.SetLevel(lvl)
}
