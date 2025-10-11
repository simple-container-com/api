package security

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// SecurityLevel defines the security enforcement level
type SecurityLevel string

const (
	SecurityLevelLow        SecurityLevel = "low"
	SecurityLevelMedium     SecurityLevel = "medium"
	SecurityLevelHigh       SecurityLevel = "high"
	SecurityLevelEnterprise SecurityLevel = "enterprise"
)

// SecurityConfig holds security configuration
type SecurityConfig struct {
	Level                  SecurityLevel `json:"level"`
	EnableInputValidation  bool          `json:"enable_input_validation"`
	EnableRateLimiting     bool          `json:"enable_rate_limiting"`
	MaxRequestsPerMinute   int           `json:"max_requests_per_minute"`
	EnableIPWhitelist      bool          `json:"enable_ip_whitelist"`
	AllowedIPs             []string      `json:"allowed_ips"`
	EnableAPIKeyAuth       bool          `json:"enable_api_key_auth"`
	RequireHTTPS           bool          `json:"require_https"`
	EnableAuditLogging     bool          `json:"enable_audit_logging"`
	MaxPromptLength        int           `json:"max_prompt_length"`
	MaxFileSize            int64         `json:"max_file_size"`
	BlockSensitivePatterns bool          `json:"block_sensitive_patterns"`
	EnableContentFiltering bool          `json:"enable_content_filtering"`
}

// SecurityManager handles security enforcement
type SecurityManager struct {
	config      SecurityConfig
	logger      logger.Logger
	rateLimiter *RateLimiter
	validator   *InputValidator
	auditor     *AuditLogger
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config SecurityConfig, logger logger.Logger) *SecurityManager {
	return &SecurityManager{
		config:      config,
		logger:      logger,
		rateLimiter: NewRateLimiter(config.MaxRequestsPerMinute, logger),
		validator:   NewInputValidator(logger),
		auditor:     NewAuditLogger(logger),
	}
}

// ValidateRequest performs comprehensive request validation
func (sm *SecurityManager) ValidateRequest(ctx context.Context, req *SecurityRequest) error {
	if !sm.config.EnableInputValidation {
		return nil
	}

	// Rate limiting
	if sm.config.EnableRateLimiting {
		if err := sm.rateLimiter.Allow(ctx, req.ClientIP); err != nil {
			sm.auditor.LogSecurityEvent(ctx, "rate_limit_exceeded", req)
			return fmt.Errorf("rate limit exceeded: %w", err)
		}
	}

	// IP whitelist validation
	if sm.config.EnableIPWhitelist {
		if !sm.isIPAllowed(req.ClientIP) {
			sm.auditor.LogSecurityEvent(ctx, "ip_not_whitelisted", req)
			return fmt.Errorf("IP address %s not in whitelist", req.ClientIP)
		}
	}

	// API key validation
	if sm.config.EnableAPIKeyAuth && req.APIKey != "" {
		if !sm.validateAPIKey(req.APIKey) {
			sm.auditor.LogSecurityEvent(ctx, "invalid_api_key", req)
			return fmt.Errorf("invalid API key")
		}
	}

	// HTTPS requirement
	if sm.config.RequireHTTPS && !req.IsHTTPS {
		sm.auditor.LogSecurityEvent(ctx, "https_required", req)
		return fmt.Errorf("HTTPS is required")
	}

	// Input validation
	if err := sm.validator.ValidateInput(ctx, req); err != nil {
		sm.auditor.LogSecurityEvent(ctx, "input_validation_failed", req)
		return fmt.Errorf("input validation failed: %w", err)
	}

	// Content filtering
	if sm.config.EnableContentFiltering {
		if err := sm.validator.FilterContent(ctx, req); err != nil {
			sm.auditor.LogSecurityEvent(ctx, "content_filtering_blocked", req)
			return fmt.Errorf("content blocked by security filter: %w", err)
		}
	}

	// Log successful validation for audit trail
	if sm.config.EnableAuditLogging {
		sm.auditor.LogSecurityEvent(ctx, "request_validated", req)
	}

	return nil
}

// isIPAllowed checks if an IP is in the whitelist
func (sm *SecurityManager) isIPAllowed(clientIP string) bool {
	if len(sm.config.AllowedIPs) == 0 {
		return true // No whitelist configured
	}

	for _, allowedIP := range sm.config.AllowedIPs {
		if sm.matchIP(clientIP, allowedIP) {
			return true
		}
	}
	return false
}

// matchIP checks if an IP matches a pattern (supports CIDR)
func (sm *SecurityManager) matchIP(clientIP, pattern string) bool {
	// Direct match
	if clientIP == pattern {
		return true
	}

	// CIDR match
	if strings.Contains(pattern, "/") {
		_, network, err := net.ParseCIDR(pattern)
		if err == nil {
			ip := net.ParseIP(clientIP)
			if ip != nil && network.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// validateAPIKey validates an API key (simplified implementation)
func (sm *SecurityManager) validateAPIKey(apiKey string) bool {
	// In production, this would validate against a secure store
	// For now, check basic format and length
	if len(apiKey) < 32 {
		return false
	}

	// Check if it looks like a valid API key format
	validFormat := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	return validFormat.MatchString(apiKey)
}

// SecurityRequest represents a request to validate
type SecurityRequest struct {
	ClientIP  string            `json:"client_ip"`
	UserAgent string            `json:"user_agent"`
	APIKey    string            `json:"api_key"`
	IsHTTPS   bool              `json:"is_https"`
	Endpoint  string            `json:"endpoint"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body"`
	Timestamp time.Time         `json:"timestamp"`
	UserID    string            `json:"user_id"`
}

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	requests map[string][]time.Time
	limit    int
	logger   logger.Logger
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute int, logger logger.Logger) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    requestsPerMinute,
		logger:   logger,
	}
}

// Allow checks if a request is allowed for the given client
func (rl *RateLimiter) Allow(ctx context.Context, clientIP string) error {
	now := time.Now()
	cutoff := now.Add(-time.Minute)

	// Clean old requests
	if requests, exists := rl.requests[clientIP]; exists {
		filtered := make([]time.Time, 0, len(requests))
		for _, t := range requests {
			if t.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		rl.requests[clientIP] = filtered
	}

	// Check current request count
	currentRequests := len(rl.requests[clientIP])
	if currentRequests >= rl.limit {
		rl.logger.Warn(ctx, "Rate limit exceeded: client_ip=%s, current_requests=%d, limit=%d", clientIP, currentRequests, rl.limit)
		return fmt.Errorf("rate limit of %d requests per minute exceeded", rl.limit)
	}

	// Add current request
	rl.requests[clientIP] = append(rl.requests[clientIP], now)
	return nil
}

// InputValidator validates and sanitizes input
type InputValidator struct {
	logger            logger.Logger
	sensitivePatterns []*regexp.Regexp
}

// NewInputValidator creates a new input validator
func NewInputValidator(logger logger.Logger) *InputValidator {
	// Common sensitive patterns to block
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["']?([^"'\s]+)`),
		regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["']?([^"'\s]+)`),
		regexp.MustCompile(`(?i)(secret|token)\s*[:=]\s*["']?([^"'\s]+)`),
		regexp.MustCompile(`(?i)aws_access_key_id\s*[:=]\s*["']?([^"'\s]+)`),
		regexp.MustCompile(`(?i)aws_secret_access_key\s*[:=]\s*["']?([^"'\s]+)`),
		regexp.MustCompile(`(?i)(private[_-]?key)\s*[:=]\s*["']?([^"'\s]+)`),
	}

	return &InputValidator{
		logger:            logger,
		sensitivePatterns: patterns,
	}
}

// ValidateInput validates request input
func (iv *InputValidator) ValidateInput(ctx context.Context, req *SecurityRequest) error {
	// Check prompt length
	if len(req.Body) > 10000 { // 10KB limit
		return fmt.Errorf("request body too large: %d bytes", len(req.Body))
	}

	// Check for suspicious patterns
	if iv.containsSuspiciousPatterns(req.Body) {
		iv.logger.Warn(ctx, "Suspicious pattern detected in request: client_ip=%s, endpoint=%s", req.ClientIP, req.Endpoint)
		return fmt.Errorf("request contains suspicious patterns")
	}

	return nil
}

// FilterContent filters potentially harmful content
func (iv *InputValidator) FilterContent(ctx context.Context, req *SecurityRequest) error {
	// Check for sensitive information
	for _, pattern := range iv.sensitivePatterns {
		if pattern.MatchString(req.Body) {
			return fmt.Errorf("request contains sensitive information pattern")
		}
	}

	return nil
}

// containsSuspiciousPatterns checks for common attack patterns
func (iv *InputValidator) containsSuspiciousPatterns(input string) bool {
	suspiciousPatterns := []string{
		"<script",
		"javascript:",
		"data:text/html",
		"../",
		"..\\",
		"SELECT * FROM",
		"DROP TABLE",
		"UNION SELECT",
		"eval(",
		"exec(",
	}

	lowerInput := strings.ToLower(input)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerInput, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// AuditLogger handles security audit logging
type AuditLogger struct {
	logger logger.Logger
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger logger.Logger) *AuditLogger {
	return &AuditLogger{logger: logger}
}

// LogSecurityEvent logs a security-related event
func (al *AuditLogger) LogSecurityEvent(ctx context.Context, eventType string, req *SecurityRequest) {
	al.logger.Info(ctx, "Security audit event: event_type=%s, client_ip=%s, user_agent=%s, endpoint=%s, method=%s, timestamp=%v, user_id=%s",
		eventType, req.ClientIP, req.UserAgent, req.Endpoint, req.Method, req.Timestamp, req.UserID)
}

// GenerateSecureToken generates a cryptographically secure token
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// SecureCompare performs constant-time string comparison
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// GetSecurityConfig returns a security configuration based on the environment
func GetSecurityConfig(environment string) SecurityConfig {
	switch strings.ToLower(environment) {
	case "production", "prod":
		return SecurityConfig{
			Level:                  SecurityLevelHigh,
			EnableInputValidation:  true,
			EnableRateLimiting:     true,
			MaxRequestsPerMinute:   100,
			EnableIPWhitelist:      false, // Configure as needed
			EnableAPIKeyAuth:       true,
			RequireHTTPS:           true,
			EnableAuditLogging:     true,
			MaxPromptLength:        8192,
			MaxFileSize:            10 * 1024 * 1024, // 10MB
			BlockSensitivePatterns: true,
			EnableContentFiltering: true,
		}
	case "staging", "stage":
		return SecurityConfig{
			Level:                  SecurityLevelMedium,
			EnableInputValidation:  true,
			EnableRateLimiting:     true,
			MaxRequestsPerMinute:   200,
			EnableIPWhitelist:      false,
			EnableAPIKeyAuth:       false,
			RequireHTTPS:           false,
			EnableAuditLogging:     true,
			MaxPromptLength:        16384,
			MaxFileSize:            50 * 1024 * 1024, // 50MB
			BlockSensitivePatterns: true,
			EnableContentFiltering: true,
		}
	case "development", "dev", "local":
		return SecurityConfig{
			Level:                  SecurityLevelLow,
			EnableInputValidation:  true,
			EnableRateLimiting:     false,
			MaxRequestsPerMinute:   1000,
			EnableIPWhitelist:      false,
			EnableAPIKeyAuth:       false,
			RequireHTTPS:           false,
			EnableAuditLogging:     false,
			MaxPromptLength:        32768,
			MaxFileSize:            100 * 1024 * 1024, // 100MB
			BlockSensitivePatterns: false,
			EnableContentFiltering: false,
		}
	default:
		// Default to medium security
		return GetSecurityConfig("staging")
	}
}
