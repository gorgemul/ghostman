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

type (
	appMode  int
	editMode int
)

const (
	APP_MODE_DASHBOARD appMode = iota
	// TODO: should provide help message in APP_MODE_EDIT mode
	APP_MODE_EDIT
	APP_MODE_RESULT
)

const (
	EDIT_MODE_ALL = iota
	EDIT_MODE_QUERY_PARAMS
	EDIT_MODE_BODY
)

type kvModel struct {
	pairs textarea.Model
}

type dashboardModel struct {
	list              list.Model
	methodConfig      selector.Model
	environmentConfig selector.Model
}

type editModel struct {
	mode        editMode
	index       int
	id          *int64 // to differenciate insert and update in edit mode
	method      selector.Model
	environment selector.Model
	url         textinput.Model
	queryParams kvModel
	body        kvModel
	confirm     selector.Model
}

type resultModel struct {
	view viewport.Model
}

// TODO: paramEdit mode should put inside edit mode
type model struct {
	store     *store.Store
	mode      appMode
	dashboard dashboardModel // Mode == APP_MODE_DASHBOARD
	edit      editModel      // Mode == APP_MODE_EDIT
	result    resultModel    // Mode == APP_MODE_RESULT
}

type itemDelegate struct{}

func initialModel(store *store.Store) model {
	config := store.FindConfig()
	methodConfig := selector.New("", []string{"all", "get", "post"})
	environmentConfig := selector.New("", []string{"all", "local", "test", "staging", "production"})
	methodConfig.SetValue(config.Method)
	environmentConfig.SetValue(config.Environment)
	return model{
		store: store,
		mode:  APP_MODE_DASHBOARD,
		dashboard: dashboardModel{
			list: newListWithStyles(store),
			// only used for state management, not for display
			methodConfig:      methodConfig,
			environmentConfig: environmentConfig,
		},
		edit: editModel{
			index:       0,
			method:      selector.New("Method", []string{"get", "post"}),
			environment: selector.New("Environment", []string{"local", "test", "staging", "production"}),
			url:         newTextinputWithStyles(),
			queryParams: kvModel{newTextareaWithStyles()},
			body:        kvModel{newTextareaWithStyles()},
			confirm:     selector.New("", []string{"save", "cancel"}),
		},
		result: resultModel{view: viewport.New()},
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.dashboard.list.SetSize(msg.Width, msg.Height)
		m.result.view.SetWidth(msg.Width)
		m.result.view.SetHeight(msg.Height)
		m.edit.queryParams.pairs.SetWidth(msg.Width)
		m.edit.queryParams.pairs.SetHeight(msg.Height)
		m.edit.body.pairs.SetWidth(msg.Width)
		m.edit.body.pairs.SetHeight(msg.Height)
	}
	switch m.mode {
	case APP_MODE_DASHBOARD:
		return handleAppModeDashboard(&m, msg)
	case APP_MODE_EDIT:
		return handleAppModeEdit(&m, msg)
	case APP_MODE_RESULT:
		return handleAppModeResult(&m, msg)
	}
	return m, nil
}

func (m model) View() tea.View {
	switch m.mode {
	case APP_MODE_DASHBOARD:
		return tea.NewView("\n" + m.dashboard.list.View())
	case APP_MODE_EDIT:
		switch m.edit.mode {
		case EDIT_MODE_ALL:
			var sb strings.Builder
			sb.WriteString(m.edit.method.View(m.edit.index == MethodIndex))
			sb.WriteString(m.edit.environment.View(m.edit.index == EnvironmentIndex))
			// TODO: in get url and has query parameters should reflect that in the url?
			sb.WriteString(m.edit.selectionView("Url: ", m.edit.url.View(), UrlIndex))
			queryParamsTitle := "QueryParams: "
			sb.WriteString(m.edit.selectionView(queryParamsTitle, m.edit.queryParams.view(len(queryParamsTitle)), QueryParamsIndex))
			bodyTitle := "Body: "
			sb.WriteString(m.edit.selectionView(bodyTitle, m.edit.body.view(len(bodyTitle)), BodyIndex))
			sb.WriteString(lipgloss.NewStyle().Render(m.edit.confirm.View(m.edit.index == ConfirmIndex)))
			return tea.NewView(sb.String())
		case EDIT_MODE_QUERY_PARAMS:
			return tea.NewView(m.edit.queryParams.pairs.View())
		case EDIT_MODE_BODY:
			return tea.NewView(m.edit.body.pairs.View())
		}
	case APP_MODE_RESULT:
		v := tea.NewView(m.result.view.View())
		v.AltScreen = true
		return v
	}
	return tea.NewView("[ERROR] should never reach here\n")
}

func (m *model) resetEditModel() {
	m.edit.mode = EDIT_MODE_ALL
	m.edit.id = nil
	m.edit.index = 0
	m.edit.method.Reset()
	m.edit.environment.Reset()
	m.edit.url.Reset()
	m.edit.confirm.Reset()
	m.edit.queryParams.pairs.Reset()
	m.edit.body.pairs.Reset()
}

// TODO: when in filter mode should disable all keybind but enter or edit
func (m *model) resetDashboardModel() {
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

func syncListWithDB(l *list.Model, store *store.Store) error {
	reqs, err := store.FindRequests()
	if err != nil {
		return fmt.Errorf("syncListWithDB: %v", err)
	}
	items := []list.Item{}
	for _, req := range reqs {
		items = append(items, req)
	}
	l.SetItems(items)
	return nil
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

// nede store to sync data with db and fetch db config
func newListWithStyles(store *store.Store) list.Model {
	config := store.FindConfig()
	l := list.New(
		[]list.Item{},
		itemDelegate{},
		60,
		20,
	)
	l.Title = fmt.Sprintf("env: %s, method: %s", config.Environment, config.Method)
	l.SetShowStatusBar(false)
	helpKeys := []key.Binding{
		key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "environment")),
		key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "method")),
	}
	l.AdditionalShortHelpKeys = func() []key.Binding { return helpKeys }
	l.AdditionalFullHelpKeys = func() []key.Binding { return helpKeys }
	l.SetDelegate(itemDelegate{})
	if err := syncListWithDB(&l, store); err != nil {
		log.Printf("newListWithStyles: %v\n", err)
	}
	return l
}

func newTextareaWithStyles() textarea.Model {
	ta := textarea.New()
	ta.SetHeight(20)
	taStyles := ta.Styles()
	taStyles.Cursor.Blink = false
	ta.SetStyles(taStyles)
	return ta
}

func newTextinputWithStyles() textinput.Model {
	ti := textinput.New()
	ti.CharLimit = 128
	ti.SetWidth(128)
	ti.Prompt = ""
	ti.Placeholder = "NULL"
	return ti
}

func handleAppModeDashboard(m *model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dashboard.list.SettingFilter() {
		var cmd tea.Cmd
		m.dashboard.list, cmd = m.dashboard.list.Update(msg)
		return *m, cmd
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return *m, tea.Quit
		case "e":
			req, ok := m.dashboard.list.SelectedItem().(store.RequestEntity)
			if !ok {
				return *m, nil
			}
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
			m.edit.queryParams.pairs.SetValue(queryParamsTrim)
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
			m.edit.body.pairs.SetValue(bodyTrim)
			m.mode = APP_MODE_EDIT
			m.resetDashboardModel()
		case "n":
			m.mode = APP_MODE_EDIT
			m.resetDashboardModel()
		case "E":
			// TODO: maybe abstract away the findConfig and set list title, also the code duplication
			m.dashboard.environmentConfig.WrappingNext()
			if err := m.store.UpdateEnvironmentConfig(m.dashboard.environmentConfig.Value()); err != nil {
				m.dashboard.environmentConfig.WrappingPrev() // restore its old state
				return *m, nil
			}
			config := m.store.FindConfig()
			m.dashboard.list.Title = fmt.Sprintf("env: %s, method: %s", config.Environment, config.Method)
			if err := syncListWithDB(&m.dashboard.list, m.store); err != nil {
				log.Printf("Update: %v\n", err)
			}
			return *m, nil
		case "M":
			m.dashboard.methodConfig.WrappingNext()
			if err := m.store.UpdateMethodConfig(m.dashboard.methodConfig.Value()); err != nil {
				m.dashboard.methodConfig.WrappingPrev() // restore its old state
				return m, nil
			}
			config := m.store.FindConfig()
			m.dashboard.list.Title = fmt.Sprintf("env: %s, method: %s", config.Environment, config.Method)
			if err := syncListWithDB(&m.dashboard.list, m.store); err != nil {
				log.Printf("Update: %v\n", err)
			}
			return m, nil
		case "enter":
			// TODO: should add a buffering status in the view mode, since right now could triger multiple requests by hitting enter
			req, ok := m.dashboard.list.SelectedItem().(store.RequestEntity)
			if !ok {
				return *m, nil
			}
			content, err := m.doRequest(req)
			if err != nil {
				log.Printf("doRequest: %v", err)
				return m, nil
			}
			width := m.result.view.Width()
			warpppedContent := lipgloss.NewStyle().Width(width).Render(content)
			m.result.view.SetContent(warpppedContent)
			m.mode = APP_MODE_RESULT
		default:
			var cmd tea.Cmd
			m.dashboard.list, cmd = m.dashboard.list.Update(msg)
			return *m, cmd
		}
	}
	return *m, nil
}

func handleAppModeEdit(m *model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.edit.mode {
	case EDIT_MODE_ALL:
		return handleEditModeAll(m, msg)
	case EDIT_MODE_QUERY_PARAMS, EDIT_MODE_BODY:
		return handleEditModeQueryParamsAndBody(m, msg)
	}
	return *m, nil
}

func handleAppModeResult(m *model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.mode = APP_MODE_DASHBOARD
			return *m, nil
		}
	}
	var cmd tea.Cmd
	m.result.view, cmd = m.result.view.Update(msg)
	return *m, cmd
}

func handleEditModeAll(m *model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if m.edit.url.Focused() {
			var cmd tea.Cmd
			m.edit.url, cmd = m.edit.url.Update(msg)
			return *m, cmd
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
				m.edit.queryParams.pairs.Focus()
				m.edit.mode = EDIT_MODE_QUERY_PARAMS
				// TODO: set value from db
			case BodyIndex:
				m.edit.body.pairs.Focus()
				m.edit.mode = EDIT_MODE_BODY
			case ConfirmIndex:
				if m.edit.confirm.Value() == "save" && len(m.edit.url.Value()) > 0 {
					if _, err := m.store.UpsertRequest(store.UpsertRequestParams{
						Id:          m.edit.id,
						Url:         m.edit.url.Value(),
						Method:      m.edit.method.Value(),
						Environment: m.edit.environment.Value(),
						QueryParams: m.edit.queryParams.value(),
						Body:        m.edit.body.value(),
					}); err != nil {
						log.Println("Update: ", err)
					} else {
						if err := syncListWithDB(&m.dashboard.list, m.store); err != nil {
							log.Println("Update: ", err)
						}
					}
				}
				m.resetEditModel()
				m.mode = APP_MODE_DASHBOARD
			}
			return *m, nil
		case "esc", "ctrl+c":
			if m.edit.index == UrlIndex && m.edit.url.Focused() {
				m.edit.url.Blur()
				return *m, nil
			}
			m.resetEditModel()
			m.mode = APP_MODE_DASHBOARD
			return *m, nil
		}
	}
	var cmd tea.Cmd
	m.edit.url, cmd = m.edit.url.Update(msg)
	return *m, cmd
}

func handleEditModeQueryParamsAndBody(m *model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.edit.mode = EDIT_MODE_ALL
			var input *textarea.Model
			switch m.edit.index {
			case QueryParamsIndex:
				input = &m.edit.queryParams.pairs
			case BodyIndex:
				input = &m.edit.body.pairs
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
			return *m, nil
		}
	}
	var cmd tea.Cmd
	switch m.edit.index {
	case QueryParamsIndex:
		m.edit.queryParams, cmd = m.edit.queryParams.update(msg)
	case BodyIndex:
		m.edit.body, cmd = m.edit.body.update(msg)
	}
	return *m, cmd
}

func (m kvModel) update(msg tea.Msg) (kvModel, tea.Cmd) {
	var cmd tea.Cmd
	m.pairs, cmd = m.pairs.Update(msg)
	return m, cmd
}

// TODO: second kv can't padding right
func (m kvModel) view(keyLenPadding int) string {
	lines := strings.Split(m.pairs.Value(), "\n")
	kv := make(map[string]any)
	// since empty strings.Split("", "\n") -> [""]
	if len(lines) < 2 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("NULL")
	}
	var content strings.Builder
	for i := 0; i < len(lines); i += 2 {
		prefix, suffix := "", ""
		if i > 0 {
			prefix = strings.Repeat(" ", keyLenPadding)
		}
		if i+1 != len(lines)-1 {
			suffix = "\n"
		}
		k, v := lines[i], lines[i+1]
		kv[k] = v
		fmt.Fprintf(&content, "%s\"%s\": \"%s\"%s", prefix, k, v, suffix)
	}
	return content.String()
}

func (m kvModel) value() string {
	content := m.pairs.Value()
	if len(content) == 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || len(lines)%2 != 0 {
		log.Printf("[kvModel.value] expect line count is even, but got %v\n", len(lines))
		return ""
	}
	kv := make(map[string]any)
	for i := 0; i < len(lines); i += 2 {
		k, v := lines[i], lines[i+1]
		kv[k] = v
	}
	data, err := json.Marshal(kv)
	if err != nil {
		log.Printf("[kvModel.value]: %v\n", err)
		return ""
	}
	return string(data)
}

func (m editModel) selectionView(title, content string, matchIndex int) string {
	style := lipgloss.NewStyle()
	paddingLeft := 1
	if m.index == matchIndex {
		title = ">" + title
		paddingLeft = 0
		style = style.Foreground(lipgloss.Color("170"))
	}
	return style.PaddingLeft(paddingLeft).Render(title+content) + "\n"
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
