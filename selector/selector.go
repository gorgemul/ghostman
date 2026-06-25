package selector

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
)

type Model struct {
	title   string
	choices []string
	index   int
}

func New(title string, choices []string) Model {
	var t string
	if len(title) > 0 {
		t = fmt.Sprintf("%s: ", title)
	}
	return Model{
		title:   t,
		choices: slices.Clone(choices),
		index:   0,
	}
}

func (m *Model) Next() {
	if m.index < len(m.choices)-1 {
		m.index++
	}
}

func (m *Model) WrappingNext() {
	m.index++
	if m.index == len(m.choices) {
		m.index = 0
	}
}

func (m *Model) Prev() {
	if m.index > 0 {
		m.index--
	}
}

func (m *Model) WrappingPrev() {
	m.index--
	if m.index < 0 {
		m.index = len(m.choices) - 1
	}
}

func (m *Model) Reset() {
	m.index = 0
}

func (m *Model) SetValue(value string) {
	for i, choice := range m.choices {
		if choice == value {
			m.index = i
			break
		}
	}
}

func (m Model) Value() string {
	return m.choices[m.index]
}

func (m Model) View(inCurrentRow bool) string {
	sb := strings.Builder{}
	sb.WriteString(m.title)
	for i, choice := range m.choices {
		selected := "[ ]"
		separator := " "
		if m.index == i {
			selected = "[x]"
		}
		if i == len(m.choices)-1 {
			separator = ""
		}
		fmt.Fprintf(&sb, "%s%s%s", selected, choice, separator)
	}
	if inCurrentRow {
		return lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170")).Render("> "+sb.String()) + "\n"
	} else {
		return lipgloss.NewStyle().PaddingLeft(4).Render(sb.String()) + "\n"
	}
}
