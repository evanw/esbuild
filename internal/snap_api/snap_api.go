package snap_api

import (
	"github.com/evanw/esbuild/internal/resolver"
	"github.com/evanw/esbuild/pkg/api"
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

func CreateShouldReplaceRequire(
	platform api.Platform,
	external []string,
	replaceRequire api.ShouldReplaceRequirePredicate, ) api.ShouldReplaceRequirePredicate {
	isExternal := IsExternalModule(platform, external)
	return func(mdl string) bool {
		return isExternal(mdl) || replaceRequire(mdl)
	}
}
