# Multi-stage build for Meteor.js application
FROM node:18-alpine AS builder

# Install Meteor
RUN npm install -g meteor

# Set working directory
WORKDIR /app

# Copy package files
COPY package*.json ./
COPY .meteor ./.meteor

# Copy source code
COPY . .

# Install dependencies
RUN meteor npm install

# Build the application
RUN meteor build --directory /app/build --architecture os.linux.x86_64

# Production stage
FROM node:18-alpine

# Install dumb-init for proper signal handling
RUN apk add --no-cache dumb-init

# Create app user
RUN addgroup -g 1001 -S meteor && \
    adduser -S meteor -u 1001

# Set working directory
WORKDIR /app

# Copy built application
COPY --from=builder --chown=meteor:meteor /app/build/bundle .

# Install production dependencies
RUN cd programs/server && npm install --production

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
