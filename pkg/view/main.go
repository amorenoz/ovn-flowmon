package view

import (
	"amorenoz/ovs-flowmon/pkg/stats"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

type MainPageConfig struct {
	FlowTable *FlowTable
	Stats     *stats.StatsView
	ExtraMenu func(menu *tview.List, log *logrus.Logger) error
	OnExit    func()
}

var defaultConfig MainPageConfig = MainPageConfig{
	ExtraMenu: nil,
}

// Generate the Main Page
// The main menu is composed of the top menu, the stats viewer and the flowtable.
// Certain callbacks and needed data are provided by MainPageConfig.
func MainPage(app *tview.Application, pages *tview.Pages, config MainPageConfig, log *logrus.Logger) {
	status := tview.NewTextView().
		SetDynamicColors(true).
		SetText("Stopped. Press Start to start capturing\n")
	log.SetOutput(TextViewLogWriter(status))

	// Top Menu
	menu := tview.NewFlex()
	menuList := tview.NewList().
		ShowSecondaryText(false)
	menu.AddItem(menuList, 0, 2, true)
	menu.AddItem(config.Stats.View(), 0, 2, false)

	exit := func() {
		log.Info("Stopping app")
		if config.OnExit != nil {
			config.OnExit()
		}
		if app != nil {
			app.Stop()
		}
	}
	logs := func() {
		app.SetFocus(status)
	}
	flows := func() {
		app.SetFocus(config.FlowTable.View)
	}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			exit()
		}
		return event
	})

	show_aggregate := func() {
		// Make columns selectable
		config.FlowTable.SetSelectMode(ModeColsKeys)
		app.SetFocus(config.FlowTable.View)
		config.FlowTable.View.SetSelectedFunc(func(row, col int) {
			config.FlowTable.ToggleAggregate(col)
			config.FlowTable.SetSelectMode(ModeRows)
			config.FlowTable.View.SetSelectedFunc(func(row, col int) {
				app.SetFocus(menuList)
			})
			app.SetFocus(menuList)
		})
	}

	sort_by := func() {
		// Make columns selectable
		config.FlowTable.SetSelectMode(ModeColsAll)
		app.SetFocus(config.FlowTable.View)
		config.FlowTable.View.SetSelectedFunc(func(row, col int) {
			err := config.FlowTable.SetSortingColumn(col)
			if err != nil {
				log.Error(err)
			}
			config.FlowTable.SetSelectMode(ModeRows)
			config.FlowTable.View.SetSelectedFunc(func(row, col int) {
				app.SetFocus(menuList)
			})
			app.SetFocus(menuList)
		})
	}

	if config.ExtraMenu != nil {
		config.ExtraMenu(menuList, log)
	}

	menuList.AddItem("Flows", "", 'f', flows).
		AddItem("Add/Remove Fields from aggregate", "", 'a', show_aggregate).
		AddItem("Sort by ", "", 's', sort_by).
		AddItem("Logs", "", 'l', logs).
		AddItem("Exit", "", 'e', exit)
	config.FlowTable.View.SetDoneFunc(func(key tcell.Key) {
		app.SetFocus(menuList)
	})
	config.FlowTable.View.SetSelectedFunc(func(row, col int) {
		app.SetFocus(menuList)
	})
	status.SetDoneFunc(func(key tcell.Key) {
		app.SetFocus(menuList)
	})
	menuList.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Menu")
	config.FlowTable.View.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Flows")
	status.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Logs")

	flex := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(menu, 0, 2, true).AddItem(config.FlowTable.View, 0, 5, false).AddItem(status, 0, 1, false)

	pages.AddPage("main", flex, true, false)

}
