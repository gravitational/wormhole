# TODO: Docs / License

.DEFAULT_GOAL := build

ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

export

define mage
	go run mage.go $(1)
endef

.PHONY: build
build:
	$(call mage,build:all)

.PHONY: publish
publish:
	$(call mage,build:publish)

.PHONY: mage
mage:
	$(call mage,$(filter-out $@,$(MAKECMDGOALS)))

.PHONY: lint
lint:
	$(call mage,test:lint)

.PHONY: test
test:
	$(call mage,test:all)