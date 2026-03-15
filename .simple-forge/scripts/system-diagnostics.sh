#!/bin/bash

# System diagnostics and failure analysis script
# Provides comprehensive system state information for debugging

echo "🔧 System diagnostics:"

# Basic system information
echo "- Timestamp: $(date)"
echo "- Hostname: $(hostname)"
echo "- User: $(whoami)"
echo "- Working directory: $(pwd)"

# Resource usage
echo "- Memory: $(free -h | grep Mem | awk '{print $3"/"$2}' 2>/dev/null || echo 'unknown')"
echo "- Disk space: $(df -h . | tail -1 | awk '{print $4" available"}' 2>/dev/null || echo 'unknown')"
echo "- CPU load: $(uptime | awk -F'load average:' '{print $2}' 2>/dev/null || echo 'unknown')"

# Network connectivity
if ping -c 1 8.8.8.8 >/dev/null 2>&1; then
  echo "- Network: connected"
else
  echo "- Network: issues detected"
fi

# Process information
echo "- Total processes: $(ps aux | wc -l)"
echo "- Claude processes: $(ps aux | grep -c claude || echo 0)"

# File system information
if [ -f claude_response.txt ]; then
  echo "- Response file size: $(wc -c < claude_response.txt 2>/dev/null || echo 0) bytes"
fi

if [ -f claude_error.txt ]; then
  echo "- Error file size: $(wc -c < claude_error.txt 2>/dev/null || echo 0) bytes"
fi

if [ -f prompt.txt ]; then
  echo "- Prompt file size: $(wc -c < prompt.txt 2>/dev/null || echo 0) bytes"
fi

# Environment variables (sanitized)
echo "- PATH length: ${#PATH} characters"
echo "- HOME: $HOME"
echo "- SHELL: $SHELL"

# Check for common issues
if [ ! -d ~/.local/bin ]; then
  echo "⚠️ ~/.local/bin directory not found"
fi

if [ ! -x ~/.local/bin/claude ]; then
  echo "⚠️ Claude binary not found or not executable in ~/.local/bin"
fi

# Show recent system logs (if available)
if command -v journalctl >/dev/null 2>&1; then
  echo "- Recent system errors:"
  journalctl --since "5 minutes ago" --priority=err --no-pager -n 3 2>/dev/null || echo "  No recent errors"
fi
