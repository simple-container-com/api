# AI Development Environment Dockerfile
FROM node:18-alpine

# Install system dependencies
RUN apk add --no-cache \
    dumb-init \
    curl \
    docker-cli \
    kubectl \
    git \
    bash

# Create app user
RUN addgroup -g 1001 -S nodejs && \
    adduser -S nodejs -u 1001

# Set working directory
WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci --only=production && npm cache clean --force

# Copy source code
COPY --chown=nodejs:nodejs . .

# Create directories for runtime operations
RUN mkdir -p /app/runtime /app/logs && \
    chown -R nodejs:nodejs /app/runtime /app/logs

# Switch to non-root user
USER nodejs

# Expose port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

# Start the application
ENTRYPOINT ["dumb-init", "--"]
CMD ["node", "server.js"]
