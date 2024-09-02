package print2pdf

import (
	"fmt"
	"time"
)

// Convert literal values to pointers.
func Ptr[T any](v T) *T {
	return &v
}

// Write Elapsed time, to be used with defer.
func Elapsed(message string) func() {
	start := time.Now()

	return func() {
		fmt.Printf("%s: %s\n", message, time.Since(start))
	}
}
