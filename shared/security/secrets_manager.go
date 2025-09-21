package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// SecretProvider defines supported secret management backends
type SecretProvider string

const (
	ProviderEnvironment  SecretProvider = "env"
	ProviderVault        SecretProvider = "vault"
	ProviderAWSSecrets   SecretProvider = "aws-secrets"
	ProviderAzureKeyVault SecretProvider = "azure-keyvault"
	ProviderFile         SecretProvider = "file"
	ProviderEncrypted    SecretProvider = "encrypted"
)

// SecretType defines types of secrets for classification
type SecretType string

const (
	SecretTypeCredential SecretType = "credential"
	SecretTypeToken      SecretType = "token"
	SecretTypeKey        SecretType = "key"
	SecretTypeCertificate SecretType = "certificate"
	SecretTypePassword   SecretType = "password"
)

// Secret represents a managed secret with metadata
type Secret struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        SecretType        `json:"type"`
	Value       string            `json:"value,omitempty"`
	EncryptedValue []byte         `json:"encrypted_value,omitempty"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	RotationInterval *time.Duration `json:"rotation_interval,omitempty"`
	Tags        []string          `json:"tags"`
	Version     int               `json:"version"`
}

// SecretsManagerConfig configuration for secrets management
type SecretsManagerConfig struct {
	Provider        SecretProvider `yaml:"provider"`
	EncryptionKey   string         `yaml:"encryption_key"`
	KeyDerivation   KeyDerivationConfig `yaml:"key_derivation"`
	Storage         SecretStorageConfig `yaml:"storage"`
	Rotation        RotationConfig      `yaml:"rotation"`
	Scanning        ScanningConfig      `yaml:"scanning"`
	Audit           AuditConfig         `yaml:"audit"`
	
	// Provider-specific configurations
	Vault      VaultConfig      `yaml:"vault"`
	AWS        AWSSecretsConfig `yaml:"aws_secrets"`
	Azure      AzureKeyVaultConfig `yaml:"azure_keyvault"`
}

// KeyDerivationConfig configuration for key derivation
type KeyDerivationConfig struct {
	Algorithm   string `yaml:"algorithm"`   // "pbkdf2", "scrypt", "argon2"
	Iterations  int    `yaml:"iterations"`  // PBKDF2 iterations
	SaltSize    int    `yaml:"salt_size"`   // Salt size in bytes
	KeySize     int    `yaml:"key_size"`    // Derived key size
}

// SecretStorageConfig configuration for secret storage
type SecretStorageConfig struct {
	Location    string        `yaml:"location"`
	Backup      bool          `yaml:"backup"`
	BackupPath  string        `yaml:"backup_path"`
	Compression bool          `yaml:"compression"`
	Integrity   bool          `yaml:"integrity"`
	TTL         time.Duration `yaml:"ttl"`
}

// RotationConfig configuration for secret rotation
type RotationConfig struct {
	Enabled         bool          `yaml:"enabled"`
	CheckInterval   time.Duration `yaml:"check_interval"`
	DefaultInterval time.Duration `yaml:"default_interval"`
	MaxAge          time.Duration `yaml:"max_age"`
	NotifyBefore    time.Duration `yaml:"notify_before"`
}

// ScanningConfig configuration for secret scanning
type ScanningConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Patterns    []string `yaml:"patterns"`
	Exclusions  []string `yaml:"exclusions"`
	ReportPath  string   `yaml:"report_path"`
	FailOnFound bool     `yaml:"fail_on_found"`
}

// AuditConfig configuration for audit logging
type AuditConfig struct {
	Enabled    bool   `yaml:"enabled"`
	LogPath    string `yaml:"log_path"`
	LogLevel   string `yaml:"log_level"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

// Provider-specific configurations
type VaultConfig struct {
	Address     string `yaml:"address"`
	Token       string `yaml:"token"`
	Path        string `yaml:"path"`
	Namespace   string `yaml:"namespace"`
	TLS         VaultTLSConfig `yaml:"tls"`
}

type VaultTLSConfig struct {
	CACert     string `yaml:"ca_cert"`
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
	Insecure   bool   `yaml:"insecure"`
}

type AWSSecretsConfig struct {
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	SessionToken    string `yaml:"session_token"`
	Endpoint        string `yaml:"endpoint"`
}

type AzureKeyVaultConfig struct {
	VaultName    string `yaml:"vault_name"`
	TenantID     string `yaml:"tenant_id"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Endpoint     string `yaml:"endpoint"`
}

// SecretsManager provides secure secret management capabilities
type SecretsManager struct {
	config   *SecretsManagerConfig
	logger   Logger
	cipher   cipher.AEAD
	scanner  *SecretScanner
	auditor  *AuditLogger
	mu       sync.RWMutex
	secrets  map[string]*Secret
}

// Logger interface for security logging
type Logger interface {
	Info(message string, fields map[string]interface{})
	Error(message string, fields map[string]interface{})
	Warn(message string, fields map[string]interface{})
	Debug(message string, fields map[string]interface{})
}

// DefaultSecretsManagerConfig returns default configuration
func DefaultSecretsManagerConfig() *SecretsManagerConfig {
	return &SecretsManagerConfig{
		Provider: ProviderEncrypted,
		KeyDerivation: KeyDerivationConfig{
			Algorithm:  "pbkdf2",
			Iterations: 100000,
			SaltSize:   32,
			KeySize:    32,
		},
		Storage: SecretStorageConfig{
			Location:    "/etc/backup-gitops/secrets",
			Backup:      true,
			BackupPath:  "/etc/backup-gitops/secrets.bak",
			Compression: true,
			Integrity:   true,
			TTL:         24 * time.Hour,
		},
		Rotation: RotationConfig{
			Enabled:         true,
			CheckInterval:   1 * time.Hour,
			DefaultInterval: 30 * 24 * time.Hour, // 30 days
			MaxAge:          90 * 24 * time.Hour, // 90 days
			NotifyBefore:    7 * 24 * time.Hour,  // 7 days
		},
		Scanning: ScanningConfig{
			Enabled:     true,
			Patterns:    DefaultSecretPatterns(),
			Exclusions:  []string{"*.test", "*.example"},
			ReportPath:  "/var/log/backup-gitops/secret-scan.log",
			FailOnFound: true,
		},
		Audit: AuditConfig{
			Enabled:    true,
			LogPath:    "/var/log/backup-gitops/secrets-audit.log",
			LogLevel:   "info",
			MaxSize:    100, // MB
			MaxBackups: 10,
			MaxAge:     30, // days
		},
	}
}

// NewSecretsManager creates a new secrets manager instance
func NewSecretsManager(config *SecretsManagerConfig, logger Logger) (*SecretsManager, error) {
	if config == nil {
		config = DefaultSecretsManagerConfig()
	}

	sm := &SecretsManager{
		config:  config,
		logger:  logger,
		secrets: make(map[string]*Secret),
	}

	// Initialize encryption
	if err := sm.initializeEncryption(); err != nil {
		return nil, fmt.Errorf("failed to initialize encryption: %v", err)
	}

	// Initialize secret scanner
	if config.Scanning.Enabled {
		sm.scanner = NewSecretScanner(config.Scanning, logger)
	}

	// Initialize audit logger
	if config.Audit.Enabled {
		sm.auditor = NewAuditLogger(config.Audit, logger)
	}

	// Load existing secrets
	if err := sm.loadSecrets(); err != nil {
		logger.Warn("failed to load existing secrets", map[string]interface{}{
			"error": err.Error(),
		})
	}

	logger.Info("secrets manager initialized", map[string]interface{}{
		"provider": string(config.Provider),
		"encryption": "enabled",
		"scanning": config.Scanning.Enabled,
		"audit": config.Audit.Enabled,
	})

	return sm, nil
}

// StoreSecret securely stores a secret
func (sm *SecretsManager) StoreSecret(ctx context.Context, secret *Secret) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Validate secret
	if err := sm.validateSecret(secret); err != nil {
		return fmt.Errorf("secret validation failed: %v", err)
	}

	// Generate ID if not provided
	if secret.ID == "" {
		secret.ID = sm.generateSecretID(secret.Name, secret.Type)
	}

	// Set timestamps
	now := time.Now()
	if secret.CreatedAt.IsZero() {
		secret.CreatedAt = now
	}
	secret.UpdatedAt = now
	secret.Version++

	// Encrypt the secret value
	if err := sm.encryptSecret(secret); err != nil {
		return fmt.Errorf("failed to encrypt secret: %v", err)
	}

	// Store the secret
	sm.secrets[secret.ID] = secret

	// Persist to storage
	if err := sm.persistSecrets(); err != nil {
		return fmt.Errorf("failed to persist secrets: %v", err)
	}

	// Audit log
	if sm.auditor != nil {
		sm.auditor.LogSecretAccess("store", secret.ID, secret.Name, "success", "")
	}

	sm.logger.Info("secret stored", map[string]interface{}{
		"secret_id": secret.ID,
		"name":      secret.Name,
		"type":      string(secret.Type),
		"version":   secret.Version,
	})

	return nil
}

// GetSecret retrieves and decrypts a secret
func (sm *SecretsManager) GetSecret(ctx context.Context, id string) (*Secret, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	secret, exists := sm.secrets[id]
	if !exists {
		if sm.auditor != nil {
			sm.auditor.LogSecretAccess("get", id, "", "failure", "secret not found")
		}
		return nil, fmt.Errorf("secret not found: %s", id)
	}

	// Check expiration
	if secret.ExpiresAt != nil && time.Now().After(*secret.ExpiresAt) {
		if sm.auditor != nil {
			sm.auditor.LogSecretAccess("get", id, secret.Name, "failure", "secret expired")
		}
		return nil, fmt.Errorf("secret expired: %s", id)
	}

	// Create a copy and decrypt
	secretCopy := *secret
	if err := sm.decryptSecret(&secretCopy); err != nil {
		if sm.auditor != nil {
			sm.auditor.LogSecretAccess("get", id, secret.Name, "failure", "decryption failed")
		}
		return nil, fmt.Errorf("failed to decrypt secret: %v", err)
	}

	// Audit log
	if sm.auditor != nil {
		sm.auditor.LogSecretAccess("get", id, secret.Name, "success", "")
	}

	return &secretCopy, nil
}

// RotateSecret rotates a secret by generating a new value
func (sm *SecretsManager) RotateSecret(ctx context.Context, id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	secret, exists := sm.secrets[id]
	if !exists {
		return fmt.Errorf("secret not found: %s", id)
	}

	// Generate new secret value based on type
	newValue, err := sm.generateSecretValue(secret.Type)
	if err != nil {
		return fmt.Errorf("failed to generate new secret value: %v", err)
	}

	// Update secret
	secret.Value = newValue
	secret.UpdatedAt = time.Now()
	secret.Version++

	// Set next rotation time
	if secret.RotationInterval != nil {
		nextRotation := time.Now().Add(*secret.RotationInterval)
		secret.ExpiresAt = &nextRotation
	}

	// Encrypt the new value
	if err := sm.encryptSecret(secret); err != nil {
		return fmt.Errorf("failed to encrypt rotated secret: %v", err)
	}

	// Persist changes
	if err := sm.persistSecrets(); err != nil {
		return fmt.Errorf("failed to persist rotated secret: %v", err)
	}

	// Audit log
	if sm.auditor != nil {
		sm.auditor.LogSecretAccess("rotate", id, secret.Name, "success", "")
	}

	sm.logger.Info("secret rotated", map[string]interface{}{
		"secret_id": id,
		"name":      secret.Name,
		"version":   secret.Version,
	})

	return nil
}

// ScanForSecrets scans text content for exposed secrets
func (sm *SecretsManager) ScanForSecrets(content string) (*SecretScanResult, error) {
	if sm.scanner == nil {
		return nil, fmt.Errorf("secret scanner not enabled")
	}

	return sm.scanner.ScanContent(content)
}

// ScanFile scans a file for exposed secrets
func (sm *SecretsManager) ScanFile(filePath string) (*SecretScanResult, error) {
	if sm.scanner == nil {
		return nil, fmt.Errorf("secret scanner not enabled")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	result, err := sm.scanner.ScanContent(string(content))
	if err != nil {
		return nil, err
	}

	result.FilePath = filePath
	return result, nil
}

// initializeEncryption sets up encryption for secrets
func (sm *SecretsManager) initializeEncryption() error {
	// Derive encryption key
	key, err := sm.deriveEncryptionKey()
	if err != nil {
		return fmt.Errorf("failed to derive encryption key: %v", err)
	}

	// Create AES-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %v", err)
	}

	sm.cipher = gcm
	return nil
}

// deriveEncryptionKey derives an encryption key from configuration
func (sm *SecretsManager) deriveEncryptionKey() ([]byte, error) {
	// Get master key from environment or configuration
	masterKey := os.Getenv("SECRETS_MASTER_KEY")
	if masterKey == "" {
		masterKey = sm.config.EncryptionKey
	}
	if masterKey == "" {
		return nil, fmt.Errorf("no encryption key provided")
	}

	// Generate salt (in production, this should be stored securely)
	salt := []byte("backup-gitops-secrets-salt-v1") // Fixed salt for demo

	// Derive key using PBKDF2
	key := pbkdf2.Key(
		[]byte(masterKey),
		salt,
		sm.config.KeyDerivation.Iterations,
		sm.config.KeyDerivation.KeySize,
		sha256.New,
	)

	return key, nil
}

// encryptSecret encrypts a secret's value
func (sm *SecretsManager) encryptSecret(secret *Secret) error {
	if secret.Value == "" {
		return nil
	}

	// Generate nonce
	nonce := make([]byte, sm.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %v", err)
	}

	// Encrypt the value
	ciphertext := sm.cipher.Seal(nonce, nonce, []byte(secret.Value), nil)
	secret.EncryptedValue = ciphertext

	// Clear plaintext value
	secret.Value = ""

	return nil
}

// decryptSecret decrypts a secret's value
func (sm *SecretsManager) decryptSecret(secret *Secret) error {
	if len(secret.EncryptedValue) == 0 {
		return nil
	}

	// Extract nonce
	nonceSize := sm.cipher.NonceSize()
	if len(secret.EncryptedValue) < nonceSize {
		return fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := secret.EncryptedValue[:nonceSize], secret.EncryptedValue[nonceSize:]

	// Decrypt
	plaintext, err := sm.cipher.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt: %v", err)
	}

	secret.Value = string(plaintext)
	return nil
}

// validateSecret validates a secret before storage
func (sm *SecretsManager) validateSecret(secret *Secret) error {
	if secret.Name == "" {
		return fmt.Errorf("secret name is required")
	}

	if secret.Value == "" {
		return fmt.Errorf("secret value is required")
	}

	if secret.Type == "" {
		secret.Type = SecretTypeCredential
	}

	// Validate secret value based on type
	switch secret.Type {
	case SecretTypeToken:
		if len(secret.Value) < 16 {
			return fmt.Errorf("token must be at least 16 characters")
		}
	case SecretTypePassword:
		if len(secret.Value) < 8 {
			return fmt.Errorf("password must be at least 8 characters")
		}
	case SecretTypeKey:
		// Validate key format (base64, hex, etc.)
		if !isValidKey(secret.Value) {
			return fmt.Errorf("invalid key format")
		}
	}

	return nil
}

// generateSecretID generates a unique ID for a secret
func (sm *SecretsManager) generateSecretID(name string, secretType SecretType) string {
	// Create deterministic ID based on name and type
	hash := sha256.Sum256([]byte(name + string(secretType) + time.Now().String()))
	return base64.URLEncoding.EncodeToString(hash[:16])
}

// generateSecretValue generates a new secret value based on type
func (sm *SecretsManager) generateSecretValue(secretType SecretType) (string, error) {
	switch secretType {
	case SecretTypeToken:
		return generateRandomToken(32)
	case SecretTypePassword:
		return generateRandomPassword(16)
	case SecretTypeKey:
		return generateRandomKey(32)
	default:
		return generateRandomToken(24)
	}
}

// loadSecrets loads secrets from persistent storage
func (sm *SecretsManager) loadSecrets() error {
	storageFile := sm.config.Storage.Location
	if _, err := os.Stat(storageFile); os.IsNotExist(err) {
		return nil // No existing secrets file
	}

	data, err := os.ReadFile(storageFile)
	if err != nil {
		return fmt.Errorf("failed to read secrets file: %v", err)
	}

	var secrets map[string]*Secret
	if err := json.Unmarshal(data, &secrets); err != nil {
		return fmt.Errorf("failed to unmarshal secrets: %v", err)
	}

	sm.secrets = secrets
	return nil
}

// persistSecrets saves secrets to persistent storage
func (sm *SecretsManager) persistSecrets() error {
	// Create storage directory
	storageDir := filepath.Dir(sm.config.Storage.Location)
	if err := os.MkdirAll(storageDir, 0700); err != nil {
		return fmt.Errorf("failed to create storage directory: %v", err)
	}

	// Backup existing file if enabled
	if sm.config.Storage.Backup {
		if _, err := os.Stat(sm.config.Storage.Location); err == nil {
			if err := sm.backupSecretsFile(); err != nil {
				sm.logger.Warn("failed to backup secrets file", map[string]interface{}{
					"error": err.Error(),
				})
			}
		}
	}

	// Marshal secrets
	data, err := json.MarshalIndent(sm.secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal secrets: %v", err)
	}

	// Write to file with restricted permissions
	if err := os.WriteFile(sm.config.Storage.Location, data, 0600); err != nil {
		return fmt.Errorf("failed to write secrets file: %v", err)
	}

	return nil
}

// backupSecretsFile creates a backup of the secrets file
func (sm *SecretsManager) backupSecretsFile() error {
	source := sm.config.Storage.Location
	dest := sm.config.Storage.BackupPath

	sourceData, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	return os.WriteFile(dest, sourceData, 0600)
}

// Helper functions

func isValidKey(key string) bool {
	// Check if it's valid base64
	if _, err := base64.StdEncoding.DecodeString(key); err == nil {
		return true
	}
	// Check if it's valid hex
	if matched, _ := regexp.MatchString("^[0-9a-fA-F]+$", key); matched {
		return true
	}
	return false
}

func generateRandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i, b := range bytes {
		bytes[i] = charset[b%byte(len(charset))]
	}
	return string(bytes), nil
}

func generateRandomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// DefaultSecretPatterns returns common secret patterns to scan for
func DefaultSecretPatterns() []string {
	return []string{
		// AWS
		"(?i)aws[_-]?access[_-]?key[_-]?id[\"']?\\s*[:=]\\s*[\"']?([a-za-z0-9]{20})",
		"(?i)aws[_-]?secret[_-]?access[_-]?key[\"']?\\s*[:=]\\s*[\"']?([a-za-z0-9/+=]{40})",
		
		// Google API
		"(?i)google[_-]?api[_-]?key[\"']?\\s*[:=]\\s*[\"']?([a-za-z0-9-_]{39})",
		
		// GitHub
		"(?i)github[_-]?token[\"']?\\s*[:=]\\s*[\"']?([a-za-z0-9]{40})",
		
		// Generic tokens
		"(?i)token[\"']?\\s*[:=]\\s*[\"']?([a-za-z0-9-_]{32,})",
		"(?i)secret[\"']?\\s*[:=]\\s*[\"']?([a-za-z0-9-_]{16,})",
		"(?i)password[\"']?\\s*[:=]\\s*[\"']?([a-za-z0-9-_!@#$%^&*]{8,})",
		
		// Database connections
		"(?i)postgres://[a-za-z0-9_-]+:[a-za-z0-9_-]+@[a-za-z0-9.-]+",
		"(?i)mysql://[a-za-z0-9_-]+:[a-za-z0-9_-]+@[a-za-z0-9.-]+",
		
		// Private keys
		"-----BEGIN[A-Z ]+PRIVATE KEY-----",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
	}
}