package version

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("astra %s (commit=%s date=%s go=%s)", Version, GitCommit, BuildDate, runtime.Version())
}
