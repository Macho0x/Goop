package main

import (
	"errors"
	"fmt"
)

func div(x, y float64) (float64, error) {
	if y == 0 {
		return 0, errors.New("division by zero")
	}
	return x / y, nil
}

func compute() (float64, error) {
	a, err := div(10, 2)
	if err != nil {
		return 0, err
	}
	b, err := div(6, 3)
	if err != nil {
		return 0, err
	}
	return a + b, nil
}

func main() {
	x, err := compute()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(x)
}
