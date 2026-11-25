// Package chain resolves the HTTP filter chain each route runs: which HCM
// filters run, which per-filter config decided that, and in which order they
// see the request and the response. It reads Envoy data only.
package chain

import (
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

const (
	filterConfigType = "envoy.config.route.v3.FilterConfig"
	matcherType      = "envoy.extensions.common.matching.v3.ExtensionWithMatcher"
	compositeType    = "envoy.extensions.filters.http.composite.v3.Composite"
)

// level is one place per-filter config can sit.
type level struct {
	level model.ConfigLevel
	pfc   map[string]any
}

// Effective resolves the chain of every route in snap, keyed by route ID. It
// reads snap only. An HCM whose route config isn't joined has its HCM default
// chain under its route config use ID, so callers can show what it would run.
//
// Per-filter config is read at route, virtual host and route config level,
// most specific first, as Envoy does (source/common/router/config_impl.cc,
// RouteEntryImplBase::filterDisabled). A weighted-cluster backend's config
// isn't resolved: it marks the chain Conditional, and the entries are the
// baseline before backend selection.
func Effective(snap *model.Snapshot) map[model.ID]model.EffectiveChain {
	out := map[model.ID]model.EffectiveChain{}
	for _, l := range snap.Listeners {
		for _, fc := range l.FilterChains {
			hcm := fc.HCM
			if hcm == nil {
				continue
			}
			rc := hcm.RouteConfig
			if rc == nil {
				out[hcm.RouteConfigUse.ID] = resolve(hcm.HTTPFilters, nil, nil)
				continue
			}
			for _, vh := range rc.VirtualHosts {
				for _, r := range vh.Routes {
					levels := []level{{model.LevelRoute, r.TypedPerFilterConfig}, {model.LevelVirtualHost, vh.TypedPerFilterConfig}, {model.LevelRouteConfig, rc.TypedPerFilterConfig}}
					out[r.ID] = resolve(hcm.HTTPFilters, levels, r.WeightedClusters)
				}
			}
		}
	}
	return out
}

// resolve builds one chain from the HCM filters and the per-filter config levels.
func resolve(filters []model.HTTPFilter, levels []level, backends []model.WeightedCluster) model.EffectiveChain {
	var c model.EffectiveChain
	for _, f := range filters {
		e := model.ChainEntry{Slot: f.ID, Name: f.Name, Type: f.Type, Runs: model.TriYes, State: model.FilterOn, ConfigSource: model.ConfigInline}
		if f.Disabled {
			e.Runs, e.State = model.TriNo, model.FilterOff
		}
		installed := installedType(f)
		e.Request, e.Response = sidesOf(installed)
		e.Wrapper = wrapper(f, installed)
		e.Conditional = e.Wrapper != ""
		for _, l := range levels {
			if v, ok := l.pfc[f.Name]; ok {
				cfg, _ := v.(map[string]any)
				e.Level = l.level
				e.PerFilterType, _ = cfg["@type"].(string)
				e.Runs, e.State = decide(f.Disabled, cfg)
				break
			}
		}
		// An ECDS filter with no config in the dump: whether it runs, and as
		// what, is unknown unless the route doesn't run it at all.
		if d := f.Discovery; d != nil {
			e.ConfigSource = d.Source
			if d.Source == "" && e.Runs != model.TriNo {
				e.Runs, e.State = model.TriUnknown, model.FilterUnresolved
			}
		}
		c.Filters = append(c.Filters, e)
	}
	for _, wc := range backends {
		if len(wc.TypedPerFilterConfig) > 0 {
			c.Conditional = true
		}
	}
	return c
}

// installedType is the type of the filter Envoy installs. f.Type can name a
// composite's delegate, but the composite runs on both sides whatever the
// delegate. An ExtensionWithMatcher keeps its inner filter's sides
// (match_delegate/config.h), so its inner type counts.
func installedType(f model.HTTPFilter) string {
	typ, _ := f.TypedConfig["@type"].(string)
	if typeName(typ) == matcherType {
		ext, _ := f.TypedConfig["extension_config"].(map[string]any)
		inner, _ := ext["typed_config"].(map[string]any)
		typ, _ = inner["@type"].(string)
	}
	if typ == "" {
		return f.Type
	}
	return typ
}

// wrapper names the matcher wrapper around f, if any. A Composite needs a
// matcher to pick its delegate, so it wins over the ExtensionWithMatcher around it.
func wrapper(f model.HTTPFilter, installed string) string {
	switch {
	case typeName(installed) == compositeType:
		return model.WrapperComposite
	case typeName(stringOf(f.TypedConfig["@type"])) == matcherType:
		return model.WrapperExtensionWithMatcher
	}
	return ""
}

func stringOf(v any) string { s, _ := v.(string); return s }

// typeName strips the type URL prefix: "type.googleapis.com/a.B" → "a.B".
func typeName(typeURL string) string { return typeURL[strings.LastIndex(typeURL, "/")+1:] }

// decide applies one per-filter config entry to a filter the HCM enables or
// disables. A FilterConfig with disabled set wins over its config; one with
// neither field is invalid, and Envoy rejects it, so whether the filter runs is
// unknown (source/common/router/per_filter_config.cc, PerFilterConfigs).
func decide(hcmDisabled bool, cfg map[string]any) (model.Tri, model.FilterState) {
	if typeName(stringOf(cfg["@type"])) == filterConfigType {
		if d, _ := cfg["disabled"].(bool); d {
			if hcmDisabled {
				return model.TriNo, model.FilterOff
			}
			return model.TriNo, model.FilterDisabled
		}
		// ProtoJSON leaves a null message unset, so it counts as missing.
		if cfg["config"] == nil {
			return model.TriUnknown, model.FilterUnknown
		}
	}
	if hcmDisabled {
		return model.TriYes, model.FilterEnabled
	}
	return model.TriYes, model.FilterOn
}
