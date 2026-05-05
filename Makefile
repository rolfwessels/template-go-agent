.DEFAULT_GOAL := help

# General Variables
date=$(shell date +'%y.%m.%d.%H.%M')
project := template-go-agent
binary := template-go-agent
container := dev
docker-file-check := /.dockerenv
docker-warning := ""
RED=\033[0;31m
GREEN=\033[0;32m
NC=\033[0m # No Color
versionPrefix := 0.1
version := $(versionPrefix).$(shell git rev-list HEAD --count 2>/dev/null || echo 0)
git-short-hash := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo 00000000)
version-suffix := ''
dockerhub := rolfwessels/template-go-agent

release := release
ifeq ($(env), dev)
 release := debug
 version-suffix:= ""
endif

ifdef GITHUB_BASE_REF
 current-branch := $(patsubst refs/heads/%,%,${GITHUB_HEAD_REF})
else ifdef GITHUB_REF
 current-branch := $(patsubst refs/heads/%,%,${GITHUB_REF})
else
 current-branch := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)
endif

ifeq ($(current-branch), main)
 docker-tags := -t $(dockerhub):alpha -t $(dockerhub):latest -t $(dockerhub):v$(version) -t $(dockerhub):$(git-short-hash)
 version-full := $(version)
else
 version := $(versionPrefix).$(shell git rev-list origin/main --count 2>/dev/null || echo 0).$(shell git rev-list origin/main..HEAD --count 2>/dev/null || echo 0)
 version-suffix := alpha
 version-full := $(version)-$(version-suffix)
 docker-tags := -t $(dockerhub):$(version-suffix) -t $(dockerhub):$(git-short-hash) -t $(dockerhub):v$(version-full)
endif

# Docker Warning
ifeq ("$(wildcard $(docker-file-check))","")
 docker-warning = "⚠️  WARNING: Can't find /.dockerenv - it's strongly recommended that you run this from within the docker container."
endif

# Targets
help:
	@echo "The following commands can be used for building & running & deploying the $(project)"
	@echo "-------------------------------------------------------------------------------------"
	@echo "Targets:"
	@echo " Docker Targets (run from local machine)"
	@echo " - up            : brings up the container & attach to the default container ($(container))"
	@echo " - down          : stops the container"
	@echo " - build         : (re)builds the container"
	@echo ""
	@echo " Service Targets (should only be run inside the docker container)"
	@echo " - version       : Show current version number"
	@echo " - start         : Run the $(project)"
	@echo " - vet           : Vet the $(project)"
	@echo " - test          : Vet and test the $(project)"
	@echo " - publish       : Publish the $(project)"
	@echo " - install       : Symlink dist/linux-amd64/$(binary) into ~/.local/bin (run on host)"
	@echo " - uninstall     : Remove the ~/.local/bin/$(binary) symlink (run on host)"
	@echo " - docker-login  : Login to docker registry"
	@echo " - docker-build  : Build the docker image"
	@echo " - docker-push   : Push the docker image"
	@echo " - docker-pull-short-tag : Pull the docker image based on git short hash"
	@echo " - docker-tag-env        : Tag the docker image based on the environment"
	@echo " - docker-publish        : Publish the docker image"
	@echo " - deploy        : Deploy the $(project)"
	@echo " - update-packages : Update Go dependencies"
	@echo ""
	@echo "Options:"
	@echo " - env : sets the environment - supported environments are: dev | prod"
	@echo ""
	@echo "Examples:"
	@echo " - Start Docker Container        : make up"
	@echo " - Rebuild Docker Container      : make build"
	@echo " - Rebuild & Start Docker Container : make build up"
	@echo " - Publish and deploy            : make publish deploy env=dev"

up:
	@echo "Starting containers..."
	@docker compose up -d
	@echo "Attaching shell..."
	@docker compose exec $(container) zsh

down:
	@echo "Stopping containers..."
	@docker compose down

build: down
	@echo "Building containers..."
	@docker compose build

version:
	@echo -e "Version ${GREEN}v$(version-full)${NC}"

print-version:
	@echo $(version-full)

start:
	@echo -e "Starting $(project)"
	@go run ./cmd/$(binary)

vet:
	@echo -e "Vetting ${GREEN}v$(version)${NC}"
	@go vet ./...

test: vet
	@echo -e "Testing ${GREEN}v$(version)${NC}"
	@go test ./... -v -count=1

PLATFORMS := linux-amd64 linux-arm64 windows-amd64 darwin-amd64 darwin-arm64

publish:
	@echo -e "Building ${GREEN}v$(version-full)${NC} release of $(project)"
	@rm -rf ./dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%-*}; arch=$${platform##*-}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		echo "  → $$platform"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build \
			-ldflags="-s -w -X 'main.version=$(version-full)'" \
			-o ./dist/$$platform/$(binary)$$ext ./cmd/$(binary) || exit 1; \
		if [ "$$os" = "windows" ]; then \
			(cd ./dist/$$platform && zip -q ../$(binary)-$$platform.zip $(binary)$$ext); \
		else \
			tar -czf ./dist/$(binary)-$$platform.tar.gz -C ./dist/$$platform $(binary); \
		fi; \
	done
	@echo "Artifacts in ./dist/"
	@ls -lh ./dist/*.tar.gz ./dist/*.zip

install:
	@test -f dist/linux-amd64/$(binary) || { printf "${RED}error${NC}: dist/linux-amd64/$(binary) not found — run 'make publish' first\n"; exit 1; }
	@mkdir -p $(HOME)/.local/bin
	@ln -sf "$(CURDIR)/dist/linux-amd64/$(binary)" "$(HOME)/.local/bin/$(binary)"
	@printf "Linked ${GREEN}$(HOME)/.local/bin/$(binary)${NC} → $(CURDIR)/dist/linux-amd64/$(binary)\n"
	@command -v $(binary) >/dev/null 2>&1 || printf "${RED}warning${NC}: $(HOME)/.local/bin not on PATH; add it to your shell profile\n"

uninstall:
	@rm -f "$(HOME)/.local/bin/$(binary)"
	@echo "Removed $(HOME)/.local/bin/$(binary)"

docker-login:
	@echo -e "Login to docker hub"
	@read -p "Username: " docker_username; \
	read -s -p "Password: " docker_password; \
	echo ""; \
	echo $$docker_password | docker login --username $$docker_username --password-stdin

docker-build:
	@echo -e "Building branch ${RED}$(current-branch)${NC} to ${GREEN}$(docker-tags)${NC} with ${GREEN}$(version-full)${NC}"
	@docker build --target runtime \
		--build-arg VERSION=$(version-full) \
		$(docker-tags) .

docker-push:
	@echo -e "Pushing to ${GREEN}$(dockerhub)${NC}"
	@docker push --all-tags $(dockerhub)

docker-pull-short-tag:
	@echo -e "Pulling ${GREEN}$(dockerhub):$(git-short-hash)${NC}"
	@docker pull "$(dockerhub):$(git-short-hash)"

docker-tag-env: env-check
	@echo -e "Tagging release ${GREEN}$(env)${NC}"
	@docker tag "$(dockerhub):$(git-short-hash)" "$(dockerhub):$(env)"
	@docker images | grep "$(dockerhub)"

docker-publish: docker-build docker-login docker-push
	@echo -e "Done"

deploy: env-check
	@echo -e "Deploying ${GREEN}v$(version-full)${NC}"

update-packages:
	@echo "Updating Go dependencies to latest versions..."
	@go get -u ./...
	@go mod tidy
	@echo "Update complete."

docker-check:
	$(call assert-file-exists,$(docker-file-check), This step should only be run from Docker. Please run `make up` first.)

env-check:
	$(call check_defined, env, No environment set. Supported environments are: [ dev | prod ]. Please set the env variable. e.g. `make env=dev deploy`)

# Check that given variables are set and all have non-empty values,
# die with an error otherwise.
check_defined = \
	$(strip $(foreach 1,$1, \
		$(call __check_defined,$1,$(strip $(value 2)))))
__check_defined = \
	$(if $(value $1),, \
		$(error Undefined $1$(if $2, ($2))))

define assert
  $(if $1,,$(error Assertion failed: $2))
endef

define assert-file-exists
  $(call assert,$(wildcard $1),$1 does not exist. $2)
endef
