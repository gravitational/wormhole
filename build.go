//+build mage

/*
Copyright 2018 Gravitational, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var (
	// buildContainer is a docker container used to build go binaries
	buildContainer = "golang:1.11.1"

	golangciVersion = "v1.10.1"
)

type Build mg.Namespace

// Build is main entrypoint to build the project
func (Build) All() error {
	mg.Deps(Build.Go, Build.Docker)

	return nil
}

// GoBuild builds go binaries
func (Build) Go() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Building Gravitational Wormhole Go Binary...\n")
	start := time.Now()

	err := trace.Wrap(sh.RunV(
		"docker",
		"run",
		"-it",
		"--rm=true",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/wormhole:delegated", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/wormhole/build/cache/go"`,
		"wormhole-build:dev",
		"go",
		"--",
		"build",
		"-ldflags",
		flags(),
		"-o",
		"/go/src/github.com/gravitational/wormhole/build/wormhole",
		"github.com/gravitational/wormhole/cmd/wormhole",
	))

	elapsed := time.Since(start)
	fmt.Println("Build completed in ", elapsed)

	return err
}

// DockerBuild builds a docker image for this project
func (Build) Docker() error {
	mg.Deps(Build.Go)
	fmt.Println("\n=====> Building Gravitational Wormhole Docker Image...\n")

	return trace.Wrap(sh.RunV(
		"docker",
		"build",
		"--tag",
		fmt.Sprint("wormhole:", version()),
		"-f",
		"Dockerfile",
		".",
	))
}

func (Build) BuildContainer() error {

	fmt.Println("\n=====> Building Gravitational Wormhole Build Container...\n")
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer os.RemoveAll(dir)
	fmt.Println("Using temp directory: ", dir)

	return trace.Wrap(sh.RunV(
		"docker",
		"build",
		"--tag",
		"wormhole-build:dev",
		"--build-arg",
		fmt.Sprint("BUILD_IMAGE=", buildContainer),
		"--build-arg",
		fmt.Sprint("GOLANGCI_VER=", golangciVersion),
		"-f",
		"Dockerfile.build",
		dir,
	))
}

type Test mg.Namespace

func (Test) All() error {
	mg.Deps(Test.Unit)
	return nil
}

func (Test) Unit() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Running Gravitational Wormhole Unit Tests...\n")

	return trace.Wrap(sh.RunV(
		"docker",
		"run",
		"-it",
		"--rm=true",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/wormhole", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/wormhole/build/cache/go"`,
		`-w=/go/src/github.com/gravitational/wormhole/`,
		"wormhole-build:dev",
		"go",
		"--",
		"test",
		"./...",
		"-race",
	))
}

// Lint runs golangci linter against the repo
func Lint() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Linting Gravitational Wormhole...\n")

	return trace.Wrap(sh.RunV(
		"docker",
		"run",
		"-it",
		"--rm=true",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/wormhole", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/wormhole/build/cache/go"`,
		"wormhole-build:dev",
		"golangci-lint",
		"run",
		"--enable-all",
		"/go/src/github.com/gravitational/wormhole/...",
	))
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
