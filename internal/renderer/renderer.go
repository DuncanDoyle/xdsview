// Package renderer styles krp's text output: it prints the tree from
// [render.Text] with lipgloss colours, and adds the TUI's cursor and expanded
// filter config through render's hooks.
package renderer

import (
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
	"github.com/charmbracelet/lipgloss"
)

// styles maps render's classes to colours.
var styles = map[render.Class]lipgloss.Style{
	render.ClassListener:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),  // bright blue
	render.ClassVirtualHost: lipgloss.NewStyle().Foreground(lipgloss.Color("15")),             // white
	render.ClassRoute:       lipgloss.NewStyle().Foreground(lipgloss.Color("15")),             // white
	render.ClassFilter:      lipgloss.NewStyle().Foreground(lipgloss.Color("13")),             // magenta
	render.ClassFilterOff:   lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true), // gray
	render.ClassOrigin:      lipgloss.NewStyle().Foreground(lipgloss.Color("8")),              // gray
	render.ClassBackend:     lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),  // green
	render.ClassWarning:     lipgloss.NewStyle().Foreground(lipgloss.Color("9")),              // red
}

func style(c render.Class, _ render.Row, s string) string { return styles[c].Render(s) }

// View is what krp prints: a snapshot, its chains from chain.Effective, and
// the labels and trailer lines krp puts on rows (nil for none).
type View struct {
	Snapshot *model.Snapshot
	Chains   map[model.ID]model.EffectiveChain
	Labels   func(render.Row) []string
	Trailer  func(render.Row) []render.Line
}

// Render returns the view as krp's coloured text tree.
func Render(v View) string {
	return text(v, render.Options{Style: style})
}

func text(v View, opts render.Options) string {
	opts.Labels, opts.Trailer = v.Labels, v.Trailer
	var b strings.Builder
	_ = render.Text(&b, v.Snapshot, v.Chains, opts) // a strings.Builder doesn't fail
	return strings.TrimSuffix(b.String(), "\n")
}
