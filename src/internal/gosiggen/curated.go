package gosiggen

// CuratedPackages is the H5 starter set of Go stdlib packages that the
// generator targets. Generation quality varies; not every export maps cleanly.
var CuratedPackages = []string{
	"strings",
	"fmt",
	"errors",
	"strconv",
	"bytes",
	"io",
	"os",
	"time",
	"context",
	"sync",
	"sync/atomic",
	"sort",
	"math",
	"math/rand",
	"net",
	"net/http",
	"database/sql",
	"encoding/json",
	"encoding/csv",
	"encoding/base64",
	"crypto/sha256",
	"log/slog",
}

// SmokePackages is a small subset used for CI / quick verification.
var SmokePackages = []string{
	"strings",
	"fmt",
	"errors",
	"strconv",
}

// IsCurated reports whether importPath is in the H5 curated set.
// M7 still generates for arbitrary paths; this only flags quality expectations.
func IsCurated(importPath string) bool {
	for _, p := range CuratedPackages {
		if p == importPath {
			return true
		}
	}
	return false
}
