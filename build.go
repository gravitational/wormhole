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
	"os"
	"path"
	"strings"
	"time"

	"github.com/gravitational/trace"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"
)

var (
	// buildContainer is a docker container used to build go binaries
	buildContainer = "golang:1.12.0"

	// golangciVersion is the version of golangci-lint to use for linting
	// https://github.com/golangci/golangci-lint/releases
	golangciVersion = "v1.15.0"

	// cniVersion is the version of cni plugin binaries to ship
	cniVersion = "v0.7.1"

	// registryImage is the docker tag to use to push the container to the requested registry
	registryImage = env("WORM_REGISTRY_IMAGE", "quay.io/gravitational/wormhole-dev")
)

// env, loads a variable from the environment, or uses the provided default
func env(env, d string) string {
	if os.Getenv(env) != "" {
		return os.Getenv(env)
	}
	return d
}

type Build mg.Namespace

// Build is main entrypoint to build the project
func (Build) All() error {
	mg.Deps(Build.Go)

	return nil
}

// GoBuild builds go binaries
func (Build) Go() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Building Gravitational Wormhole Go Binary...\n")
	start := time.Now()

	updated, err := target.Dir("build/wormhole", "pkg", "cmd")
	if err != nil {
		return trace.Wrap(err)
	}

	if !updated {
		fmt.Println("Build up to date")
		return nil
	}
	err = trace.Wrap(sh.RunV(
		"docker",
		"run",
		"-it",
		"--rm=true",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/wormhole:delegated", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/wormhole/build/cache/go"`,
		fmt.Sprint("wormhole-build:", version()),
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

	return trace.Wrap(err)
}

// DockerBuild builds a docker image for this project
func (Build) Docker() error {
	mg.Deps(Build.Go)
	fmt.Println("\n=====> Building Gravitational Wormhole Docker Image...\n")

	return trace.Wrap(sh.RunV(
		"docker",
		"build",
		"--pull",
		"--tag",
		fmt.Sprint("wormhole:", version()),
		"--build-arg",
		fmt.Sprint("CNI_VERSION=", cniVersion),
		"--build-arg",
		"ARCH=amd64",
		"--build-arg",
		fmt.Sprint("VERSION=", version()),
		"-f",
		"Dockerfile",
		".",
	))
}

func (Build) Publish() error {
	mg.Deps(Build.Docker)
	fmt.Println("\n=====> Publishing Gravitational Wormhole Docker Image...\n")

	err := sh.RunV(
		"docker",
		"tag",
		fmt.Sprint("wormhole:", version()),
		fmt.Sprint(registryImage, ":", version()),
	)
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(sh.RunV(
		"docker",
		"push",
		fmt.Sprint(registryImage, ":", version()),
	))
}

func (Build) BuildContainer() error {

	return trace.Wrap(sh.RunV(
		"docker",
		"build",
		"--pull",
		"--tag",
		fmt.Sprint("wormhole-build:", version()),
		"--build-arg",
		fmt.Sprint("BUILD_IMAGE=", buildContainer),
		"--build-arg",
		fmt.Sprint("GOLANGCI_VER=", golangciVersion),
		"-f",
		"Dockerfile.build",
		"./assets",
	))
}

type Test mg.Namespace

func (Test) All() error {
	mg.Deps(Test.Unit, Test.Lint)
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
		fmt.Sprint("wormhole-build:", version()),
		"go",
		"--",
		"test",
		"./...",
		"-race",
	))
}

// Lint runs golangci linter against the repo
func (Test) Lint() error {
	mg.Deps(Build.BuildContainer)
	fmt.Println("\n=====> Linting Gravitational Wormhole...\n")

	return trace.Wrap(sh.RunV(
		"docker",
		"run",
		"-it",
		"--rm=true",
		fmt.Sprintf("--volume=%v:/go/src/github.com/gravitational/wormhole", srcDir()),
		`--env="GOCACHE=/go/src/github.com/gravitational/wormhole/build/cache/go"`,
		fmt.Sprint("wormhole-build:", version()),
		"bash",
		"-c",
		"cd /go/src/github.com/gravitational/wormhole; golangci-lint run --deadline=30m --enable-all"+
			" -D gochecknoglobals -D gochecknoinits",
	))
}

func srcDir() string {
	return path.Join(os.Getenv("GOPATH"), "src/github.com/gravitational/wormhole/")
}

func flags() string {
	timestamp := time.Now().Format(time.RFC3339)
	hash := hash()
	version := version()

	flags := []string{
		fmt.Sprint(`-X "main.timestamp=`, timestamp, `"`),
		fmt.Sprint(`-X "main.commitHash=`, hash, `"`),
		fmt.Sprint(`-X "main.gitTag=`, version, `"`),
		"-s -w", // shrink the binary
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
	//shortTag, _ := sh.Output("git", "describe", "--tags", "--abbrev=0")
	longTag, _ := sh.Output("git", "describe", "--tags", "--dirty")

	return longTag
}
