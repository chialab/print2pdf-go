package print2pdf

import (
	"fmt"
	"math"
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

// Human readable representation of an IEC byte size (taken from https://github.com/dustin/go-humanize/blob/master/bytes.go).
func HumanizeBytes(s uint64) string {
	var base float64 = 1024
	sizes := []string{"B", "KiB", "MiB", "GiB"}
	if s < 10 {
		return fmt.Sprintf("%d B", s)
	}
	e := math.Floor(math.Log(float64(s)) / math.Log(base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	f := "%.1f %s"

	return fmt.Sprintf(f, val, suffix)
}
