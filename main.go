package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"ghostman/selector"
	"ghostman/store"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	RequestMethodSelectorIndex int = iota
	RequestEnvironmentSelectorIndex
	UrlInputIndex
	// TODO: handle dynamic row like header and body
	ConfirmSelectorIndex
)

type Mode int

const (
	Dashboard Mode = iota
	// TODO: should provide help message in Edit mode
	Edit
	// TODO: should do a view mode when enter to view a list
)

type dashboardModel struct {
	list              list.Model
	methodConfig      selector.Model
	environmentConfig selector.Model
}

type editModel struct {
	requestId          *int64 // to differenciate insert and update in edit mode
	index              int
	requestMethod      selector.Model
	requestEnvironment selector.Model
	urlInput           textinput.Model
	confirm            selector.Model
}

type model struct {
	store     *store.Store
	mode      Mode
	dashboard dashboardModel // Mode == Dashboard
	edit      editModel      // Mode == Edit
}

type itemDelegate struct{}

func initialModel(store *store.Store) model {
	ti := textinput.New()
	ti.Placeholder = "Please input url"
	ti.CharLimit = 128
	ti.SetWidth(128)
	ti.Prompt = ""
	l := list.New(
		[]list.Item{},
		itemDelegate{},
		60,
		20,
	)
	config := store.FindConfig()
	l.Title = fmt.Sprintf("env: %s, method: %s", config.Environment, config.Method)
	l.SetShowStatusBar(false)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
			key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
			// TODO: implement this, should change configs data in the db
			key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "environment")),
			key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "method")),
		}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
			key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
			key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "environment")),
			key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "method")),
		}
	}
	m := model{
		store: store,
		mode:  Dashboard,
		dashboard: dashboardModel{
			list: l,
			// only used for state management, for for display
			methodConfig:      selector.New("mthodConfig", []string{"all", "get", "post"}),
			environmentConfig: selector.New("environmentConfig", []string{"all", "local", "test", "staging", "production"}),
		},
		edit: editModel{
			index:              0,
			requestMethod:      selector.New("Method", []string{"get", "post"}),
			requestEnvironment: selector.New("Environment", []string{"local", "test", "staging", "production"}),
			urlInput:           ti,
			confirm:            selector.New("", []string{"save", "cancel"}),
		},
	}
	m.tryPopulateListWithDB()
	m.dashboard.methodConfig.SetValue(config.Method)
	m.dashboard.environmentConfig.SetValue(config.Environment)
	isDark := true
	m.dashboard.list.Styles.Title = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("170"))
	m.dashboard.list.Styles.PaginationStyle = list.DefaultStyles(isDark).PaginationStyle.PaddingLeft(4)
	m.dashboard.list.Styles.HelpStyle = list.DefaultStyles(isDark).HelpStyle.PaddingLeft(4).PaddingBottom(2)
	m.dashboard.list.SetDelegate(itemDelegate{})
	return m
}

func (m model) Init() tea.Cmd { return nil }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case Dashboard:
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			// TODO: should add a add/update mode for create entry for url, sqlite3 would be great for store
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "e":
				req, ok := m.dashboard.list.SelectedItem().(store.RequestEntity)
				if ok {
					m.resetDashboardModel()
					m.edit.requestId = &req.Id
					m.edit.requestMethod.SetValue(req.Method)
					m.edit.requestEnvironment.SetValue(req.Environment)
					m.edit.urlInput.SetValue(req.Url)
					m.mode = Edit
				}
				return m, nil
			case "n":
				m.mode = Edit
				m.resetDashboardModel()
				return m, nil
			// when "E" and "M" success should render list title
			case "E":
				m.dashboard.environmentConfig.WrappingNext()
				newEnvironment := m.dashboard.environmentConfig.Value()
				if err := m.store.UpdateEnvironmentConfig(newEnvironment); err != nil {
					m.dashboard.environmentConfig.WrappingPrev() // should restore it's old state
					return m, nil
				}
				config := m.store.FindConfig()
				m.dashboard.list.Title = fmt.Sprintf("env: %s, method: %s", config.Environment, config.Method)
				m.tryPopulateListWithDB()
				return m, nil
			case "M":
				m.dashboard.methodConfig.WrappingNext()
				newMethod := m.dashboard.methodConfig.Value()
				if err := m.store.UpdateMethodConfig(newMethod); err != nil {
					m.dashboard.methodConfig.WrappingPrev() // should restore it's old state
					return m, nil
				}
				config := m.store.FindConfig()
				m.dashboard.list.Title = fmt.Sprintf("env: %s, method: %s", config.Environment, config.Method)
				m.tryPopulateListWithDB()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.dashboard.list, cmd = m.dashboard.list.Update(msg)
		return m, cmd
	case Edit:
		switch msg := msg.(type) {
		case tea.PasteMsg:
			if m.edit.urlInput.Focused() {
				var cmd tea.Cmd
				m.edit.urlInput, cmd = m.edit.urlInput.Update(msg)
				return m, cmd
			}
		case tea.KeyPressMsg:
			switch msg.String() {
			case "j", "down":
				if m.edit.index < ConfirmSelectorIndex && !m.edit.urlInput.Focused() {
					m.edit.index++
				}
			case "k", "up":
				// TODO: maybe need to change when adding headers and body
				if m.edit.index > 0 && !m.edit.urlInput.Focused() {
					m.edit.index--
				}
			case "l", "right":
				switch m.edit.index {
				case RequestMethodSelectorIndex:
					m.edit.requestMethod.Next()
				case RequestEnvironmentSelectorIndex:
					m.edit.requestEnvironment.Next()
				case ConfirmSelectorIndex:
					m.edit.confirm.Next()
				}
			case "h", "left":
				switch m.edit.index {
				case RequestMethodSelectorIndex:
					m.edit.requestMethod.Prev()
				case RequestEnvironmentSelectorIndex:
					m.edit.requestEnvironment.Prev()
				case ConfirmSelectorIndex:
					m.edit.confirm.Prev()
				}
			// not doing any return in above case, when we should not shadow key when user input
			case "enter":
				switch m.edit.index {
				case RequestMethodSelectorIndex, RequestEnvironmentSelectorIndex:
					m.edit.index++
				case UrlInputIndex:
					if m.edit.urlInput.Focused() {
						m.edit.urlInput.Blur()
					} else {
						m.edit.urlInput.Focus()
					}
				case ConfirmSelectorIndex:
					if m.edit.confirm.Value() == "save" {
						if _, err := m.store.UpsertRequest(store.UpsertRequestParams{
							Id:          m.edit.requestId,
							Url:         m.edit.urlInput.Value(),
							Method:      m.edit.requestMethod.Value(),
							Environment: m.edit.requestEnvironment.Value(),
						}); err != nil {
							log.Println("Update: ", err)
						} else {
							m.tryPopulateListWithDB()
						}
					}
					m.resetEditModel()
					m.mode = Dashboard
				}
				return m, nil
			case "esc", "ctrl+c":
				if m.edit.index == UrlInputIndex && m.edit.urlInput.Focused() {
					m.edit.urlInput.Blur()
					return m, nil
				}
			}
		}
		var cmd tea.Cmd
		m.edit.urlInput, cmd = m.edit.urlInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() tea.View {
	switch m.mode {
	case Dashboard:
		return tea.NewView("\n" + m.dashboard.list.View())
	case Edit:
		var sb strings.Builder
		sb.WriteString(m.edit.requestMethod.View(m.edit.index == RequestMethodSelectorIndex))
		sb.WriteString(m.edit.requestEnvironment.View(m.edit.index == RequestEnvironmentSelectorIndex))
		// TODO: in get url and has query parameters should reflect that in the url?
		if m.edit.index == UrlInputIndex {
			color := "255"
			if m.edit.urlInput.Focused() {
				color = "170"
			}
			sb.WriteString(lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color(color)).Render("> Url: " + m.edit.urlInput.View()))
		} else {
			sb.WriteString(lipgloss.NewStyle().PaddingLeft(4).Render("Url: " + m.edit.urlInput.View()))
		}
		sb.WriteString(lipgloss.NewStyle().PaddingTop(1).Render(m.edit.confirm.View(m.edit.index == ConfirmSelectorIndex)))
		return tea.NewView(lipgloss.NewStyle().PaddingTop(1).Render(sb.String()))
	}
	return tea.NewView("[ERROR] should never reach here")
}

func (m *model) resetEditModel() {
	m.edit.requestId = nil
	m.edit.index = 0
	m.edit.requestMethod.Reset()
	m.edit.requestEnvironment.Reset()
	m.edit.urlInput.Reset()
	m.edit.confirm.Reset()
}

// TODO: when in filter mode should disable all keybind but enter or edit
func (m *model) resetDashboardModel() {
	m.dashboard.list.ResetSelected()
	m.dashboard.list.ResetFilter()
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(store.RequestEntity)
	if !ok {
		return
	}
	str := fmt.Sprintf("%s", i.Url)

	if index == m.Index() {
		str = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170")).Render("> " + str)
	} else {
		str = lipgloss.NewStyle().PaddingLeft(4).Render(str)
	}

	fmt.Fprint(w, str)
}

// If fail then not populate, handle error inside this function
func (m *model) tryPopulateListWithDB() {
	reqs, err := m.store.FindRequests()
	if err != nil {
		log.Println("tryPopulateListWithDB: ", err)
		return
	}
	items := []list.Item{}
	for _, req := range reqs {
		items = append(items, req)
	}
	m.dashboard.list.SetItems(items)
}

func main() {
	if len(os.Getenv("DEBUG")) > 0 {
		f, err := tea.LogToFile("dev.log", "[DEBUG]")
		if err != nil {
			fmt.Println("[FATAL] fail to log to file: ", err)
			os.Exit(1)
		}
		defer f.Close()
	}
	store, err := store.New()
	if err != nil {
		fmt.Println("[FATAL] fail to initialize store: ", err)
		os.Exit(1)
	}
	defer store.Close()
	if _, err := tea.NewProgram(initialModel(store)).Run(); err != nil {
		fmt.Println("[FATAL] fail to run ghostman: ", err)
		os.Exit(1)
	}
}
