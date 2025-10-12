# Dockerfile: A final, self-contained, and portable TractStack Sandbox appliance

# 1. Start with a standard Debian Linux base image.
FROM debian:bullseye-slim

# Set environment to prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# 2. Install all system-level prerequisites as the root user.
# Includes build tools which will be removed later.
RUN apt-get update && apt-get install -y \
    curl \
    git \
    ca-certificates \
    gnupg \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# 3. Securely compile the su-exec utility from the trusted source repository.
RUN git clone https://github.com/ncopa/su-exec.git /tmp/su-exec \
    && cd /tmp/su-exec \
    && make \
    && mv /tmp/su-exec/su-exec /usr/local/bin/ \
    && rm -rf /tmp/su-exec

# 4. Install Go, Node.js, pnpm, and Python dependencies.
RUN curl -fsSL https://go.dev/dl/go1.22.5.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"
RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
RUN apt-get install -y nodejs
RUN npm install -g pnpm
RUN apt-get install -y python3 python3-pip && pip3 install beautifulsoup4

# 5. Create a dedicated, unprivileged user named 'sandbox'.
RUN useradd --create-home --shell /bin/bash sandbox

# 6. Copy the entrypoint script into the image and make it executable.
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh

# 7. Switch to the 'sandbox' user to run the installer.
USER sandbox
WORKDIR /home/sandbox

# 8. Run the TractStack installer. This requires 'git' to be available.
RUN curl -fsSL https://get.tractstack.com | bash -s -- --quick --non-interactive

# 9. Switch back to the 'root' user to perform final cleanup.
USER root

# 10. Clean up build-only dependencies to keep the final image small.
RUN apt-get purge -y --auto-remove build-essential git

# 11. Expose the Astro development server port.
EXPOSE 4321
EXPOSE 8080

# 12. Set the entrypoint to our permission-fixing script.
ENTRYPOINT ["entrypoint.sh"]

# 13. Define the default command to be executed by the entrypoint.
CMD ["/bin/bash", "-c", "\
  (cd /home/sandbox/t8k/src/tractstack-go && ./tractstack-go) & \
  echo Allowing backend to bootstrap; \
  sleep 15; \
  (cd /home/sandbox/t8k/src/my-tractstack && pnpm dev --host)"]
