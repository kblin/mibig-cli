package editor

import (
	"slices"
	"strconv"
)

func contains(list []string, s string) bool { return slices.Contains(list, s) }
func itoa(i int) string                     { return strconv.Itoa(i) }
