// Package hand is the hand-written Go baseline for list fold/map.
package hand

// FoldAdd sums xs starting from acc (same recursion shape as Goop list match).
func FoldAdd(xs []int, acc int) int {
	if len(xs) == 0 {
		return acc
	}
	return FoldAdd(xs[1:], acc+xs[0])
}

// MapInc returns a new slice with each element incremented.
func MapInc(xs []int) []int {
	if len(xs) == 0 {
		return nil
	}
	return append([]int{xs[0] + 1}, MapInc(xs[1:])...)
}
