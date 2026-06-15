package main

import (
	"fmt"
	"io"
	"os"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type (
	RequestMethod      int
	RequestEnvironment int
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

type styles struct {
	title        lipgloss.Style
	item         lipgloss.Style
	selectedItem lipgloss.Style
	pagination   lipgloss.Style
	help         lipgloss.Style
}

type model struct {
	list         list.Model
	selectedItem string
	styles       styles
}

type item struct {
	url    string
	method RequestMethod
	env    RequestEnvironment
}

type itemDelegate struct {
	styles *styles
}

func newStyles(darkBG bool) styles {
	return styles{
		title:        lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("170")),
		item:         lipgloss.NewStyle().PaddingLeft(4),
		selectedItem: lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170")),
		pagination:   list.DefaultStyles(darkBG).PaginationStyle.PaddingLeft(4),
		help:         list.DefaultStyles(darkBG).HelpStyle.PaddingLeft(4).PaddingBottom(2),
	}
}

func (m *model) updateStyles(isDark bool) {
	m.styles = newStyles(isDark)
	m.list.Styles.Title = m.styles.title
	m.list.Styles.PaginationStyle = m.styles.pagination
	m.list.Styles.HelpStyle = m.styles.help
	m.list.SetDelegate(itemDelegate{styles: &m.styles})
}

func (m model) Init() tea.Cmd { return nil }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// TODO: should add a add/update mode for create entry for url, sqlite3 would be great for store
		switch key := msg.String(); key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			i, ok := m.list.SelectedItem().(item)
			if ok {
				m.selectedItem = i.url
			}
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() tea.View {
	if m.selectedItem != "" {
		return tea.NewView(lipgloss.NewStyle().Render(fmt.Sprintf("%s\n", m.selectedItem)))
	}
	return tea.NewView("\n" + m.list.View())
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
		str = d.styles.selectedItem.Render("> " + str)
	} else {
		str = d.styles.item.Render(str)
	}

	fmt.Fprint(w, str)
}

func newModel() model {
	// TODO: should get it from store
	items := []list.Item{
		item{url: "https://backend/something", method: Post, env: Test},
		item{url: "http://localhost:1234/whoami", method: Get, env: Local},
		item{url: "http://localhost:5678/foo/bar", method: Post, env: Local},
	}
	l := list.New(items, itemDelegate{}, 20, 16)
	// TODO: get this from config table
	l.Title = "[Staging] [Get]"
	l.SetShowStatusBar(false)

	m := model{list: l}
	m.updateStyles(true)
	return m
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Println("[ERROR] ghostman running fail:", err)
		os.Exit(1)
	}
}
