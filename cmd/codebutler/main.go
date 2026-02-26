package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/leandrotocalini/codebutler/internal/skills"
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
	// Handle subcommands
	if len(os.Args) > 1 && os.Args[1] == "validate" {
		runValidate()
		return
	}

	role := flag.String("role", "", "Agent role (pm, coder, reviewer, researcher, artist, lead)")
	flag.Parse()

	if *role == "" {
		fmt.Fprintln(os.Stderr, "error: --role is required")
		fmt.Fprintln(os.Stderr, "usage: codebutler --role <role>")
		fmt.Fprintln(os.Stderr, "       codebutler validate [skills-dir]")
		flag.Usage()
		os.Exit(1)
	}

	if !validRoles[*role] {
		fmt.Fprintf(os.Stderr, "error: invalid role %q (valid: pm, coder, reviewer, researcher, artist, lead)\n", *role)
		os.Exit(1)
	}

	fmt.Printf("codebutler: role=%s\n", *role)
}

// runValidate validates all skill files in the given directory.
func runValidate() {
	skillsDir := ".codebutler/skills"
	if len(os.Args) > 2 {
		skillsDir = os.Args[2]
	}

	absDir, err := filepath.Abs(skillsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Validating skills in %s...\n", absDir)

	errs := skills.Validate(absDir)
	if len(errs) == 0 {
		fmt.Println("All skills valid.")
		return
	}

	fmt.Printf("Found %d validation error(s):\n", len(errs))
	for _, e := range errs {
		fmt.Printf("  %s\n", e.Error())
	}
	os.Exit(1)
}
