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
	Edit
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

type model struct {
	mode         Mode
	list         list.Model
	selectors    []selector.Model
	textInput    textinput.Model
	selectedItem string
	rowIndex     int
}

type item struct {
	url    string
	method RequestMethod
	env    RequestEnvironment
}

type itemDelegate struct{}

func initialModel() model {
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
	selectors := []selector.Model{
		selector.New("Method", []string{Get.String(), Post.String()}),
		selector.New("Environment", []string{Local.String(), Test.String(), Staging.String(), Production.String()}),
	}
	ti := textinput.New()
	ti.Placeholder = "Please input url"
	ti.CharLimit = 128
	ti.SetWidth(128)
	ti.Prompt = ""
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

	isDark := true
	m := model{mode: Dashboard, list: l, selectors: selectors, textInput: ti, rowIndex: 0}
	m.list.Styles.Title = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("170"))
	m.list.Styles.PaginationStyle = list.DefaultStyles(isDark).PaginationStyle.PaddingLeft(4)
	m.list.Styles.HelpStyle = list.DefaultStyles(isDark).HelpStyle.PaddingLeft(4).PaddingBottom(2)
	m.list.SetDelegate(itemDelegate{})
	return m
}

func (m model) Init() tea.Cmd { return nil }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()
		// TODO: should add a add/update mode for create entry for url, sqlite3 would be great for store
		switch m.mode {
		case Dashboard:
			switch key {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "enter":
				i, ok := m.list.SelectedItem().(item)
				if ok {
					m.selectedItem = i.url
					m.mode = Edit
				}
				return m, nil
			case "n":
				m.mode = Edit
				return m, nil
			}
		case Edit:
			switch key {
			case "ctrl+n", "down", "enter":
				if m.rowIndex < len(m.selectors)-1 {
					m.rowIndex++
				}
			case "ctrl+p", "up":
				if m.rowIndex > 0 {
					m.rowIndex--
				}
			case "ctrl+f", "right":
				m.selectors[m.rowIndex].Next()
				return m, nil
			case "ctrl+b", "left":
				m.selectors[m.rowIndex].Prev()
				return m, nil
			case "esc":
				m.mode = Dashboard
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() tea.View {
	switch m.mode {
	case Dashboard:
		return tea.NewView("\n" + m.list.View())
	default: // Edit
		var sb strings.Builder
		for i, selector := range m.selectors {
			sb.WriteString(selector.View(i == m.rowIndex))
		}
		fmt.Fprintf(&sb, "%surl: %s", strings.Repeat(" ", 4), m.textInput.View())
		return tea.NewView(lipgloss.NewStyle().PaddingTop(1).Render(sb.String()))
	}
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
