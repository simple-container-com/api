FROM ubuntu:22.04

# Install required system packages
RUN apt-get update && apt-get install -y \
    git \
    curl \
    jq \
    wget \
    unzip \
    docker.io \
    openssh-client \
    python3 \
    python3-pip \
    yq \
    && rm -rf /var/lib/apt/lists/*

# Copy pre-built Simple Container CLI binary (built during welder build process)
COPY dist/linux-amd64/sc /usr/local/bin/sc
RUN chmod +x /usr/local/bin/sc

# Install Pulumi
RUN curl -fsSL https://get.pulumi.com | sh
ENV PATH="/root/.pulumi/bin:${PATH}"

# Install GitHub CLI for release operations
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update \
    && apt-get install -y gh

# Copy pre-built Go binary from welder build
COPY dist/github-actions /usr/local/bin/github-actions
RUN chmod +x /usr/local/bin/github-actions

# Set working directory
WORKDIR /workspace

# Use Go binary as entrypoint with deploy action type
ENTRYPOINT ["/usr/local/bin/github-actions", "deploy-client-stack"]
