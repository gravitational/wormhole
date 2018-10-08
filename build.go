//+build mage

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Build is main entrypoint to build the project
func Build() error {
	fmt.Println("Building Gravitational Wormhole...")
	goCmd := mg.GoCmd()

	fmt.Println("Using flags: [", flags(), "]")

	return sh.RunV(goCmd, "build", "-ldflags", flags(), "github.com/gravitational/wormhole/cmd/wormhole")
}

func flags() string {
	timestamp := time.Now().Format(time.RFC3339)
	hash := hash()
	tag := tag()
	if tag == "" {
		tag = "dev"
	}

	flags := []string{
		fmt.Sprint(`-X "github.com/gravitational/wormhole/cmd/wormhole.timestamp=`, timestamp, `"`),
		fmt.Sprint(`-X "github.com/gravitational/wormhole/cmd/wormhole.commitHash=`, hash, `"`),
		fmt.Sprint(`-X "github.com/gravitational/wormhole/cmd/wormhole.gitTag=`, tag, `"`),
	}

	return strings.Join(flags, " ")
}

// hash returns the git hash for the current repo or "" if none.
func hash() string {
	hash, _ := sh.Output("git", "rev-parse", "--short", "HEAD")
	return hash
}

// tag returns the git tag for the current branch or "" if none.
func tag() string {
	s, _ := sh.Output("git", "describe", "--tags")
	return s
}
