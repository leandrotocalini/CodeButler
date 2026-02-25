package main

import (
	"flag"
	"fmt"
	"os"
)

// validRoles defines the set of agent roles supported by CodeButler.
var validRoles = map[string]bool{
	"pm":         true,
	"coder":      true,
	"reviewer":   true,
	"researcher": true,
	"artist":     true,
	"lead":       true,
}

func main() {
	role := flag.String("role", "", "Agent role (pm, coder, reviewer, researcher, artist, lead)")
	flag.Parse()

	if *role == "" {
		fmt.Fprintln(os.Stderr, "error: --role is required")
		flag.Usage()
		os.Exit(1)
	}

	if !validRoles[*role] {
		fmt.Fprintf(os.Stderr, "error: invalid role %q (valid: pm, coder, reviewer, researcher, artist, lead)\n", *role)
		os.Exit(1)
	}

	fmt.Printf("codebutler: role=%s\n", *role)
}
