package server

import "runtime/debug"

const (
	modulePath = "github.com/Digital-Creators-Team/slot-game-module"
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
