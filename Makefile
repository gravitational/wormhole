# TODO: Docs / License

.DEFAULT_GOAL := build
mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
current_dir := $(notdir $(patsubst %/,%,$(dir $(mkfile_path))))
ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))


.PHONY: build
build:
	docker build \
	-t quay.io/gravitational/wormhole:latest \
	$(ROOT_DIR)