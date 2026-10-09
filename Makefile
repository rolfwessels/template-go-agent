.DEFAULT_GOAL := help

binary := template-go-agent
dockerhub := rolfwessels/template-go-agent
versionPrefix := 0.1
git-short-hash := $(shell git rev-parse --short=8 HEAD 2>/dev/null || printf '00000000')
PLATFORMS := linux-amd64 linux-arm64 windows-amd64 darwin-amd64 darwin-arm64

ifdef GITHUB_BASE_REF
current-branch := $(patsubst refs/heads/%,%,$(GITHUB_HEAD_REF))
else ifdef GITHUB_REF
current-branch := $(patsubst refs/heads/%,%,$(GITHUB_REF))
else
current-branch := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || printf 'main')
endif

ifeq ($(current-branch),main)
version-full := $(versionPrefix).$(shell git rev-list HEAD --count 2>/dev/null || printf '0')
tags := alpha latest v$(version-full) $(git-short-hash)
else
version-full := $(versionPrefix).$(shell git rev-list origin/main --count 2>/dev/null || printf '0').$(shell git rev-list origin/main..HEAD --count 2>/dev/null || printf '0')-alpha
tags := alpha $(git-short-hash) v$(version-full)
endif
docker-tags := $(addprefix -t $(dockerhub):,$(tags))

.PHONY: help up down build shell start vet test publish version print-version \
	docker-build docker-push docker-publish warn-container

help: ## Show available commands (default).
	@awk 'BEGIN { FS = ":.*## " } /^[a-zA-Z0-9_-]+:.*## / { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

up: ## Start the dev container and attach a shell (host).
	@docker compose up -d
	@$(MAKE) --no-print-directory shell

down: ## Stop the dev container (host).
	@docker compose down

build: down ## Rebuild the dev container (host).
	@docker compose build

shell: ## Attach zsh to the running dev container (host).
	@docker compose exec dev zsh

version: ## Display the current version.
	@printf 'Version v%s\n' '$(version-full)'

print-version: ## Print only the version for scripts.
	@printf '%s\n' '$(version-full)'

start: | warn-container ## Run the agent (CLI or Discord).
	@go run ./cmd/$(binary)

vet: | warn-container ## Run go vet.
	@go vet ./...

test: vet ## Vet, then run all tests without cached results.
	@go test ./... -v -count=1

# Default prompts are embedded; each archive contains only the binary.
publish: | warn-container ## Build five platform archives in dist/ (requires tar/zip).
	@printf 'Building v%s release\n' '$(version-full)'
	@rm -rf ./dist
	@set -e; for platform in $(PLATFORMS); do \
		os=$${platform%-*}; arch=$${platform##*-}; ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		printf '  %s\n' "$$platform"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build \
			-ldflags="-s -w -X 'main.version=$(version-full)'" \
			-o ./dist/$$platform/$(binary)$$ext ./cmd/$(binary); \
		if [ "$$os" = "windows" ]; then \
			(cd ./dist/$$platform && zip -q ../$(binary)-$$platform.zip $(binary)$$ext); \
		else \
			tar -czf ./dist/$(binary)-$$platform.tar.gz -C ./dist/$$platform $(binary); \
		fi; \
	done
	@printf 'Artifacts in ./dist/\n'
	@ls -lh ./dist/*.tar.gz ./dist/*.zip

docker-build: ## Build the runtime image with version and branch tags.
	@docker build --target runtime \
		--build-arg VERSION=$(version-full) \
		$(docker-tags) .

docker-push: ## Push all local image tags (run docker login first).
	@docker push --all-tags $(dockerhub)

docker-publish: docker-build ## Build and push the runtime image (run docker login first).
	@$(MAKE) --no-print-directory docker-push

warn-container:
	@test -f /.dockerenv || printf '%s\n' 'Warning: run Go targets in the dev container (make up).' >&2
