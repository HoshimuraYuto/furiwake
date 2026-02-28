FROM mcr.microsoft.com/devcontainers/typescript-node:24-bookworm

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
      tmux \
      golang-go \
  && apt-get clean \
  && rm -rf /var/lib/apt/lists/*

USER node
RUN curl -fsSL https://claude.ai/install.sh | bash
USER root

RUN npm i -g @openai/codex @google/gemini-cli

ENV TZ=Asia/Tokyo

ENV CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1
