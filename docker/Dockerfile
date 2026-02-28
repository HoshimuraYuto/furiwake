FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends curl ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# Install latest furiwake binary (skip systemd service setup)
RUN curl -fsSL https://github.com/HoshimuraYuto/furiwake/releases/latest/download/install.sh | \
    bash -s -- --bin-dir /usr/local/bin --config-dir /etc/furiwake --service no --start-service no

WORKDIR /app

EXPOSE 52860

# Mount furiwake.yaml at /app/furiwake.yaml via volume
CMD ["furiwake", "--config", "/app/furiwake.yaml"]
