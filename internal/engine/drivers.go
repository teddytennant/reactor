package engine

import (
	"github.com/reactor-sec/reactor/internal/chamber"
	"github.com/reactor-sec/reactor/internal/chamber/daytona"
	"github.com/reactor-sec/reactor/internal/chamber/local"
)

// selectDrivers orders chamber drivers by preference: a real Daytona sandbox
// when credentials are present (the sponsor story — "we live inside it", SPEC
// §8), otherwise the local throwaway tree. The local driver is always appended
// as a fallback so a detonation can never be left with nowhere to run.
func selectDrivers() []chamber.Driver {
	var ds []chamber.Driver
	if d := daytona.New(); d.Available() {
		ds = append(ds, d)
	}
	ds = append(ds, local.New())
	return ds
}
