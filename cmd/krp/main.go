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

	"github.com/DuncanDoyle/krp/internal/renderer"
	"github.com/DuncanDoyle/krp/internal/tui"
	"github.com/DuncanDoyle/krp/pkg/envoy/filter"
	"github.com/DuncanDoyle/krp/pkg/envoy/model"
	"github.com/DuncanDoyle/krp/pkg/envoy/origin"
	"github.com/DuncanDoyle/krp/pkg/envoy/parser"
	"github.com/DuncanDoyle/krp/pkg/envoy/source"
	"github.com/DuncanDoyle/krp/pkg/envoy/source/kube"
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

// extraMappers run before the kgateway mapper. Tests use it to plug in a mapper.
var extraMappers []origin.Mapper

// The kube entry points, replaced in tests to run the live path without a cluster.
var (
	kubeGateway    = kube.Gateway
	kubeDeployment = kube.Deployment
	kubeHTTPRoutes = kube.HTTPRoutes
)

// mapper builds the origin mapper from what the CLI knows: the Gateway behind
// --gateway, and the HTTPRoutes listed from the cluster or named by --route,
// which resolve route names.
func mapper(gateway, namespace, routeName, routeNS string, known []model.Ref) origin.Mapper {
	opts := origin.KgatewayOptions{Known: known}
	if gateway != "" {
		opts.Gateway = &model.Ref{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: namespace, Name: gateway}
	}
	if routeName != "" {
		opts.Known = append(opts.Known, model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: routeNS, Name: routeName})
	}
	return origin.Chain(append(append([]origin.Mapper(nil), extraMappers...), origin.Kgateway(opts))...)
}

// parseRule reads --rule: empty or -1 means all rules, a non-negative integer
// is a rule index, and anything else is a rule name.
func parseRule(rule string) (index int, name string, err error) {
	if rule == "" {
		return filter.AllRules, "", nil
	}
	if n, err := strconv.Atoi(rule); err == nil {
		if n < filter.AllRules {
			return 0, "", fmt.Errorf("--rule must be a rule index >= 0 or a rule name")
		}
		return n, "", nil
	}
	return filter.AllRules, rule, nil
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

	// Parse the config dump into an EnvoySnapshot.
	result, err := parser.Parse(rc)
	if err != nil {
		return err
	}

	// Print any non-fatal parse warnings to stderr so users know if sections were skipped.
	for _, w := range result.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}

	// Apply the HTTPRoute filter when requested.
	snapshot := result.Snapshot
	if routeName != "" {
		snapshot = filter.Filter(snapshot, filter.Options{
			HTTPRouteName:      routeName,
			HTTPRouteNamespace: routeNS,
			RuleIndex:          ruleIndex,
			RuleName:           ruleName,
		})
		if len(snapshot.Listeners) == 0 {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"warning: no routes found for HTTPRoute %s/%s — check name, namespace, and that the HTTPRoute is attached to this Gateway\n",
				routeNS, routeName)
		}
	}

	// In live mode, the cluster's HTTPRoutes resolve route names with several
	// readings (D-006). Without them the output only lacks route origins.
	var known []model.Ref
	if file == "" {
		if known, err = kubeHTTPRoutes(context.Background(), kube.Options{KubeContext: kubeContext}); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: route origins unavailable: %v\n", err)
		}
	}
	snapshot = origin.Annotate(snapshot, mapper(gateway, namespace, routeName, routeNS, known))

	// Render and print — or launch the interactive TUI.
	switch {
	case interactive:
		return tui.Run(snapshot)
	case output == outputJSON:
		return writeJSON(cmd.OutOrStdout(), snapshot, result.Warnings)
	}
	fmt.Fprintln(cmd.OutOrStdout(), renderer.Render(snapshot))
	return nil
}
