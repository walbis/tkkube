package security

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AuthMethod defines authentication methods
type AuthMethod string

const (
	AuthMethodAPIKey    AuthMethod = "api_key"
	AuthMethodBasic     AuthMethod = "basic"
	AuthMethodBearer    AuthMethod = "bearer"
	AuthMethodWebhook   AuthMethod = "webhook"
	AuthMethodMutualTLS AuthMethod = "mutual_tls"
	AuthMethodOAuth2    AuthMethod = "oauth2"
)

// Role defines user roles in the system
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleOperator  Role = "operator"
	RoleReadOnly  Role = "readonly"
	RoleWebhook   Role = "webhook"
	RoleService   Role = "service"
	RoleGuest     Role = "guest"
)

// Permission defines specific permissions
type Permission string

const (
	PermissionRead             Permission = "read"
	PermissionWrite            Permission = "write"
	PermissionDelete           Permission = "delete"
	PermissionExecute          Permission = "execute"
	PermissionManageSecrets    Permission = "manage_secrets"
	PermissionManageUsers      Permission = "manage_users"
	PermissionManageConfig     Permission = "manage_config"
	PermissionViewAuditLogs    Permission = "view_audit_logs"
	PermissionTriggerPipeline  Permission = "trigger_pipeline"
	PermissionViewMetrics      Permission = "view_metrics"
	PermissionWebhookAccess    Permission = "webhook_access"
	PermissionBackupAccess     Permission = "backup_access"
	PermissionGitOpsAccess     Permission = "gitops_access"
)

// Resource defines system resources
type Resource string

const (
	ResourceSecrets     Resource = "secrets"
	ResourceConfig      Resource = "config"
	ResourceUsers       Resource = "users"
	ResourceAuditLogs   Resource = "audit_logs"
	ResourcePipelines   Resource = "pipelines"
	ResourceMetrics     Resource = "metrics"
	ResourceWebhooks    Resource = "webhooks"
	ResourceBackups     Resource = "backups"
	ResourceGitOps      Resource = "gitops"
	ResourceSystem      Resource = "system"
)

// User represents a system user
type User struct {
	ID          string            `json:"id"`
	Username    string            `json:"username"`
	Email       string            `json:"email,omitempty"`
	PasswordHash string           `json:"password_hash,omitempty"`
	Roles       []Role            `json:"roles"`
	APIKeys     []APIKey          `json:"api_keys,omitempty"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	LastLogin   *time.Time        `json:"last_login,omitempty"`
	LoginCount  int               `json:"login_count"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

// APIKey represents an API key for authentication
type APIKey struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	KeyHash     string            `json:"key_hash"`
	Prefix      string            `json:"prefix"`
	Permissions []Permission      `json:"permissions,omitempty"`
	Roles       []Role            `json:"roles,omitempty"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	LastUsed    *time.Time        `json:"last_used,omitempty"`
	UsageCount  int               `json:"usage_count"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Session represents an authenticated session
type Session struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Username  string            `json:"username"`
	Roles     []Role            `json:"roles"`
	Method    AuthMethod        `json:"method"`
	SourceIP  string            `json:"source_ip"`
	UserAgent string            `json:"user_agent"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// AuthConfig configures authentication and authorization
type AuthConfig struct {
	Enabled                bool              `yaml:"enabled"`
	RequireAuthentication  bool              `yaml:"require_authentication"`
	DefaultRole           Role              `yaml:"default_role"`
	SessionTimeout        time.Duration     `yaml:"session_timeout"`
	MaxSessionsPerUser    int               `yaml:"max_sessions_per_user"`
	PasswordMinLength     int               `yaml:"password_min_length"`
	PasswordRequireSpecial bool             `yaml:"password_require_special"`
	APIKeyEnabled         bool              `yaml:"api_key_enabled"`
	APIKeyLength          int               `yaml:"api_key_length"`
	RateLimiting          RateLimitConfig   `yaml:"rate_limiting"`
	WebhookAuth           WebhookAuthConfig `yaml:"webhook_auth"`
	RBAC                  RBACConfig        `yaml:"rbac"`
}

// RateLimitConfig configures rate limiting
type RateLimitConfig struct {
	Enabled         bool          `yaml:"enabled"`
	RequestsPerMin  int           `yaml:"requests_per_minute"`
	BurstSize       int           `yaml:"burst_size"`
	BlockDuration   time.Duration `yaml:"block_duration"`
	WhitelistIPs    []string      `yaml:"whitelist_ips"`
}

// RBACConfig configures role-based access control
type RBACConfig struct {
	Enabled     bool                       `yaml:"enabled"`
	StrictMode  bool                       `yaml:"strict_mode"`
	Roles       map[Role]RoleDefinition    `yaml:"roles"`
	Resources   map[Resource][]Permission  `yaml:"resources"`
}

// RoleDefinition defines permissions for a role
type RoleDefinition struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Permissions map[Resource][]Permission `json:"permissions"`
	Inherits    []Role                    `json:"inherits,omitempty"`
}

// AuthManager provides authentication and authorization services
type AuthManager struct {
	config      AuthConfig
	logger      Logger
	auditor     *AuditLogger
	users       map[string]*User
	sessions    map[string]*Session
	apiKeys     map[string]*APIKey
	rateLimiter map[string]*RateLimiter
	mu          sync.RWMutex
}

// RateLimiter tracks rate limiting for IPs/users
type RateLimiter struct {
	Requests   []time.Time
	BlockedUntil time.Time
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(config AuthConfig, logger Logger, auditor *AuditLogger) *AuthManager {
	am := &AuthManager{
		config:      config,
		logger:      logger,
		auditor:     auditor,
		users:       make(map[string]*User),
		sessions:    make(map[string]*Session),
		apiKeys:     make(map[string]*APIKey),
		rateLimiter: make(map[string]*RateLimiter),
	}

	// Initialize default RBAC if enabled
	if config.RBAC.Enabled {
		am.initializeDefaultRBAC()
	}

	// Create default admin user if none exists
	am.createDefaultAdmin()

	return am
}

// DefaultAuthConfig returns default authentication configuration
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		Enabled:               true,
		RequireAuthentication: true,
		DefaultRole:          RoleGuest,
		SessionTimeout:       24 * time.Hour,
		MaxSessionsPerUser:   5,
		PasswordMinLength:    12,
		PasswordRequireSpecial: true,
		APIKeyEnabled:        true,
		APIKeyLength:         32,
		RateLimiting: RateLimitConfig{
			Enabled:        true,
			RequestsPerMin: 60,
			BurstSize:      10,
			BlockDuration:  15 * time.Minute,
		},
		WebhookAuth: WebhookAuthConfig{
			Enabled:    true,
			Token:      "",
			HeaderName: "Authorization",
		},
		RBAC: RBACConfig{
			Enabled:    true,
			StrictMode: true,
			Roles:      make(map[Role]RoleDefinition),
			Resources:  make(map[Resource][]Permission),
		},
	}
}

// Authenticate validates credentials and returns a session
func (am *AuthManager) Authenticate(ctx context.Context, method AuthMethod, credentials map[string]string) (*Session, error) {
	sourceIP := credentials["source_ip"]
	userAgent := credentials["user_agent"]

	// Check rate limiting
	if am.config.RateLimiting.Enabled {
		if blocked := am.checkRateLimit(sourceIP); blocked {
			am.auditor.LogAuthenticationEvent("", sourceIP, string(method), "failure", "rate limit exceeded")
			return nil, fmt.Errorf("rate limit exceeded")
		}
	}

	var user *User
	var err error

	// Authenticate based on method
	switch method {
	case AuthMethodAPIKey:
		user, err = am.authenticateAPIKey(credentials["api_key"])
	case AuthMethodBasic:
		user, err = am.authenticateBasic(credentials["username"], credentials["password"])
	case AuthMethodBearer:
		user, err = am.authenticateBearer(credentials["token"])
	case AuthMethodWebhook:
		user, err = am.authenticateWebhook(credentials["token"], credentials["header"])
	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", method)
	}

	if err != nil {
		am.recordFailedAuth(sourceIP)
		am.auditor.LogAuthenticationEvent("", sourceIP, string(method), "failure", err.Error())
		return nil, err
	}

	// Check if user is enabled
	if !user.Enabled {
		am.auditor.LogAuthenticationEvent(user.ID, sourceIP, string(method), "failure", "user disabled")
		return nil, fmt.Errorf("user account disabled")
	}

	// Check user expiration
	if user.ExpiresAt != nil && time.Now().After(*user.ExpiresAt) {
		am.auditor.LogAuthenticationEvent(user.ID, sourceIP, string(method), "failure", "user expired")
		return nil, fmt.Errorf("user account expired")
	}

	// Create session
	session := am.createSession(user, method, sourceIP, userAgent)

	// Update user login stats
	am.updateUserLoginStats(user)

	am.auditor.LogAuthenticationEvent(user.ID, sourceIP, string(method), "success", "")
	am.logger.Info("user authenticated", map[string]interface{}{
		"user_id":   user.ID,
		"username":  user.Username,
		"method":    string(method),
		"source_ip": sourceIP,
	})

	return session, nil
}

// Authorize checks if a session has permission for a resource action
func (am *AuthManager) Authorize(session *Session, resource Resource, permission Permission) error {
	if !am.config.RBAC.Enabled {
		return nil // Authorization disabled
	}

	// Check session validity
	if time.Now().After(session.ExpiresAt) {
		return fmt.Errorf("session expired")
	}

	// Check permissions
	if !am.hasPermission(session.Roles, resource, permission) {
		am.auditor.LogAuthorizationEvent(session.UserID, session.SourceIP, 
			string(permission), string(resource), "denied", "insufficient permissions")
		return fmt.Errorf("permission denied: %s on %s", permission, resource)
	}

	am.auditor.LogAuthorizationEvent(session.UserID, session.SourceIP,
		string(permission), string(resource), "granted", "")

	return nil
}

// CreateUser creates a new user
func (am *AuthManager) CreateUser(username, email, password string, roles []Role) (*User, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if user already exists
	for _, user := range am.users {
		if user.Username == username {
			return nil, fmt.Errorf("user already exists: %s", username)
		}
	}

	// Validate password
	if err := am.validatePassword(password); err != nil {
		return nil, fmt.Errorf("password validation failed: %v", err)
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %v", err)
	}

	// Create user
	user := &User{
		ID:           am.generateUserID(),
		Username:     username,
		Email:        email,
		PasswordHash: string(passwordHash),
		Roles:        roles,
		Enabled:      true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		LoginCount:   0,
	}

	am.users[user.ID] = user

	am.logger.Info("user created", map[string]interface{}{
		"user_id":  user.ID,
		"username": username,
		"roles":    roles,
	})

	return user, nil
}

// CreateAPIKey creates a new API key for a user
func (am *AuthManager) CreateAPIKey(userID, name string, permissions []Permission, expiresAt *time.Time) (*APIKey, string, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if user exists
	user, exists := am.users[userID]
	if !exists {
		return nil, "", fmt.Errorf("user not found: %s", userID)
	}

	// Generate API key
	keyBytes := make([]byte, am.config.APIKeyLength)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %v", err)
	}
	
	rawKey := base64.URLEncoding.EncodeToString(keyBytes)
	prefix := rawKey[:8]
	
	// Hash the key
	keyHash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash API key: %v", err)
	}

	apiKey := &APIKey{
		ID:          am.generateAPIKeyID(),
		Name:        name,
		KeyHash:     string(keyHash),
		Prefix:      prefix,
		Permissions: permissions,
		Roles:       user.Roles,
		Enabled:     true,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	}

	am.apiKeys[prefix] = apiKey

	am.logger.Info("API key created", map[string]interface{}{
		"api_key_id": apiKey.ID,
		"user_id":    userID,
		"name":       name,
		"prefix":     prefix,
	})

	return apiKey, rawKey, nil
}

// ValidateSession validates a session ID and returns the session
func (am *AuthManager) ValidateSession(sessionID string) (*Session, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	session, exists := am.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		delete(am.sessions, sessionID)
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

// RevokeSession revokes a session
func (am *AuthManager) RevokeSession(sessionID string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	session, exists := am.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}

	delete(am.sessions, sessionID)

	am.auditor.LogSystemEvent("session_revoked", "auth_manager", "success",
		fmt.Sprintf("Session revoked for user %s", session.UserID))

	return nil
}

// RevokeAPIKey revokes an API key
func (am *AuthManager) RevokeAPIKey(prefix string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	apiKey, exists := am.apiKeys[prefix]
	if !exists {
		return fmt.Errorf("API key not found")
	}

	delete(am.apiKeys, prefix)

	am.auditor.LogSystemEvent("api_key_revoked", "auth_manager", "success",
		fmt.Sprintf("API key revoked: %s", apiKey.Name))

	return nil
}

// authenticateAPIKey authenticates using an API key
func (am *AuthManager) authenticateAPIKey(apiKey string) (*User, error) {
	if len(apiKey) < 8 {
		return nil, fmt.Errorf("invalid API key format")
	}

	prefix := apiKey[:8]
	
	am.mu.RLock()
	key, exists := am.apiKeys[prefix]
	am.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("invalid API key")
	}

	if !key.Enabled {
		return nil, fmt.Errorf("API key disabled")
	}

	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return nil, fmt.Errorf("API key expired")
	}

	// Verify key hash
	if err := bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(apiKey)); err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	// Update usage stats
	am.updateAPIKeyUsage(key)

	// Find user by roles (simplified - in production, link to specific user)
	for _, user := range am.users {
		if am.rolesMatch(user.Roles, key.Roles) {
			return user, nil
		}
	}

	return nil, fmt.Errorf("no user found for API key")
}

// authenticateBasic authenticates using username/password
func (am *AuthManager) authenticateBasic(username, password string) (*User, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	for _, user := range am.users {
		if user.Username == username {
			if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err == nil {
				return user, nil
			}
			return nil, fmt.Errorf("invalid credentials")
		}
	}

	return nil, fmt.Errorf("user not found")
}

// authenticateBearer authenticates using a bearer token
func (am *AuthManager) authenticateBearer(token string) (*User, error) {
	// In a real implementation, this would validate JWT or other tokens
	// For now, treat it as an API key
	return am.authenticateAPIKey(token)
}

// authenticateWebhook authenticates webhook requests
func (am *AuthManager) authenticateWebhook(token, header string) (*User, error) {
	if !am.config.WebhookAuth.Enabled {
		return nil, fmt.Errorf("webhook authentication disabled")
	}

	expectedToken := am.config.WebhookAuth.Token
	if expectedToken == "" {
		return nil, fmt.Errorf("webhook token not configured")
	}

	// Constant time comparison
	if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
		return nil, fmt.Errorf("invalid webhook token")
	}

	// Return a webhook service user
	return &User{
		ID:       "webhook-service",
		Username: "webhook",
		Roles:    []Role{RoleWebhook},
		Enabled:  true,
	}, nil
}

// createSession creates a new session
func (am *AuthManager) createSession(user *User, method AuthMethod, sourceIP, userAgent string) *Session {
	am.mu.Lock()
	defer am.mu.Unlock()

	sessionID := am.generateSessionID()
	session := &Session{
		ID:        sessionID,
		UserID:    user.ID,
		Username:  user.Username,
		Roles:     user.Roles,
		Method:    method,
		SourceIP:  sourceIP,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(am.config.SessionTimeout),
	}

	// Check session limit per user
	if am.config.MaxSessionsPerUser > 0 {
		am.cleanupUserSessions(user.ID)
	}

	am.sessions[sessionID] = session
	return session
}

// hasPermission checks if roles have permission for resource
func (am *AuthManager) hasPermission(userRoles []Role, resource Resource, permission Permission) bool {
	for _, role := range userRoles {
		if roleDef, exists := am.config.RBAC.Roles[role]; exists {
			if perms, hasResource := roleDef.Permissions[resource]; hasResource {
				for _, perm := range perms {
					if perm == permission {
						return true
					}
				}
			}
			
			// Check inherited roles
			for _, inheritedRole := range roleDef.Inherits {
				if am.hasPermission([]Role{inheritedRole}, resource, permission) {
					return true
				}
			}
		}
	}
	return false
}

// checkRateLimit checks if an IP is rate limited
func (am *AuthManager) checkRateLimit(ip string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	limiter, exists := am.rateLimiter[ip]
	if !exists {
		limiter = &RateLimiter{Requests: []time.Time{}}
		am.rateLimiter[ip] = limiter
	}

	now := time.Now()

	// Check if still blocked
	if now.Before(limiter.BlockedUntil) {
		return true
	}

	// Clean old requests
	cutoff := now.Add(-time.Minute)
	var validRequests []time.Time
	for _, req := range limiter.Requests {
		if req.After(cutoff) {
			validRequests = append(validRequests, req)
		}
	}
	limiter.Requests = validRequests

	// Check rate limit
	if len(limiter.Requests) >= am.config.RateLimiting.RequestsPerMin {
		limiter.BlockedUntil = now.Add(am.config.RateLimiting.BlockDuration)
		return true
	}

	// Add current request
	limiter.Requests = append(limiter.Requests, now)
	return false
}

// recordFailedAuth records a failed authentication attempt
func (am *AuthManager) recordFailedAuth(ip string) {
	// This would typically be used for brute force detection
	am.logger.Warn("authentication failed", map[string]interface{}{
		"source_ip": ip,
	})
}

// Helper methods

func (am *AuthManager) generateUserID() string {
	return generateSecureID("user")
}

func (am *AuthManager) generateSessionID() string {
	return generateSecureID("sess")
}

func (am *AuthManager) generateAPIKeyID() string {
	return generateSecureID("key")
}

func generateSecureID(prefix string) string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%s_%s", prefix, base64.URLEncoding.EncodeToString(bytes))
}

func (am *AuthManager) validatePassword(password string) error {
	if len(password) < am.config.PasswordMinLength {
		return fmt.Errorf("password must be at least %d characters", am.config.PasswordMinLength)
	}

	if am.config.PasswordRequireSpecial {
		hasSpecial := false
		hasUpper := false
		hasLower := false
		hasDigit := false

		for _, char := range password {
			switch {
			case char >= 'A' && char <= 'Z':
				hasUpper = true
			case char >= 'a' && char <= 'z':
				hasLower = true
			case char >= '0' && char <= '9':
				hasDigit = true
			case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
				hasSpecial = true
			}
		}

		if !hasSpecial || !hasUpper || !hasLower || !hasDigit {
			return fmt.Errorf("password must contain uppercase, lowercase, digit, and special character")
		}
	}

	return nil
}

func (am *AuthManager) updateUserLoginStats(user *User) {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	user.LastLogin = &now
	user.LoginCount++
	user.UpdatedAt = now
}

func (am *AuthManager) updateAPIKeyUsage(apiKey *APIKey) {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	apiKey.LastUsed = &now
	apiKey.UsageCount++
}

func (am *AuthManager) rolesMatch(userRoles, keyRoles []Role) bool {
	for _, userRole := range userRoles {
		for _, keyRole := range keyRoles {
			if userRole == keyRole {
				return true
			}
		}
	}
	return false
}

func (am *AuthManager) cleanupUserSessions(userID string) {
	var userSessions []*Session
	for _, session := range am.sessions {
		if session.UserID == userID {
			userSessions = append(userSessions, session)
		}
	}

	// Remove oldest sessions if limit exceeded
	if len(userSessions) >= am.config.MaxSessionsPerUser {
		// Sort by creation time and remove oldest
		for i := 0; i < len(userSessions)-am.config.MaxSessionsPerUser+1; i++ {
			delete(am.sessions, userSessions[i].ID)
		}
	}
}

func (am *AuthManager) createDefaultAdmin() {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if admin user already exists
	for _, user := range am.users {
		for _, role := range user.Roles {
			if role == RoleAdmin {
				return // Admin exists
			}
		}
	}

	// Create default admin
	defaultPassword := "admin123!@#" // Should be changed immediately
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)

	admin := &User{
		ID:           "admin_default",
		Username:     "admin",
		Email:        "admin@localhost",
		PasswordHash: string(passwordHash),
		Roles:        []Role{RoleAdmin},
		Enabled:      true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	am.users[admin.ID] = admin

	am.logger.Warn("default admin user created", map[string]interface{}{
		"username": admin.Username,
		"password": defaultPassword,
		"action":   "change_password_immediately",
	})
}

func (am *AuthManager) initializeDefaultRBAC() {
	// Define default role permissions
	am.config.RBAC.Roles = map[Role]RoleDefinition{
		RoleAdmin: {
			Name:        "Administrator",
			Description: "Full system access",
			Permissions: map[Resource][]Permission{
				ResourceSecrets:   {PermissionRead, PermissionWrite, PermissionDelete, PermissionManageSecrets},
				ResourceConfig:    {PermissionRead, PermissionWrite, PermissionManageConfig},
				ResourceUsers:     {PermissionRead, PermissionWrite, PermissionDelete, PermissionManageUsers},
				ResourceAuditLogs: {PermissionRead, PermissionViewAuditLogs},
				ResourcePipelines: {PermissionRead, PermissionWrite, PermissionExecute, PermissionTriggerPipeline},
				ResourceMetrics:   {PermissionRead, PermissionViewMetrics},
				ResourceWebhooks:  {PermissionRead, PermissionWrite, PermissionWebhookAccess},
				ResourceBackups:   {PermissionRead, PermissionWrite, PermissionBackupAccess},
				ResourceGitOps:    {PermissionRead, PermissionWrite, PermissionGitOpsAccess},
				ResourceSystem:    {PermissionRead, PermissionWrite, PermissionExecute},
			},
		},
		RoleOperator: {
			Name:        "Operator",
			Description: "Operations and pipeline management",
			Permissions: map[Resource][]Permission{
				ResourceSecrets:   {PermissionRead},
				ResourceConfig:    {PermissionRead},
				ResourcePipelines: {PermissionRead, PermissionWrite, PermissionExecute, PermissionTriggerPipeline},
				ResourceMetrics:   {PermissionRead, PermissionViewMetrics},
				ResourceBackups:   {PermissionRead, PermissionWrite, PermissionBackupAccess},
				ResourceGitOps:    {PermissionRead, PermissionWrite, PermissionGitOpsAccess},
			},
		},
		RoleReadOnly: {
			Name:        "Read Only",
			Description: "Read-only access to resources",
			Permissions: map[Resource][]Permission{
				ResourceSecrets:   {},
				ResourceConfig:    {PermissionRead},
				ResourcePipelines: {PermissionRead},
				ResourceMetrics:   {PermissionRead, PermissionViewMetrics},
				ResourceBackups:   {PermissionRead},
				ResourceGitOps:    {PermissionRead},
			},
		},
		RoleWebhook: {
			Name:        "Webhook Service",
			Description: "Webhook endpoint access",
			Permissions: map[Resource][]Permission{
				ResourceWebhooks:  {PermissionWebhookAccess},
				ResourcePipelines: {PermissionTriggerPipeline},
			},
		},
		RoleService: {
			Name:        "Service Account",
			Description: "Automated service access",
			Permissions: map[Resource][]Permission{
				ResourcePipelines: {PermissionRead, PermissionExecute, PermissionTriggerPipeline},
				ResourceBackups:   {PermissionRead, PermissionWrite, PermissionBackupAccess},
				ResourceGitOps:    {PermissionRead, PermissionWrite, PermissionGitOpsAccess},
				ResourceMetrics:   {PermissionRead},
			},
		},
		RoleGuest: {
			Name:        "Guest",
			Description: "Minimal access",
			Permissions: map[Resource][]Permission{
				ResourceMetrics: {PermissionRead},
			},
		},
	}
}