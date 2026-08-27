// Package meta carries the tool's identity so that provenance written into a
// manifest or a document says the same thing everywhere.
package meta

// Name and URL identify the tool in provenance records. A translated document
// that says which tool produced it can be traced back and re-run; one that does
// not looks like something a person wrote.
const (
	Name = "mdsplit"
	URL  = "https://github.com/mlechner911/mdsplit"
)

// Version is set from the VERSION file at build time via ldflags.
var Version = "dev"

// Stamp renders "mdsplit 1.3.0".
func Stamp() string { return Name + " " + Version }
