package chain

import "github.com/DuncanDoyle/xdsview/pkg/envoy/model"

// sides records, per filter type, whether Envoy installs the filter as a
// decoder (request side), an encoder (response side) or both. Each entry cites
// the add*Filter call in the filter's factory at Envoy v1.39.6 (aacd0340), the
// Envoy kgateway v2.5.0 ships. Paths are under source/extensions/filters/http/
// unless noted. A filter installed on a side may still pass through it.
var sides = map[string][2]model.Side{
	"envoy.extensions.filters.http.router.v3.Router":                       decoder, // router/config.cc:26
	"envoy.extensions.filters.http.set_metadata.v3.Config":                 decoder, // set_metadata/config.cc:24
	"envoy.extensions.filters.http.cors.v3.Cors":                           both,    // cors/config.cc:26
	"envoy.extensions.filters.http.ratelimit.v3.RateLimit":                 both,    // ratelimit/config.cc:43
	"envoy.extensions.filters.http.ext_authz.v3.ExtAuthz":                  both,    // ext_authz/config.cc:45,70
	"envoy.extensions.filters.http.dynamic_modules.v3.DynamicModuleFilter": both,    // dynamic_modules/factory.cc:72
	"envoy.extensions.filters.http.composite.v3.Composite":                 both,    // composite/config.cc:113
	"envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor":          both,    // ext_proc/config.cc:103
	"envoy.extensions.filters.http.rbac.v3.RBAC":                           decoder, // rbac/config.cc:23
	"envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit":      both,    // local_ratelimit/config.cc:24
	"envoy.extensions.filters.http.grpc_web.v3.GrpcWeb":                    both,    // grpc_web/config.cc:16
	"envoy.extensions.filters.http.fault.v3.HTTPFault":                     both,    // fault/config.cc:22
	"envoy.extensions.filters.http.csrf.v3.CsrfPolicy":                     decoder, // csrf/config.cc:20
	"envoy.extensions.filters.http.upstream_codec.v3.UpstreamCodec":        decoder, // source/common/router/upstream_codec_filter.h:121
	"envoy.extensions.filters.http.buffer.v3.Buffer":                       decoder, // buffer/config.cc:25
	"envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication":         decoder, // jwt_authn/filter_factory.cc:49
	"envoy.extensions.filters.http.lua.v3.Lua":                             both,    // lua/config.cc:24
	"envoy.extensions.filters.http.header_mutation.v3.HeaderMutation":      both,    // header_mutation/config.cc:21
	"envoy.extensions.filters.http.compressor.v3.Compressor":               both,    // compressor/config.cc:39
	"envoy.extensions.filters.http.oauth2.v3.OAuth2":                       both,    // oauth2/config.cc:152
	"envoy.extensions.filters.http.basic_auth.v3.BasicAuth":                decoder, // basic_auth/config.cc:80
	"envoy.extensions.filters.http.api_key_auth.v3.ApiKeyAuth":             decoder, // api_key_auth/config.cc:21
	"envoy.extensions.filters.http.stateful_session.v3.StatefulSession":    both,    // stateful_session/config.cc:21
	// kgateway's transformation filter, from solo-io/envoy-gloo (2fac0b83):
	// transformation/transformation_filter_config_factory.cc:29
	"envoy.api.v2.filter.http.FilterTransformations": both,
}

var (
	decoder = [2]model.Side{model.SideYes, model.SideNo}
	both    = [2]model.Side{model.SideYes, model.SideYes}
)

// sidesOf returns the request and response side of a filter type URL.
func sidesOf(typeURL string) (model.Side, model.Side) {
	if s, ok := sides[typeName(typeURL)]; ok {
		return s[0], s[1]
	}
	return model.SideUnknown, model.SideUnknown
}
