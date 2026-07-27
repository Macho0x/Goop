// Package hand is the hand-written Go baseline for branded ID wrap/unwrap.
// Uses a defined string type (closest idiomatic Go stand-in for a brand).
package hand

type OrderID string

func Wrap(s string) OrderID { return OrderID(s) }

func Unwrap(id OrderID) string { return string(id) }

func Roundtrip(s string) string { return Unwrap(Wrap(s)) }
