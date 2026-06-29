package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"ghostman/selector"
	"ghostman/store"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	MethodIndex int = iota
	EnvironmentIndex
	UrlIndex
	QueryParamsIndex
	BodyIndex
	ConfirmIndex
)

type Mode int

const (
	Dashboard Mode = iota
	// TODO: should provide help message in Edit mode
	Edit
	Result
	// TODO: Better name
	ParamEdit
)

type dashboardModel struct {
	list              list.Model
	methodConfig      selector.Model
	environmentConfig selector.Model
}

type editModel struct {
	index       int
	id          *int64 // to differenciate insert and update in edit mode
	method      selector.Model
	environment selector.Model
	url         textinput.Model
	confirm     selector.Model // TODO: maybe just keep it as cmd+s or ctrl + s to save
}

type resultModel struct {
	view viewport.Model
}

type paramEditModel struct {
	queryParamInput textarea.Model
	bodyInput       textarea.Model
}

// TODO: paramEdit mode should put inside edit mode
type model struct {
	store     *store.Store
	mode      Mode
	dashboard dashboardModel // Mode == Dashboard
	edit      editModel      // Mode == Edit
	result    resultModel    // Mode == Result
	paramEdit paramEditModel // Mode == ParamEdit
}

type itemDelegate struct{}

func initialModel(store *store.Store) model {
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
	url := textinput.New()
	url.CharLimit = 128
	url.SetWidth(128)
	url.Prompt = ""
	url.Placeholder = "NULL"
	queryParamInput := textarea.New()
	queryParamInput.SetHeight(20)
	queryParamInputStyles := queryParamInput.Styles()
	queryParamInputStyles.Cursor.Blink = false
	queryParamInput.SetStyles(queryParamInputStyles)
	bodyInput := textarea.New()
	bodyInput.SetHeight(20)
	bodyInputStyles := bodyInput.Styles()
	bodyInputStyles.Cursor.Blink = false
	bodyInput.SetStyles(bodyInputStyles)
	m := model{
		store: store,
		mode:  Dashboard,
		dashboard: dashboardModel{
			list: l,
			// only used for state management, not for display
			methodConfig:      selector.New("mthodConfig", []string{"all", "get", "post"}),
			environmentConfig: selector.New("environmentConfig", []string{"all", "local", "test", "staging", "production"}),
		},
		edit: editModel{
			index:       0,
			method:      selector.New("Method", []string{"get", "post"}),
			environment: selector.New("Environment", []string{"local", "test", "staging", "production"}),
			url:         url,
			confirm:     selector.New("", []string{"save", "cancel"}),
		},
		result:    resultModel{view: viewport.New()},
		paramEdit: paramEditModel{queryParamInput: queryParamInput, bodyInput: bodyInput},
	}
	m.tryPopulateListWithDB()
	m.dashboard.methodConfig.SetValue(config.Method)
	m.dashboard.environmentConfig.SetValue(config.Environment)
	m.dashboard.list.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	m.dashboard.list.SetDelegate(itemDelegate{})
	return m
}

func (m model) Init() tea.Cmd { return nil }
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.dashboard.list.SetSize(msg.Width, msg.Height)
		m.result.view.SetWidth(msg.Width)
		m.result.view.SetHeight(msg.Height)
		m.paramEdit.queryParamInput.SetWidth(msg.Width)
		m.paramEdit.bodyInput.SetWidth(msg.Width)
	}
	switch m.mode {
	case Dashboard:
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "e":
				if !m.dashboard.list.SettingFilter() {
					req, ok := m.dashboard.list.SelectedItem().(store.RequestEntity)
					if ok {
						m.edit.id = &req.Id
						m.edit.method.SetValue(req.Method)
						m.edit.environment.SetValue(req.Environment)
						m.edit.url.SetValue(req.Url)
						// TODO: too much code duplicate
						queryParamsText := req.QueryParams
						queryParams := make(map[string]any)
						var queryParamsInputContent strings.Builder
						if err := json.Unmarshal([]byte(queryParamsText), &queryParams); err == nil {
							for k, v := range queryParams {
								// TODO: how to handle object/array
								fmt.Fprintf(&queryParamsInputContent, "%v\n%v\n", k, v)
							}
						}
						queryParamsTrim := queryParamsInputContent.String()
						if len(queryParamsTrim) > 0 {
							queryParamsTrim = queryParamsTrim[:len(queryParamsTrim)-1]
						}
						m.paramEdit.queryParamInput.SetValue(queryParamsTrim)
						bodyText := req.Body
						body := make(map[string]any)
						var bodyInputContent strings.Builder
						if err := json.Unmarshal([]byte(bodyText), &body); err == nil {
							for k, v := range body {
								fmt.Fprintf(&bodyInputContent, "%v\n%v\n", k, v)
							}
						}
						bodyTrim := bodyInputContent.String()
						if len(bodyTrim) > 0 {
							bodyTrim = bodyTrim[:len(bodyTrim)-1]
						}
						m.paramEdit.bodyInput.SetValue(bodyTrim)
						m.mode = Edit
					}
				}
			case "n":
				if !m.dashboard.list.SettingFilter() {
					m.mode = Edit
					m.resetDashboardModel()
				}
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
			case "enter":
				// TODO: should add a buffering status in the view mode, since right now could triger multiple requests by hitting enter
				req, ok := m.dashboard.list.SelectedItem().(store.RequestEntity)
				if ok {
					content, err := m.doRequest(req)
					if err != nil {
						log.Printf("doRequest: %v", err)
						return m, nil
					}
					width := m.result.view.Width()
					warpppedContent := lipgloss.NewStyle().Width(width).Render(content)
					m.result.view.SetContent(warpppedContent)
					m.mode = Result
				}
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.dashboard.list, cmd = m.dashboard.list.Update(msg)
		return m, cmd
	case Edit:
		switch msg := msg.(type) {
		case tea.PasteMsg:
			if m.edit.url.Focused() {
				var cmd tea.Cmd
				m.edit.url, cmd = m.edit.url.Update(msg)
				return m, cmd
			}
		case tea.KeyPressMsg:
			switch msg.String() {
			case "j", "down":
				if m.edit.index < ConfirmIndex && !m.edit.url.Focused() {
					m.edit.index++
				}
			case "k", "up":
				// TODO: maybe need to change when adding headers and body
				if m.edit.index > 0 && !m.edit.url.Focused() {
					m.edit.index--
				}
			case "l", "right":
				switch m.edit.index {
				case MethodIndex:
					m.edit.method.Next()
				case EnvironmentIndex:
					m.edit.environment.Next()
				case ConfirmIndex:
					m.edit.confirm.Next()
				}
			case "h", "left":
				switch m.edit.index {
				case MethodIndex:
					m.edit.method.Prev()
				case EnvironmentIndex:
					m.edit.environment.Prev()
				case ConfirmIndex:
					m.edit.confirm.Prev()
				}
			// not doing any return in above case, when we should not shadow key when user input
			case "enter":
				switch m.edit.index {
				case MethodIndex, EnvironmentIndex:
					m.edit.index++
				case UrlIndex:
					if m.edit.url.Focused() {
						m.edit.url.Blur()
					} else {
						m.edit.url.Focus()
					}
				case QueryParamsIndex:
					m.paramEdit.queryParamInput.Focus()
					m.mode = ParamEdit
					// TODO: set value from db
				case BodyIndex:
					m.paramEdit.bodyInput.Focus()
					m.mode = ParamEdit
				case ConfirmIndex:
					if m.edit.confirm.Value() == "save" && len(m.edit.url.Value()) > 0 {
						if _, err := m.store.UpsertRequest(store.UpsertRequestParams{
							Id:          m.edit.id,
							Url:         m.edit.url.Value(),
							Method:      m.edit.method.Value(),
							Environment: m.edit.environment.Value(),
							QueryParams: textareaContentAsJSON(&m.paramEdit.queryParamInput),
							Body:        textareaContentAsJSON(&m.paramEdit.bodyInput),
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
				if m.edit.index == UrlIndex && m.edit.url.Focused() {
					m.edit.url.Blur()
					return m, nil
				}
				m.resetEditModel()
				m.mode = Dashboard
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.edit.url, cmd = m.edit.url.Update(msg)
		return m, cmd
	case Result:
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				m.mode = Dashboard
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.result.view, cmd = m.result.view.Update(msg)
		return m, cmd
	case ParamEdit:
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case "ctrl+c", "esc":
				m.mode = Edit
				var input *textarea.Model
				switch m.edit.index {
				case QueryParamsIndex:
					input = &m.paramEdit.queryParamInput
				case BodyIndex:
					input = &m.paramEdit.bodyInput
				}
				v := input.Value()
				nLines := input.LineCount()
				// TODO: handle doubleQuote, singleQuote, backQuotes
				if nLines > 0 && nLines%2 != 0 {
					lines := strings.Split(strings.ReplaceAll(v, "\r\n", "\n"), "\n")
					lines = lines[:nLines-1]
					v = strings.Join(lines, "\n")
				}
				input.SetValue(v)
				return m, nil
			}
		}
		var cmd tea.Cmd
		switch m.edit.index {
		case QueryParamsIndex:
			m.paramEdit.queryParamInput, cmd = m.paramEdit.queryParamInput.Update(msg)
		case BodyIndex:
			m.paramEdit.bodyInput, cmd = m.paramEdit.bodyInput.Update(msg)
		}
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
		sb.WriteString(m.edit.method.View(m.edit.index == MethodIndex))
		sb.WriteString(m.edit.environment.View(m.edit.index == EnvironmentIndex))
		// TODO: in get url and has query parameters should reflect that in the url?
		urlColor := "255"
		urlTitle := "Url: "
		urlPaddingLeft := 1
		if m.edit.index == UrlIndex {
			if m.edit.url.Focused() {
				urlColor = "170"
			}
			urlTitle = ">" + urlTitle
			urlPaddingLeft = 0
		}
		sb.WriteString(lipgloss.NewStyle().PaddingLeft(urlPaddingLeft).Foreground(lipgloss.Color(urlColor)).Render(urlTitle + m.edit.url.View()))
		sb.WriteByte('\n')
		queryParamInputTitle := "QueryParameter: "
		queryParamInputPaddingLeft := 1
		if m.edit.index == QueryParamsIndex {
			queryParamInputTitle = ">" + queryParamInputTitle
			queryParamInputPaddingLeft = 0
		}
		var queryParamInputContent strings.Builder
		queryParamInputLines := strings.Split(m.paramEdit.queryParamInput.Value(), "\n")
		queryParams := make(map[string]any)
		// since empty strings.Split("", "\n") -> [""]
		if len(queryParamInputLines) < 2 {
			queryParamInputContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("NULL"))
		} else {
			for i := 0; i < len(queryParamInputLines); i += 2 {
				prefix, suffix := "", ""
				if i > 0 {
					prefix = strings.Repeat(" ", len(queryParamInputTitle))
				}
				if i+1 != len(queryParamInputLines)-1 {
					suffix = "\n"
				}
				k, v := queryParamInputLines[i], queryParamInputLines[i+1]
				queryParams[k] = v
				fmt.Fprintf(&queryParamInputContent, "%s\"%s\": \"%s\"%s", prefix, k, v, suffix)
			}
		}
		sb.WriteString(lipgloss.NewStyle().PaddingLeft(queryParamInputPaddingLeft).Render(queryParamInputTitle + queryParamInputContent.String()))
		sb.WriteByte('\n')
		bodyInputTitle := "Body: "
		bodyInputPaddingLeft := 1
		if m.edit.index == BodyIndex {
			bodyInputTitle = ">" + bodyInputTitle
			bodyInputPaddingLeft = 0
		}
		var bodyInputContent strings.Builder
		bodyInputLines := strings.Split(m.paramEdit.bodyInput.Value(), "\n")
		bodies := make(map[string]any)
		if len(bodyInputLines) < 2 {
			bodyInputContent.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("NULL"))
		} else {
			for i := 0; i < len(bodyInputLines); i += 2 {
				prefix, suffix := "", ""
				if i > 0 {
					prefix = strings.Repeat(" ", len(bodyInputTitle))
				}
				if i+1 != len(bodyInputLines)-1 {
					suffix = "\n"
				}
				k, v := bodyInputLines[i], bodyInputLines[i+1]
				bodies[k] = v
				fmt.Fprintf(&bodyInputContent, "%s\"%s\": \"%s\"%s", prefix, k, v, suffix)
			}
		}
		sb.WriteString(lipgloss.NewStyle().PaddingLeft(bodyInputPaddingLeft).Render(bodyInputTitle + bodyInputContent.String()))
		sb.WriteByte('\n')
		sb.WriteString(lipgloss.NewStyle().Render(m.edit.confirm.View(m.edit.index == ConfirmIndex)))
		return tea.NewView(lipgloss.NewStyle().Render(sb.String()))
	case Result:
		v := tea.NewView(m.result.view.View())
		v.AltScreen = true
		return v
	case ParamEdit:
		switch m.edit.index {
		case QueryParamsIndex:
			return tea.NewView(m.paramEdit.queryParamInput.View())
		case BodyIndex:
			return tea.NewView(m.paramEdit.bodyInput.View())
		}
	}
	return tea.NewView("[ERROR] should never reach here\n")
}

func (m *model) resetEditModel() {
	m.edit.id = nil
	m.edit.index = 0
	m.edit.method.Reset()
	m.edit.environment.Reset()
	m.edit.url.Reset()
	m.edit.confirm.Reset()
	m.paramEdit.queryParamInput.Reset()
	m.paramEdit.bodyInput.Reset()
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
	req, ok := listItem.(store.RequestEntity)
	if !ok {
		return
	}
	str := req.Url
	suffix := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf("  [%s] [%s]", req.Environment, req.Method))

	if index == m.Index() {
		str = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Render(">" + str)
	} else {
		str = lipgloss.NewStyle().PaddingLeft(1).Render(str)
	}

	fmt.Fprint(w, str+suffix)
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

/*
TODO:
  - GET
    1. Query string parameters — ?key=value&key2=value2 after the URL path
    2. Support dynamic path parameter in
    3. When type in url with query string params, should also relect on the params
  - POST
    1. request body support
    2. also has query string parameters
*/
func (m *model) doRequest(req store.RequestEntity) (string, error) {
	// TODO: later should support other method
	// TODO: later may set support setting the header
	var res *http.Response
	var err error
	switch req.Method {
	case "post":
		res, err = http.Post(req.Url, "application/json", strings.NewReader(req.Body))
	case "get":
		// TODO: combine the query param from req
		res, err = http.Get(req.Url)
	default:
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	var indentedJSONContent bytes.Buffer
	if err := json.Indent(&indentedJSONContent, body, "", "  "); err != nil {
		return "", err
	}
	return indentedJSONContent.String(), nil
}

func textareaContentAsJSON(ta *textarea.Model) string {
	content := ta.Value()
	if len(content) == 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || len(lines)%2 != 0 {
		log.Printf("[textareaContentAsJSON] expect line count is even, but got %v\n", len(lines))
		return ""
	}
	kv := make(map[string]any)
	for i := 0; i < len(lines); i += 2 {
		k, v := lines[i], lines[i+1]
		kv[k] = v
	}
	data, err := json.Marshal(kv)
	if err != nil {
		log.Printf("[textareaContentAsJSON]: %v\n", err)
		return ""
	}
	return string(data)
}

func main() {
	// TODO: add promt UI for error notification instead of streaming to file
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
