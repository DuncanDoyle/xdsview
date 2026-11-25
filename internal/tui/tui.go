// Package tui provides the interactive bubbletea TUI for krp.
// It wraps a [envoymodel.Snapshot] in a scrollable, navigable terminal
// UI where the user can expand HTTP filter typed configs inline.
package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/DuncanDoyle/xdsview/internal/renderer"
	envoymodel "github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultWidth  = 80
	defaultHeight = 23 // 24 rows minus 1 for the help bar
)

var helpBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

const helpText = "↑/↓ navigate • Enter/Space expand • a toggle all • q quit"

// model holds all mutable state for the interactive TUI session.
type model struct {
	view     renderer.View
	items    []render.Row
	cursor   int
	expanded map[render.Row]bool
	viewport viewport.Model
}

// buildItems returns the flat ordered list of all navigable filter rows in the
// snapshot. It follows the canonical traversal order so that items[N] always
// corresponds to the N-th filter rendered on screen by [renderer.RenderInteractive].
//
// Filter chains with a nil HCM or a nil RouteConfig are skipped — they produce
// no navigable filters (same as the renderer, which shows "[no HCM]" / "[RDS not found]").
func buildItems(snapshot *envoymodel.Snapshot) []render.Row {
	var items []render.Row
	for _, listener := range snapshot.Listeners {
		for _, fc := range listener.FilterChains {
			if fc.HCM == nil || fc.HCM.RouteConfig == nil {
				continue
			}
			for _, vh := range fc.HCM.RouteConfig.VirtualHosts {
				for _, r := range vh.Routes {
					for _, f := range fc.HCM.HTTPFilters {
						items = append(items, render.Row{Subject: f.ID, Route: r.ID, Slot: f.ID})
					}
				}
			}
		}
	}
	return items
}

// findCursorLine finds the line number of the cursor item in the rendered
// content by locating the ANSI reverse-video code emitted by cursorStyle.
// Returns 0 if no cursor item is present (no cursor set or ANSI disabled).
func findCursorLine(content string) int {
	const reverseCode = "\x1b[7m"
	idx := strings.Index(content, reverseCode)
	if idx < 0 {
		return 0
	}
	return strings.Count(content[:idx], "\n")
}

// renderOpts builds the RenderOpts for the current model state.
func (m model) renderOpts() renderer.RenderOpts {
	if len(m.items) == 0 {
		return renderer.RenderOpts{}
	}
	cursor := m.items[m.cursor]
	return renderer.RenderOpts{
		Cursor:   &cursor,
		Expanded: m.expanded,
	}
}

// scrollToCursor adjusts the viewport offset to keep cursorLine visible
// without moving the viewport when the cursor is already in view.
// If the cursor is above the top of the viewport it scrolls up; if it is
// below the bottom it scrolls down; otherwise the offset is left unchanged.
// This preserves access to content rendered above the first navigable item
// (e.g. Listener and FilterChain headers) that would otherwise be hidden if
// the viewport were unconditionally pinned to the cursor line.
func (m *model) scrollToCursor(cursorLine int) {
	if cursorLine < m.viewport.YOffset {
		m.viewport.SetYOffset(cursorLine)
	} else if cursorLine >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(cursorLine - m.viewport.Height + 1)
	}
}

// setContent re-renders the snapshot with the current interactive state and
// sets the viewport content. Called on every state change.
func (m *model) setContent() {
	content := renderer.RenderInteractive(m.view, m.renderOpts())
	m.viewport.SetContent(content)
	m.scrollToCursor(findCursorLine(content))
}

// initialModel constructs the initial TUI model for the given view.
// It calls buildItems once to populate the flat navigation list and initialises
// the viewport with default dimensions; the actual terminal size is applied when
// the first tea.WindowSizeMsg is received in Update.
func initialModel(v renderer.View) model {
	vp := viewport.New(defaultWidth, defaultHeight)
	m := model{
		view:     v,
		items:    buildItems(v.Snapshot),
		cursor:   0,
		expanded: make(map[render.Row]bool),
		viewport: vp,
	}
	return m
}

// Init implements tea.Model. No initial commands are needed.
func (m model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 1 // reserve one line for the help bar
		m.setContent()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.setContent()
			} else if m.viewport.YOffset > 0 {
				// Cursor is already at the first item; continue scrolling the
				// viewport up line by line so that Listener/FilterChain/HCM
				// headers above the first filter are revealed smoothly.
				m.viewport.SetYOffset(m.viewport.YOffset - 1)
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.setContent()
			}
			return m, nil

		case "enter", " ":
			if len(m.items) > 0 {
				ref := m.items[m.cursor]
				if m.expanded[ref] {
					delete(m.expanded, ref)
				} else {
					m.expanded[ref] = true
				}
				m.setContent()
			}
			return m, nil

		case "a":
			if len(m.items) == 0 {
				return m, nil
			}
			if len(m.expanded) == len(m.items) {
				// All expanded — collapse all.
				m.expanded = make(map[render.Row]bool)
			} else {
				// Some or none expanded — expand all.
				for _, ref := range m.items {
					m.expanded[ref] = true
				}
			}
			m.setContent()
			return m, nil
		}
	}

	// Forward remaining messages (e.g. mouse events) to the viewport.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m model) View() string {
	return m.viewport.View() + "\n" + helpBarStyle.Render(helpText)
}

// Run starts the interactive TUI for the given view. With no filter to
// select, for example when only rejected listeners remain, it prints the
// static tree to out instead, so their state and errors still show. It blocks
// until the user quits (q or Ctrl+C) and returns any bubbletea program error.
func Run(v renderer.View, out, errOut io.Writer) error {
	m := initialModel(v)
	if len(m.items) == 0 {
		fmt.Fprintln(errOut, "no expandable filters found; printing the tree")
		fmt.Fprintln(out, renderer.Render(v))
		return nil
	}
	// Content is initialised in Update() when the first tea.WindowSizeMsg arrives.
	// This is the standard bubbletea pattern — do not call setContent() here.
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
