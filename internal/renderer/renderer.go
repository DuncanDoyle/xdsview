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

func style(c render.Class, _ *render.FilterRef, s string) string { return styles[c].Render(s) }

// Render returns the snapshot as krp's coloured text tree.
func Render(snapshot *model.EnvoySnapshot) string {
	return text(snapshot, render.Options{Style: style})
}

func text(snapshot *model.EnvoySnapshot, opts render.Options) string {
	var b strings.Builder
	_ = render.Text(&b, snapshot, opts) // a strings.Builder doesn't fail
	return strings.TrimSuffix(b.String(), "\n")
}
