package main

import "fmt"

// Distinct defined types — same idea as Goop single-ctor brands.
type OrderID string
type Symbol string

func place(sym Symbol, oid OrderID) string {
	return "placed"
}

func main() {
	oid := OrderID("ord-1")
	sym := Symbol("ETH-USD")
	fmt.Println(place(sym, oid))
}
