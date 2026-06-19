package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"ghostman/selector"

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

type (
	RequestMethod      int
	RequestEnvironment int
	Mode               int
)

const (
	Get RequestMethod = iota
	Post
)

const (
	Local RequestEnvironment = iota
	Test
	Staging
	Production
	All
)

const (
	Dashboard Mode = iota
	// TODO: should provide help message in Edit mode
	Edit
	// TODO: should do a view mode when enter to view a list
)

var requestMethodName = map[RequestMethod]string{
	Get:  "get",
	Post: "post",
}

var requestEnvironmentName = map[RequestEnvironment]string{
	Local:      "local",
	Test:       "test",
	Staging:    "staging",
	Production: "production",
	All:        "all",
}

func (rm RequestMethod) String() string      { return requestMethodName[rm] }
func (re RequestEnvironment) String() string { return requestEnvironmentName[re] }

type dashboardModel struct {
	list list.Model
}

type editModel struct {
	index              int
	requestMethod      selector.Model
	requestEnvironment selector.Model
	urlInput           textinput.Model
	confirm            selector.Model
}

type model struct {
	mode      Mode
	dashboard dashboardModel // Mode == Dashboard
	edit      editModel      // Mode == Edit
}

// TODO: should has extra info about headers and body
type item struct {
	id     int
	url    string
	method RequestMethod
	env    RequestEnvironment
}

type itemDelegate struct{}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Please input url"
	ti.CharLimit = 128
	ti.SetWidth(128)
	ti.Prompt = ""
	// TODO: should get it from store
	l := list.New(
		[]list.Item{
			item{url: "https://backend/something", method: Post, env: Test},
			item{url: "http://localhost:1234/whoami", method: Get, env: Local},
			item{url: "http://localhost:5678/foo/bar", method: Post, env: Local},
		},
		itemDelegate{},
		20,
		16,
	)
	// TODO: get this from config table
	l.Title = "[Staging] [Get]"
	l.SetShowStatusBar(false)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new request")),
		}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new request")),
		}
	}
	m := model{
		mode:      Dashboard,
		dashboard: dashboardModel{list: l},
		edit: editModel{
			index:              0,
			requestMethod:      selector.New("Method", []string{Get.String(), Post.String()}),
			requestEnvironment: selector.New("Environment", []string{Local.String(), Test.String(), Staging.String(), Production.String()}),
			urlInput:           ti,
			confirm:            selector.New("", []string{"save", "cancel"}),
		},
	}
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
			case "enter":
				_, ok := m.dashboard.list.SelectedItem().(item)
				if ok {
					m.mode = Edit
					m.resetDashboardModel()
				}
				return m, nil
			case "n":
				m.mode = Edit
				m.resetDashboardModel()
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
				// TODO: should reset the list status here
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
					if m.edit.confirm.Choice() == "save" {
						// TODO: do a save to db if in edit mode, but in view mode should just save whatever user has updated
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
		// render edit progress selector
		sb.WriteString(lipgloss.NewStyle().PaddingTop(1).Render(m.edit.confirm.View(m.edit.index == ConfirmSelectorIndex)))
		return tea.NewView(lipgloss.NewStyle().PaddingTop(1).Render(sb.String()))
	}
	return tea.NewView("[ERROR] should never reach here")
}

func (m *model) resetEditModel() {
	m.edit.index = 0
	m.edit.requestMethod.Reset()
	m.edit.requestEnvironment.Reset()
	m.edit.urlInput.Reset()
	m.edit.confirm.Reset()
}

func (m *model) resetDashboardModel() {
	m.dashboard.list.ResetSelected()
	m.dashboard.list.ResetFilter()
}

func (i item) FilterValue() string { return string(i.url) }

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}
	str := fmt.Sprintf("%s", i.url)

	if index == m.Index() {
		str = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170")).Render("> " + str)
	} else {
		str = lipgloss.NewStyle().PaddingLeft(4).Render(str)
	}

	fmt.Fprint(w, str)
}

func main() {
	if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		fmt.Println("[ERROR] ghostman running fail:", err)
		os.Exit(1)
	}
}
