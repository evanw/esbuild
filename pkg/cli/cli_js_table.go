package cli

import "github.com/evanw/esbuild/pkg/api"

var validEngines = map[string]api.EngineName{
	"chrome":  api.EngineChrome,
	"edge":    api.EngineEdge,
	"firefox": api.EngineFirefox,
	"ie":      api.EngineIE,
	"ios":     api.EngineIOS,
	"node":    api.EngineNode,
	"opera":   api.EngineOpera,
	"safari":  api.EngineSafari,
}
