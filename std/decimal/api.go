// Package decimal re-exports github.com/shopspring/decimal behind a stable
// import path for mixed Go packages and future Goop `import go` targeting.
//
// The Goop module std/decimal/decimal.goop currently imports shopspring
// directly (cache builds resolve it via `go mod tidy`). This helper is the
// intended long-term boundary so callers do not depend on shopspring's
// surface API.
//
// Money and trading quantities must use Decimal, not float64 — see
// docs/design/25-decimal.md and docs/design/12-trading-bot-safety.md.
package decimal

import shopspring "github.com/shopspring/decimal"

// Decimal is an arbitrary-precision fixed-point decimal.
type Decimal = shopspring.Decimal

// FromString parses s, or returns an error on invalid input.
func FromString(s string) (Decimal, error) {
	return shopspring.NewFromString(s)
}

// MustFromString parses s, or panics on invalid input.
func MustFromString(s string) Decimal {
	return shopspring.RequireFromString(s)
}

// FromInt converts an int64 to Decimal.
func FromInt(n int64) Decimal {
	return shopspring.NewFromInt(n)
}

// Zero is the decimal zero value.
func Zero() Decimal {
	return shopspring.Zero
}
