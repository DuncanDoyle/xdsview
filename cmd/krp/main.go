// Command krp (Kgateway Route Printer) visualizes the Envoy route configuration
// for a Kubernetes Gateway managed by kgateway.
//
// Usage:
//
//	krp dump --file <path>                                                                     # parse a local config_dump JSON file
//	krp dump --gateway <name> -n <ns>                                                          # live fetch via port-forward to gateway-proxy pod
//	krp dump --deployment <name> -n <ns>                                                       # live fetch via port-forward to any pod in Deployment
//	krp dump --gateway <name> -n <ns> --context ctx                                            # same, with an explicit kubeconfig context
//	krp dump --file <path> --route <name> --route-ns <ns>                                       # filter by HTTPRoute
//	krp dump --file <path> --route <name> --route-ns <ns> --rule 0                             # filter by rule index
//	krp dump --file <path> --route <name> --route-ns <ns> --rule api                           # filter by rule name
//	krp dump --file <path> --route <name> --route-ns <ns> --rule 0 --interactive            # interactive TUI mode with filter config expansion
//	krp dump --file <path> -o json                                                             # machine-readable output (see output.go)
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DuncanDoyle/xdsview/internal/renderer"
	"github.com/DuncanDoyle/xdsview/internal/tui"
	"github.com/DuncanDoyle/xdsview/pkg/adapter/kgateway"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/chain"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/parser"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/provenance"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/render"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/source"
	"github.com/DuncanDoyle/xdsview/pkg/envoy/source/kube"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// newRootCmd builds the krp command tree. Output goes to the command's Out and
// Err writers, which default to stdout and stderr.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "krp",
		Short: "Kgateway route printer — visualize Envoy config",
	}

	dump := &cobra.Command{
		Use:   "dump",
		Short: "Dump and visualize the Envoy route configuration",
		RunE:  runDump,
	}

	// Config source — exactly one must be provided.
	dump.Flags().String("file", "", "Path to an Envoy config_dump JSON file")
	dump.Flags().String("gateway", "", "Gateway name (port-forward to gateway-proxy pod via gateway label)")
	dump.Flags().String("deployment", "", "Deployment name (port-forward to any ready pod in the Deployment)")
	dump.Flags().StringP("namespace", "n", "default", "Namespace (used with --gateway or --deployment)")
	dump.Flags().String("context", "", "Kubeconfig context (used with --gateway or --deployment, default: current context)")

	// HTTPRoute filter — narrows the output to routes belonging to one HTTPRoute.
	// --route-ns is intentionally separate from -n/--namespace because the
	// HTTPRoute namespace can differ from the Gateway namespace.
	dump.Flags().String("route", "", "Filter output to routes belonging to this HTTPRoute name")
	dump.Flags().String("route-ns", "", "Namespace of the HTTPRoute (required with --route; may differ from the Gateway namespace)")
	dump.Flags().String("rule", "", "Rule within the HTTPRoute: a zero-based index or a rule name (default all rules, used with --route)")

	// Interactive mode — launches a bubbletea TUI instead of printing static output.
	// Composes with --route/--route-ns/--rule: the filter is applied before the TUI launches.
	dump.Flags().BoolP("interactive", "i", false, "Launch the interactive TUI instead of printing static output")

	dump.Flags().StringP("output", "o", outputText, "Output format: text, json")

	root.AddCommand(dump)
	return root
}

// extraModules are composed after the kgateway module. Tests use it to plug in a module.
var extraModules []provenance.Module

// listTimeout bounds the optional HTTPRoute listing, so a slow API server can't
// hold back a dump that's already fetched.
var listTimeout = 10 * time.Second

// cmdContext is the command's context, or Background when it has none.
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// The kube entry points, replaced in tests to run the live path without a cluster.
var (
	kubeGateway    = kube.Gateway
	kubeDeployment = kube.Deployment
	kubeHTTPRoutes = kube.HTTPRoutes
)

const gatewayGroup = "gateway.networking.k8s.io"

// request is what the CLI knows beyond the dump.
type request struct {
	query   *kgateway.Query // --route and --rule; nil shows every route
	live    bool            // the dump was fetched from a cluster
	known   []model.Ref     // HTTPRoutes listed from the cluster in live mode
	gateway *model.Ref      // the Gateway behind --gateway, krp's own context
}

// result is krp's view of one dump: the snapshot it prints, pruned under
// --route, with the chains, index and selection computed on the whole dump.
type result struct {
	full       *model.Snapshot
	snap       *model.Snapshot
	chains     map[model.ID]model.EffectiveChain
	ix         *provenance.Index
	narrowings []provenance.Narrowing // printed inline, marked as chosen from known HTTPRoutes
	selection  *selection             // nil unless --route is set or the dump is live
	gateway    *model.Ref
}

// build runs krp's flow: chain.Effective, Compose, then Select under --route
// or Narrow in live mode, then Prune (design §7, D-022).
func build(parsed *model.Snapshot, req request) (result, error) {
	ix, err := provenance.Compose(parsed, append([]provenance.Module{kgateway.New()}, extraModules...)...)
	if err != nil {
		return result{}, err
	}
	res := result{full: parsed, snap: parsed, chains: chain.Effective(parsed), ix: ix, gateway: req.gateway}
	switch {
	case req.query != nil:
		sel := kgateway.Select(ix, parsed, *req.query)
		res.selection = newSelection(parsed, *req.query, sel)
		keep := map[model.ID]bool{}
		for _, id := range res.selection.Routes {
			keep[id] = true
		}
		res.snap = model.Prune(parsed, func(id model.ID) bool { return keep[id] })
		res.narrowings = sel.Narrowed
	case req.live:
		res.narrowings = kgateway.Narrow(ix, req.known)
		res.selection = liveSelection(parsed, res.narrowings)
	}
	return res, nil
}

// labels is the render Labels hook: the provenance labels, plus the --gateway
// Gateway on listener rows, labelled as krp's context rather than a claim (D-022).
func (r result) labels() func(render.Row) []string {
	base := provenance.Labels(r.ix, r.narrowings)
	return func(row render.Row) []string {
		out := base(row)
		if r.gateway == nil || row.Route != "" || row.Slot != "" {
			return out
		}
		if l, ok := r.snap.Listener(row.Subject); ok && !l.Static {
			out = append(out, r.gateway.String()+" (from --gateway)")
		}
		return out
	}
}

// view is the renderer's input for r.
func (r result) view() renderer.View {
	return renderer.View{Snapshot: r.snap, Chains: r.chains, Labels: r.labels(), Trailer: provenance.Notes(r.ix)}
}

// hasRoutes reports whether any filter chain in snap kept a virtual host.
func hasRoutes(snap *model.Snapshot) bool {
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			if fc.HCM != nil && fc.HCM.RouteConfig != nil && len(fc.HCM.RouteConfig.VirtualHosts) > 0 {
				return true
			}
		}
	}
	return false
}

// parseRule reads --rule: empty or -1 means all rules, a non-negative integer
// is a rule index, and anything else is a rule name.
func parseRule(rule string) (index *int, name string, err error) {
	if rule == "" {
		return nil, "", nil
	}
	if n, err := strconv.Atoi(rule); err == nil {
		if n < -1 {
			return nil, "", fmt.Errorf("--rule must be a rule index >= 0 or a rule name")
		}
		if n == -1 {
			return nil, "", nil
		}
		return &n, "", nil
	}
	return nil, rule, nil
}

// runDump is the cobra RunE handler for the "dump" subcommand.
// It reads the config dump (from a file or live via port-forward), parses it,
// applies any HTTPRoute filter, prints warnings to stderr, then renders the
// snapshot to stdout.
func runDump(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	gateway, _ := cmd.Flags().GetString("gateway")
	deployment, _ := cmd.Flags().GetString("deployment")
	namespace, _ := cmd.Flags().GetString("namespace")
	kubeContext, _ := cmd.Flags().GetString("context")

	routeName, _ := cmd.Flags().GetString("route")
	routeNS, _ := cmd.Flags().GetString("route-ns")
	rule, _ := cmd.Flags().GetString("rule")
	interactive, _ := cmd.Flags().GetBool("interactive")
	output, _ := cmd.Flags().GetString("output")

	// Exactly one source must be specified; --gateway and --deployment are mutually exclusive.
	if file == "" && gateway == "" && deployment == "" {
		return fmt.Errorf("specify one of --file <path>, --gateway <name>, or --deployment <name>")
	}
	if file != "" && (gateway != "" || deployment != "") {
		return fmt.Errorf("--file cannot be combined with --gateway or --deployment")
	}
	if gateway != "" && deployment != "" {
		return fmt.Errorf("--gateway and --deployment are mutually exclusive")
	}

	if output != outputText && output != outputJSON {
		return fmt.Errorf("unknown output format %q: use one of text, json", output)
	}
	if output == outputJSON && interactive {
		return fmt.Errorf("-o json cannot be combined with --interactive")
	}

	// Validate HTTPRoute filter flags.
	// --route and --route-ns must always be used together because two HTTPRoutes
	// with the same name in different namespaces are distinct resources.
	if routeName != "" && routeNS == "" {
		return fmt.Errorf("--route-ns is required when --route is set")
	}
	if routeNS != "" && routeName == "" {
		return fmt.Errorf("--route-ns requires --route")
	}
	ruleIndex, ruleName, err := parseRule(rule)
	if err != nil {
		return err
	}
	if rule != "" && routeName == "" {
		return fmt.Errorf("--rule requires --route")
	}

	// Pick the config dump source; live sources port-forward to the admin API.
	var src source.Source
	switch {
	case file != "":
		src = source.File(file)
	case gateway != "":
		src = kubeGateway(gateway, namespace, kube.Options{KubeContext: kubeContext})
	default:
		src = kubeDeployment(deployment, namespace, kube.Options{KubeContext: kubeContext})
	}
	if file == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "Connecting to Envoy admin API...")
	}

	rc, err := src.ConfigDump(context.Background())
	if err != nil {
		return err
	}
	defer rc.Close()

	// Parse the config dump into a Snapshot.
	parsed, err := parser.Parse(rc, parser.Options{Capture: src.Capture()})
	if err != nil {
		return err
	}

	// Print any non-fatal parse warnings to stderr so users know if sections were skipped.
	for _, w := range parsed.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
	}

	req := request{live: file == ""}
	if gateway != "" {
		req.gateway = &model.Ref{Group: gatewayGroup, Kind: "Gateway", Namespace: namespace, Name: gateway}
	}
	if routeName != "" {
		// --route names the HTTPRoute, so it is also the known HTTPRoute (D-022).
		ref := model.Ref{Group: gatewayGroup, Kind: "HTTPRoute", Namespace: routeNS, Name: routeName}
		req.query = &kgateway.Query{Route: ref, RuleIndex: ruleIndex, RuleName: ruleName, Known: []model.Ref{ref}}
	}

	// In live mode, the cluster's HTTPRoutes narrow route names with several
	// readings (D-006). --route names the HTTPRoute itself, so it skips the
	// listing; a failed or slow listing only costs route origins.
	if req.live && req.query == nil {
		ctx, cancel := context.WithTimeout(cmdContext(cmd), listTimeout)
		req.known, err = kubeHTTPRoutes(ctx, kube.Options{KubeContext: kubeContext})
		cancel()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: route origins unavailable: %v\n", err)
		}
	}

	res, err := build(parsed, req)
	if err != nil {
		return err
	}
	for _, w := range res.ix.Warnings() {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w.Message)
	}
	// Listeners that aren't active survive pruning, so count routes.
	if req.query != nil && !hasRoutes(res.snap) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: no routes found for HTTPRoute %s/%s — check name, namespace, and that the HTTPRoute is attached to this Gateway\n",
			routeNS, routeName)
	}

	// Render and print — or launch the interactive TUI.
	switch {
	case interactive:
		return tui.Run(res.view(), cmd.OutOrStdout(), cmd.ErrOrStderr())
	case output == outputJSON:
		return writeJSON(cmd.OutOrStdout(), res)
	}
	fmt.Fprintln(cmd.OutOrStdout(), renderer.Render(res.view()))
	return nil
}
