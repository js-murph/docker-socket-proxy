package main

import (
	"fmt"
	"strings"
)

func main() {
	pattern := "/v1.*/containers/.*"
	globPattern := strings.ReplaceAll(pattern, ".*", "*")
	fmt.Printf("Pattern: %s\n", pattern)
	fmt.Printf("Glob pattern: %s\n", globPattern)
	parts := strings.Split(globPattern, "*")
	fmt.Printf("Parts: %v\n", parts)

	// Test the matching
	s := "/v1.42/containers/json"
	fmt.Printf("String: %s\n", s)

	// Check prefix
	fmt.Printf("Has prefix %s: %v\n", parts[0], strings.HasPrefix(s, parts[0]))

	// Check suffix
	fmt.Printf("Has suffix %s: %v\n", parts[len(parts)-1], strings.HasSuffix(s, parts[len(parts)-1]))
}
