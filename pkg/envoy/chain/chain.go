// Package chain resolves the HTTP filter chain one route runs: which HCM
// filters run, which per-filter config decided that, and in which order they
// see the request and the response.
package chain

import (
	"strings"

	"github.com/DuncanDoyle/xdsview/pkg/envoy/model"
)

const filterConfigType = "envoy.config.route.v3.FilterConfig"

// level is one place per-filter config can sit.
type level struct {
	level model.ConfigLevel
	pfc   map[string]any
}

// Effective resolves r's chain from the HCM filters and the per-filter config
// at route, virtual host and route config level, most specific first, as Envoy
// does (source/common/router/config_impl.cc, RouteEntryImplBase::filterDisabled).
// A weighted-cluster backend's config isn't resolved: it marks the chain
// Conditional, and the entries are the baseline before backend selection.
func Effective(hcm model.HCMConfig, vh model.VirtualHost, r model.Route) model.EffectiveChain {
	levels := []level{{model.LevelRoute, r.TypedPerFilterConfig}, {model.LevelVirtualHost, vh.TypedPerFilterConfig}}
	if hcm.RouteConfig != nil {
		levels = append(levels, level{model.LevelRouteConfig, hcm.RouteConfig.TypedPerFilterConfig})
	}

	var c model.EffectiveChain
	for _, f := range hcm.HTTPFilters {
		e := model.ChainEntry{Name: f.Name, Type: f.Type, Runs: !f.Disabled, State: model.FilterOn}
		if f.Disabled {
			e.State = model.FilterOff
		}
		e.Request, e.Response = sidesOf(f.Type)
		for _, l := range levels {
			if v, ok := l.pfc[f.Name]; ok {
				cfg, _ := v.(map[string]any)
				e.Level = l.level
				e.PerFilterType, _ = cfg["@type"].(string)
				e.Runs, e.State = decide(f.Disabled, cfg)
				break
			}
		}
		c.Filters = append(c.Filters, e)
	}
	for _, wc := range r.WeightedClusters {
		if len(wc.TypedPerFilterConfig) > 0 {
			c.Conditional = true
		}
	}
	return c
}

// decide applies one per-filter config entry to a filter the HCM enables or
// disables. A FilterConfig with disabled set wins over its config; one with
// neither field is invalid, and Envoy rejects it
// (source/common/router/per_filter_config.cc, PerFilterConfigs).
func decide(hcmDisabled bool, cfg map[string]any) (bool, model.FilterState) {
	if typ, _ := cfg["@type"].(string); strings.HasSuffix(typ, filterConfigType) {
		if d, _ := cfg["disabled"].(bool); d {
			if hcmDisabled {
				return false, model.FilterOff
			}
			return false, model.FilterDisabled
		}
		if _, ok := cfg["config"]; !ok {
			return false, model.FilterUnknown
		}
	}
	if hcmDisabled {
		return true, model.FilterEnabled
	}
	return true, model.FilterOn
}
