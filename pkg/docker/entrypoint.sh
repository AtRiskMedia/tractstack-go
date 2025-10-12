#!/bin/bash
set -e

# Create the data directory if it doesn't exist (e.g., on first run with a new volume).
mkdir -p /home/sandbox/t8k/t8k-go-server

# Change ownership of the data directory to the sandbox user. This is the
# critical step that fixes the permission error on the Docker volume mount.
chown -R sandbox:sandbox /home/sandbox/t8k/t8k-go-server

# Use su-exec to drop privileges from root and execute the main
# container command (the CMD from the Dockerfile).
exec su-exec sandbox "$@"
