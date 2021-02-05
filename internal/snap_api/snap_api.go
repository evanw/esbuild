package snap_api

import (
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/pkg/api"
	"strings"
)

func IsExternalModule(platform api.Platform, external []string) api.ShouldReplaceRequirePredicate {
	return func(mdl string) bool {
		if platform == api.PlatformNode {
			if _, ok := resolver.BuiltInNodeModules[mdl]; ok {
				return true
			}
		}
		for _, ext := range external {
			if ext == mdl {
				return true
			}
		}
		return false
	}
}

func IsNative(mdl string) bool {
	return strings.HasSuffix(mdl, ".node")
}

func CreateShouldReplaceRequire(
	platform api.Platform,
	external []string,
	replaceRequire api.ShouldReplaceRequirePredicate,
	rewriteModule api.ShouldRewriteModulePredicate,
) api.ShouldReplaceRequirePredicate {
	isExternal := IsExternalModule(platform, external)
	return func(mdl string) bool {
		return isExternal(mdl) || IsNative(mdl) || replaceRequire(mdl) || !rewriteModule(mdl)
	}
}
