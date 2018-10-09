//+build mage

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var (
	// buildContainer is a docker container used to build go binaries
	buildContainer = "golang:1.11.1"
)

type Build mg.Namespace

// Build is main entrypoint to build the project
func (Build) All() error {
	mg.Deps(Build.Go, Build.Docker)

	return nil
}

// GoBuild builds go binaries
func (Build) Go() error {
	fmt.Println("\n=====> Building Gravitational Wormhole Go Binary...\n")

	return sh.RunV(
		"docker",
		"run",
		"-it",
		"--rm=true",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/wormhole", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/wormhole/build/cache/go"`,
		buildContainer,
		"go",
		"--",
		"build",
		"-ldflags",
		flags(),
		"-o",
		"/go/src/github.com/gravitational/wormhole/build/wormhole",
		"github.com/gravitational/wormhole/cmd/wormhole",
	)
}

// DockerBuild builds a docker image for this project
func (Build) Docker() error {
	mg.Deps(Build.Go)
	fmt.Println("\n=====> Building Gravitational Wormhole Docker Image...\n")

	return sh.RunV(
		"docker",
		"build",
		"--tag",
		fmt.Sprint("wormhole:", version()),
		".",
	)
}

func srcDir() string {
	return os.Getenv("ROOT_DIR")
}

func flags() string {
	timestamp := time.Now().Format(time.RFC3339)
	hash := hash()
	version := version()

	flags := []string{
		fmt.Sprint(`-X "main.timestamp=`, timestamp, `"`),
		fmt.Sprint(`-X "main.commitHash=`, hash, `"`),
		fmt.Sprint(`-X "main.gitTag=`, version, `"`),
	}

	return strings.Join(flags, " ")
}

// hash returns the git hash for the current repository or "" if none.
func hash() string {
	hash, _ := sh.Output("git", "rev-parse", "--short", "HEAD")
	return hash
}

// version returns the git tag for the current branch or "" if none.
func version() string {
	s, _ := sh.Output("git", "describe", "--tags")
	if s == "" {
		s = "dev"
	}
	return s
}
