# ── base: shared Go toolchain ─────────────────────────────────────────────────
FROM golang:1.26-alpine AS base

RUN apk update \
  && apk upgrade \
  && apk add --no-cache \
    ca-certificates \
    git \
    curl \
    bash \
    make \
    zip \
    zsh \
    zsh-vcs \
  && update-ca-certificates \
  && git config --global --add safe.directory /template-go-agent

# ── build: compile the binary ─────────────────────────────────────────────────
FROM base AS build

ARG VERSION=dev
WORKDIR /template-go-agent

COPY go.mod ./
RUN go mod download

COPY . .
RUN GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X 'main.version=${VERSION}'" \
    -o /out/template-go-agent ./cmd/template-go-agent

# ── runtime: minimal production image ─────────────────────────────────────────
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates
COPY --from=build /out/template-go-agent /usr/local/bin/template-go-agent
ENTRYPOINT ["template-go-agent"]

# ── dev: full dev environment (used by docker-compose) ────────────────────────
FROM base AS dev

ARG UID=1000
ARG GID=1000

RUN addgroup -g ${GID} dev && \
    adduser -D -u ${UID} -G dev -s /bin/zsh dev && \
    mkdir -p /template-go-agent && chown dev:dev /template-go-agent

ENV HOME=/home/dev
ENV GOPATH=/home/dev/go
ENV GOCACHE=/home/dev/.cache/go-build
ENV PATH="/home/dev/go/bin:${PATH}"
ENV TERM=xterm-256color

USER dev

RUN mkdir -p /home/dev/.cache/go-build /home/dev/go/pkg/mod

RUN sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended && \
    git clone https://github.com/zsh-users/zsh-autosuggestions.git ${HOME}/.oh-my-zsh/custom/plugins/zsh-autosuggestions

WORKDIR /template-go-agent
