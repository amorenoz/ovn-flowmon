package view

import (
	"amorenoz/ovs-flowmon/pkg/stats"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

// PageName is a string representing the differnt pages in the application.
type PageName = string

const (
	// The MainPage is the applications's Main view with the menu and the flow viewer.
	MainPage PageName = "main"
	// WelcomePage is the first page that is shown with a welcome message. After pressing any key, this page
	// is hidden and is never shown again.
	Welcome PageName = "welcome"
)

// App represents the main FlowMonitoring Application
type App struct {
	log *logrus.Logger
	app *tview.Application

	// Visual elements
	pages     *tview.Pages
	flowTable *FlowTable
	stats     *stats.StatsView
	status    *tview.TextView
	menu      *tview.List

	// Callbacks
	extraMenu func(menu *tview.List, log *logrus.Logger) error
	onExit    func()
}

// App() returns the underlying tview Application.
// It can be useful to enqueue draws or register extra event handlers.
func (m *App) App() *tview.Application {
	return m.app
}

// FlowTable returns the FlowTable element.
func (m *App) FlowTable() *FlowTable {
	return m.flowTable
}

// Stats returns the aplications's Stats View.
func (m *App) Stats() *stats.StatsView {
	return m.stats
}

// OnExit configures the OnExit callback.
// This callback will be called just before the application exits.
func (m *App) OnExit(fn func()) *App {
	m.onExit = fn
	return m
}

// ExtraMenu configures the ExtraMenu callback.
// This callback will be when building the application. It allows the user to insert
// additional elements in the main menu.
func (m *App) ExtraMenu(fn func(menu *tview.List, log *logrus.Logger) error) *App {
	m.extraMenu = fn
	return m
}

// ShowPage makes the specified page visible.
func (m *App) ShowPage(name PageName) {
	m.log.Debugf("Showing page: %s", name)
	if !m.pages.HasPage(name) {
		m.log.Errorf("Do not have page: %s", name)
	}
	m.pages.ShowPage(name)
	m.pages.SendToFront(name)
}

// ShowPage makes the specified page invisible.
func (m *App) HidePage(name PageName) {
	m.log.Debugf("Hiding page: %s", name)
	if !m.pages.HasPage(name) {
		m.log.Errorf("Do not have page: %s", name)
	}
	m.pages.HidePage(name)
}

// WelcomePage sets a Welcome message before the application starts. The content
// of the message can be specified.
func (m *App) WelcomePage(message string) {
	welcome := tview.NewModal().SetText(`

Welcome to OvS Flow Monitor!

` + message + `


... hit Enter to start monitoring!
`).AddButtons([]string{"Start"}).SetDoneFunc(func(index int, label string) {
		m.pages.HidePage(Welcome)
		m.pages.ShowPage(MainPage)
	}).
		SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	m.pages.AddPage(Welcome, welcome, true, true)
}

// AddPage adds a new page to the application. The user has to control when the page is shown using
// ShowPage() and hide it when appropriate.
func (m *App) AddPage(name PageName, obj tview.Primitive, resize, visible bool) {
	m.pages.AddPage(string(name), obj, resize, visible)
}

// NewApp returns a new Application
// The main menu is composed of the top menu, the stats viewer and the flowtable.
func NewApp(log *logrus.Logger) *App {
	app := tview.NewApplication()
	stats := stats.NewStatsView(app)
	flowTable := NewFlowTable().SetStatsBackend(stats)
	pages := tview.NewPages()
	status := tview.NewTextView().
		SetDynamicColors(true).
		SetText("Stopped. Press Start to start capturing\n")
	menu := tview.NewList().
		ShowSecondaryText(false)

	mainPage := &App{
		log:       log,
		app:       app,
		pages:     pages,
		flowTable: flowTable,
		stats:     stats,
		status:    status,
		menu:      menu,
	}
	return mainPage
}

func (m *App) build() error {
	// Build Top Menu
	topBar := tview.NewFlex()
	topBar.AddItem(m.menu, 0, 2, true)
	topBar.AddItem(m.stats.View(), 0, 2, false)

	// Short callbacks for main menu buttons
	logs := func() {
		m.app.SetFocus(m.status)
	}
	flows := func() {
		m.app.SetFocus(m.flowTable.View)
	}

	if m.extraMenu != nil {
		m.log.Debug("Creating extra menu")
		m.extraMenu(m.menu, m.log)
	}

	m.menu.AddItem("Flows", "", 'f', flows).
		AddItem("Add/Remove Fields from aggregate", "", 'a', m.showAggregate).
		AddItem("Sort by ", "", 's', m.sortBy).
		AddItem("Logs", "", 'l', logs).
		AddItem("Exit", "", 'e', m.exit)

	// Configure movement between sections
	m.flowTable.View.SetDoneFunc(func(key tcell.Key) {
		m.app.SetFocus(m.menu)
	})
	m.flowTable.View.SetSelectedFunc(func(row, col int) {
		m.app.SetFocus(m.menu)
	})
	m.status.SetDoneFunc(func(key tcell.Key) {
		m.app.SetFocus(m.menu)
	})

	// Assemble everything
	m.menu.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Menu")
	m.flowTable.View.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Flows")
	m.status.SetBorder(true).SetBorderPadding(1, 1, 2, 0).SetTitle("Logs")
	flex := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(topBar, 0, 2, true).AddItem(m.flowTable.View, 0, 5, false).AddItem(m.status, 0, 1, false)

	m.pages.AddPage(MainPage, flex, true, false)
	m.app.SetRoot(m.pages, true).SetFocus(m.pages)

	// Configure Ctr-C callback.
	m.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			m.exit()
		}
		return event
	})
	return nil
}

// Interactive callbacks
// Called when the user hits exit on the main menu.
func (m *App) exit() {
	m.log.Info("Stopping app")
	if m.onExit != nil {
		m.onExit()
	}
	if m.app != nil {
		m.app.Stop()
	}
}

// Called when user hits the Add/Remove from aggregates button.
// It makes the columns of the flowtable selectable and when a column
// is selected, it calls ToggleAggregate on the flowTable
func (m *App) showAggregate() {
	m.flowTable.SetSelectMode(ModeColsKeys)
	m.app.SetFocus(m.flowTable.View)
	m.flowTable.View.SetSelectedFunc(func(row, col int) {
		m.flowTable.ToggleAggregate(col)
		m.flowTable.SetSelectMode(ModeRows)
		m.flowTable.View.SetSelectedFunc(func(row, col int) {
			m.app.SetFocus(m.menu)
		})
		m.app.SetFocus(m.menu)
	})
}

// Called when user hits SortBy button. It makes columns of the flowTables
// selectable and, when one is selected, calls SetSortingColumn.
func (m *App) sortBy() {
	m.flowTable.SetSelectMode(ModeColsAll)
	m.app.SetFocus(m.flowTable.View)
	m.flowTable.View.SetSelectedFunc(func(row, col int) {
		err := m.flowTable.SetSortingColumn(col)
		if err != nil {
			m.log.Error(err)
		}
		m.flowTable.SetSelectMode(ModeRows)
		m.flowTable.View.SetSelectedFunc(func(row, col int) {
			m.app.SetFocus(m.menu)
		})
		m.app.SetFocus(m.menu)
	})
}

// Run the main application
func (m *App) Run() error {
	if err := m.build(); err != nil {
		return err
	}
	m.log.SetOutput(TextViewLogWriter(m.status))
	if err := m.app.Run(); err != nil {
		return err
	}
	return nil
}
