package server

import "runtime/debug"

const (
	modulePath = "gl.owvn.org/veuve/server/slot-game-module.git"
)

var version = moduleVersion()

func GetVersion() string {
	return version
}

func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	if info.Main.Path == modulePath {
		return info.Main.Version
	}

	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			return dep.Version
		}
	}

	return "unknown"
}
