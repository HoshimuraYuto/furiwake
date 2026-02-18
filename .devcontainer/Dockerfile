FROM mcr.microsoft.com/devcontainers/typescript-node:24-bookworm

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
      tmux \
      golang-go \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

ENV CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1
