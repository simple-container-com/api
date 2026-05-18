# Multi-stage build for Meteor.js application
FROM node:22-alpine@sha256:757ec364de4d37cedf30871be2988927660834e656e9aa52aad9ac194814c30c AS builder

# Install Meteor CLI at a pinned version (Scorecard Pinned-Dependencies).
# Bump as needed for your app; do not leave unpinned.
ARG METEOR_VERSION=3.2.2
RUN npm install -g meteor@${METEOR_VERSION}

# Set working directory
WORKDIR /app

# Copy package files. For reproducible builds commit a package-lock.json
# alongside your Meteor app; the install step below will pick it up.
COPY package*.json ./
COPY .meteor ./.meteor

# Copy source code
COPY . .

# Install dependencies. Uses `meteor npm ci` (reproducible from
# package-lock.json) when a lockfile is present, else falls back to
# `meteor npm install` so this example builds without one. Production
# apps should commit a lockfile to take the ci path.
RUN if [ -f package-lock.json ]; then meteor npm ci; else meteor npm install; fi

# Build the application
RUN meteor build --directory /app/build --architecture os.linux.x86_64

# Production stage
FROM node:22-alpine@sha256:757ec364de4d37cedf30871be2988927660834e656e9aa52aad9ac194814c30c

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create app user
RUN addgroup -g 1001 -S meteor && \
    adduser -S meteor -u 1001

# Set working directory
WORKDIR /app

# Copy built application
COPY --from=builder --chown=meteor:meteor /app/build/bundle .

# Install production dependencies. `meteor build` may or may not emit a
# package-lock.json into programs/server depending on the Meteor version
# and project layout; use the lockfile-aware install pattern.
RUN cd programs/server && if [ -f package-lock.json ]; then npm ci --omit=dev; else npm install --omit=dev; fi

# Switch to non-root user
USER meteor

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD node -e "require('http').get('http://localhost:3000/', (res) => { process.exit(res.statusCode === 200 ? 0 : 1) })"

# Start the application
ENTRYPOINT ["dumb-init", "--"]
CMD ["node", "main.js"]
