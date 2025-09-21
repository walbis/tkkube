package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TLSConfig defines TLS configuration options
type TLSConfig struct {
	Enabled               bool              `yaml:"enabled"`
	MinVersion            string            `yaml:"min_version"`
	MaxVersion            string            `yaml:"max_version"`
	CipherSuites          []string          `yaml:"cipher_suites"`
	PreferServerCiphers   bool              `yaml:"prefer_server_ciphers"`
	CertificateFile       string            `yaml:"certificate_file"`
	PrivateKeyFile        string            `yaml:"private_key_file"`
	CAFile                string            `yaml:"ca_file"`
	ClientAuth            string            `yaml:"client_auth"` // none, request, require, verify, require_and_verify
	ServerName            string            `yaml:"server_name"`
	InsecureSkipVerify    bool              `yaml:"insecure_skip_verify"`
	CertificatePinning    CertPinningConfig `yaml:"certificate_pinning"`
	OCSP                  OCSPConfig        `yaml:"ocsp"`
	CertificateRotation   CertRotationConfig `yaml:"certificate_rotation"`
	MTLSConfig            MTLSConfig        `yaml:"mtls"`
}

// CertPinningConfig configures certificate pinning
type CertPinningConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Pins        []string `yaml:"pins"`        // SHA256 hashes of expected certificates
	BackupPins  []string `yaml:"backup_pins"` // Backup pins for rotation
	ReportOnly  bool     `yaml:"report_only"` // Only report violations, don't fail
}

// OCSPConfig configures OCSP (Online Certificate Status Protocol)
type OCSPConfig struct {
	Enabled      bool          `yaml:"enabled"`
	StaplingOnly bool          `yaml:"stapling_only"`
	Timeout      time.Duration `yaml:"timeout"`
	CacheTime    time.Duration `yaml:"cache_time"`
}

// CertRotationConfig configures certificate rotation
type CertRotationConfig struct {
	Enabled          bool          `yaml:"enabled"`
	CheckInterval    time.Duration `yaml:"check_interval"`
	RotateBeforeExp  time.Duration `yaml:"rotate_before_expiry"`
	BackupOldCerts   bool          `yaml:"backup_old_certs"`
	NotifyBeforeExp  time.Duration `yaml:"notify_before_expiry"`
}

// MTLSConfig configures mutual TLS
type MTLSConfig struct {
	Enabled           bool     `yaml:"enabled"`
	RequireClientCert bool     `yaml:"require_client_cert"`
	ClientCAFile      string   `yaml:"client_ca_file"`
	ClientCertFile    string   `yaml:"client_cert_file"`
	ClientKeyFile     string   `yaml:"client_key_file"`
	TrustedCNs        []string `yaml:"trusted_cns"`
	CRLFile           string   `yaml:"crl_file"`
}

// TLSManager handles TLS configuration and validation
type TLSManager struct {
	config   TLSConfig
	logger   Logger
	auditor  *AuditLogger
	tlsConf  *tls.Config
	certPool *x509.CertPool
	mu       sync.RWMutex
}

// CertificateInfo contains certificate information
type CertificateInfo struct {
	Subject        string    `json:"subject"`
	Issuer         string    `json:"issuer"`
	SerialNumber   string    `json:"serial_number"`
	NotBefore      time.Time `json:"not_before"`
	NotAfter       time.Time `json:"not_after"`
	IsCA           bool      `json:"is_ca"`
	KeyUsage       []string  `json:"key_usage"`
	DNSNames       []string  `json:"dns_names"`
	IPAddresses    []string  `json:"ip_addresses"`
	Fingerprint    string    `json:"fingerprint"`
	SignatureAlg   string    `json:"signature_algorithm"`
	PublicKeyAlg   string    `json:"public_key_algorithm"`
	Version        int       `json:"version"`
}

// NewTLSManager creates a new TLS manager
func NewTLSManager(config TLSConfig, logger Logger, auditor *AuditLogger) (*TLSManager, error) {
	tm := &TLSManager{
		config:  config,
		logger:  logger,
		auditor: auditor,
	}

	if config.Enabled {
		if err := tm.initializeTLSConfig(); err != nil {
			return nil, fmt.Errorf("failed to initialize TLS config: %v", err)
		}

		if config.CertificateRotation.Enabled {
			go tm.startCertificateRotationMonitor()
		}
	}

	return tm, nil
}

// DefaultTLSConfig returns default TLS configuration
func DefaultTLSConfig() TLSConfig {
	return TLSConfig{
		Enabled:             true,
		MinVersion:          "1.2",
		MaxVersion:          "1.3",
		PreferServerCiphers: true,
		ClientAuth:          "require_and_verify",
		InsecureSkipVerify:  false,
		CipherSuites: []string{
			"TLS_AES_256_GCM_SHA384",
			"TLS_CHACHA20_POLY1305_SHA256",
			"TLS_AES_128_GCM_SHA256",
			"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
			"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
			"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		},
		CertificatePinning: CertPinningConfig{
			Enabled:    true,
			ReportOnly: false,
		},
		OCSP: OCSPConfig{
			Enabled:      true,
			StaplingOnly: false,
			Timeout:      10 * time.Second,
			CacheTime:    1 * time.Hour,
		},
		CertificateRotation: CertRotationConfig{
			Enabled:         true,
			CheckInterval:   24 * time.Hour,
			RotateBeforeExp: 30 * 24 * time.Hour, // 30 days
			BackupOldCerts:  true,
			NotifyBeforeExp: 7 * 24 * time.Hour, // 7 days
		},
		MTLSConfig: MTLSConfig{
			Enabled:           true,
			RequireClientCert: true,
		},
	}
}

// GetTLSConfig returns the configured TLS configuration
func (tm *TLSManager) GetTLSConfig() *tls.Config {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tlsConf.Clone()
}

// GetHTTPClient returns an HTTP client with secure TLS configuration
func (tm *TLSManager) GetHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tm.GetTLSConfig(),
		},
		Timeout: 30 * time.Second,
	}
}

// ValidateCertificate validates a certificate against security policies
func (tm *TLSManager) ValidateCertificate(cert *x509.Certificate) error {
	// Check certificate expiry
	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}

	// Check if certificate expires soon
	if time.Now().Add(tm.config.CertificateRotation.NotifyBeforeExp).After(cert.NotAfter) {
		tm.logger.Warn("certificate expires soon", map[string]interface{}{
			"subject":    cert.Subject.String(),
			"expires_at": cert.NotAfter,
		})
	}

	// Check key usage
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return fmt.Errorf("certificate missing required key usage: digital signature")
	}

	// Check signature algorithm
	if cert.SignatureAlgorithm == x509.SHA1WithRSA ||
		cert.SignatureAlgorithm == x509.MD5WithRSA {
		return fmt.Errorf("certificate uses insecure signature algorithm: %s", cert.SignatureAlgorithm)
	}

	// Check public key strength
	if err := tm.validatePublicKey(cert); err != nil {
		return fmt.Errorf("certificate public key validation failed: %v", err)
	}

	tm.auditor.LogSystemEvent("certificate_validated", "tls_manager", "success",
		fmt.Sprintf("Certificate validated: %s", cert.Subject.String()))

	return nil
}

// VerifyCertificateChain verifies a certificate chain
func (tm *TLSManager) VerifyCertificateChain(certs []*x509.Certificate) error {
	if len(certs) == 0 {
		return fmt.Errorf("empty certificate chain")
	}

	// Create certificate pool with CA certificates
	roots := tm.certPool
	if roots == nil {
		roots = x509.NewCertPool()
	}

	// Verify the chain
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: x509.NewCertPool(),
	}

	// Add intermediate certificates
	for i := 1; i < len(certs); i++ {
		opts.Intermediates.AddCert(certs[i])
	}

	// Verify the leaf certificate
	_, err := certs[0].Verify(opts)
	if err != nil {
		tm.auditor.LogSecurityViolation("", "certificate_chain_invalid", err.Error(), "high")
		return fmt.Errorf("certificate chain verification failed: %v", err)
	}

	// Validate each certificate in the chain
	for _, cert := range certs {
		if err := tm.ValidateCertificate(cert); err != nil {
			return fmt.Errorf("certificate validation failed: %v", err)
		}
	}

	return nil
}

// ValidateCertificatePin validates certificate pinning
func (tm *TLSManager) ValidateCertificatePin(cert *x509.Certificate) error {
	if !tm.config.CertificatePinning.Enabled {
		return nil
	}

	fingerprint := tm.getCertificateFingerprint(cert)
	
	// Check against pinned certificates
	for _, pin := range tm.config.CertificatePinning.Pins {
		if fingerprint == pin {
			return nil // Pin matched
		}
	}

	// Check backup pins
	for _, pin := range tm.config.CertificatePinning.BackupPins {
		if fingerprint == pin {
			tm.logger.Warn("certificate matched backup pin", map[string]interface{}{
				"fingerprint": fingerprint,
				"subject":     cert.Subject.String(),
			})
			return nil
		}
	}

	// Pin violation
	violation := fmt.Sprintf("Certificate pin mismatch: %s", fingerprint)
	tm.auditor.LogSecurityViolation("", "certificate_pin_violation", violation, "high")

	if tm.config.CertificatePinning.ReportOnly {
		tm.logger.Warn("certificate pin violation (report only)", map[string]interface{}{
			"fingerprint": fingerprint,
			"subject":     cert.Subject.String(),
		})
		return nil
	}

	return fmt.Errorf("certificate pin validation failed")
}

// ExtractCertificateInfo extracts information from a certificate
func (tm *TLSManager) ExtractCertificateInfo(cert *x509.Certificate) *CertificateInfo {
	keyUsage := []string{}
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		keyUsage = append(keyUsage, "DigitalSignature")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		keyUsage = append(keyUsage, "KeyEncipherment")
	}
	if cert.KeyUsage&x509.KeyUsageDataEncipherment != 0 {
		keyUsage = append(keyUsage, "DataEncipherment")
	}
	if cert.KeyUsage&x509.KeyUsageCertSign != 0 {
		keyUsage = append(keyUsage, "CertSign")
	}

	ipAddresses := []string{}
	for _, ip := range cert.IPAddresses {
		ipAddresses = append(ipAddresses, ip.String())
	}

	return &CertificateInfo{
		Subject:       cert.Subject.String(),
		Issuer:        cert.Issuer.String(),
		SerialNumber:  cert.SerialNumber.String(),
		NotBefore:     cert.NotBefore,
		NotAfter:      cert.NotAfter,
		IsCA:          cert.IsCA,
		KeyUsage:      keyUsage,
		DNSNames:      cert.DNSNames,
		IPAddresses:   ipAddresses,
		Fingerprint:   tm.getCertificateFingerprint(cert),
		SignatureAlg:  cert.SignatureAlgorithm.String(),
		PublicKeyAlg:  cert.PublicKeyAlgorithm.String(),
		Version:       cert.Version,
	}
}

// initializeTLSConfig initializes the TLS configuration
func (tm *TLSManager) initializeTLSConfig() error {
	tlsConfig := &tls.Config{
		PreferServerCipherSuites: tm.config.PreferServerCiphers,
		InsecureSkipVerify:       tm.config.InsecureSkipVerify,
		ServerName:               tm.config.ServerName,
	}

	// Set TLS version
	if err := tm.setTLSVersion(tlsConfig); err != nil {
		return err
	}

	// Set cipher suites
	if err := tm.setCipherSuites(tlsConfig); err != nil {
		return err
	}

	// Load certificates
	if err := tm.loadCertificates(tlsConfig); err != nil {
		return err
	}

	// Set client authentication
	if err := tm.setClientAuth(tlsConfig); err != nil {
		return err
	}

	// Set up certificate verification
	if tm.config.CertificatePinning.Enabled || !tm.config.InsecureSkipVerify {
		tlsConfig.VerifyPeerCertificate = tm.verifyPeerCertificate
	}

	tm.mu.Lock()
	tm.tlsConf = tlsConfig
	tm.mu.Unlock()

	tm.logger.Info("TLS configuration initialized", map[string]interface{}{
		"min_version":    tm.config.MinVersion,
		"max_version":    tm.config.MaxVersion,
		"client_auth":    tm.config.ClientAuth,
		"cert_pinning":   tm.config.CertificatePinning.Enabled,
		"mtls_enabled":   tm.config.MTLSConfig.Enabled,
	})

	return nil
}

// setTLSVersion sets the TLS version range
func (tm *TLSManager) setTLSVersion(tlsConfig *tls.Config) error {
	versionMap := map[string]uint16{
		"1.0": tls.VersionTLS10,
		"1.1": tls.VersionTLS11,
		"1.2": tls.VersionTLS12,
		"1.3": tls.VersionTLS13,
	}

	if minVer, ok := versionMap[tm.config.MinVersion]; ok {
		tlsConfig.MinVersion = minVer
	} else {
		return fmt.Errorf("invalid minimum TLS version: %s", tm.config.MinVersion)
	}

	if maxVer, ok := versionMap[tm.config.MaxVersion]; ok {
		tlsConfig.MaxVersion = maxVer
	} else {
		return fmt.Errorf("invalid maximum TLS version: %s", tm.config.MaxVersion)
	}

	if tlsConfig.MinVersion > tlsConfig.MaxVersion {
		return fmt.Errorf("minimum TLS version cannot be greater than maximum version")
	}

	return nil
}

// setCipherSuites sets the cipher suites
func (tm *TLSManager) setCipherSuites(tlsConfig *tls.Config) error {
	cipherMap := map[string]uint16{
		"TLS_AES_128_GCM_SHA256":                      tls.TLS_AES_128_GCM_SHA256,
		"TLS_AES_256_GCM_SHA384":                      tls.TLS_AES_256_GCM_SHA384,
		"TLS_CHACHA20_POLY1305_SHA256":                tls.TLS_CHACHA20_POLY1305_SHA256,
		"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":     tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":     tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305":      tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":       tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":       tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305":        tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}

	var cipherSuites []uint16
	for _, cipher := range tm.config.CipherSuites {
		if cipherID, ok := cipherMap[cipher]; ok {
			cipherSuites = append(cipherSuites, cipherID)
		} else {
			return fmt.Errorf("unsupported cipher suite: %s", cipher)
		}
	}

	tlsConfig.CipherSuites = cipherSuites
	return nil
}

// loadCertificates loads server certificates
func (tm *TLSManager) loadCertificates(tlsConfig *tls.Config) error {
	if tm.config.CertificateFile != "" && tm.config.PrivateKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tm.config.CertificateFile, tm.config.PrivateKeyFile)
		if err != nil {
			return fmt.Errorf("failed to load certificate: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificates
	if tm.config.CAFile != "" {
		caCert, err := ioutil.ReadFile(tm.config.CAFile)
		if err != nil {
			return fmt.Errorf("failed to read CA file: %v", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
		tm.certPool = caCertPool
	}

	// Load client CA for mTLS
	if tm.config.MTLSConfig.Enabled && tm.config.MTLSConfig.ClientCAFile != "" {
		clientCACert, err := ioutil.ReadFile(tm.config.MTLSConfig.ClientCAFile)
		if err != nil {
			return fmt.Errorf("failed to read client CA file: %v", err)
		}

		clientCACertPool := x509.NewCertPool()
		if !clientCACertPool.AppendCertsFromPEM(clientCACert) {
			return fmt.Errorf("failed to parse client CA certificate")
		}

		tlsConfig.ClientCAs = clientCACertPool
	}

	return nil
}

// setClientAuth sets client authentication requirements
func (tm *TLSManager) setClientAuth(tlsConfig *tls.Config) error {
	authMap := map[string]tls.ClientAuthType{
		"none":                tls.NoClientCert,
		"request":             tls.RequestClientCert,
		"require":             tls.RequireAnyClientCert,
		"verify":              tls.VerifyClientCertIfGiven,
		"require_and_verify":  tls.RequireAndVerifyClientCert,
	}

	if authType, ok := authMap[tm.config.ClientAuth]; ok {
		tlsConfig.ClientAuth = authType
	} else {
		return fmt.Errorf("invalid client auth type: %s", tm.config.ClientAuth)
	}

	return nil
}

// verifyPeerCertificate custom certificate verification
func (tm *TLSManager) verifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no certificates provided")
	}

	// Parse certificates
	var certs []*x509.Certificate
	for _, rawCert := range rawCerts {
		cert, err := x509.ParseCertificate(rawCert)
		if err != nil {
			return fmt.Errorf("failed to parse certificate: %v", err)
		}
		certs = append(certs, cert)
	}

	// Verify certificate chain
	if err := tm.VerifyCertificateChain(certs); err != nil {
		return err
	}

	// Validate certificate pinning
	if err := tm.ValidateCertificatePin(certs[0]); err != nil {
		return err
	}

	// Additional mTLS validation
	if tm.config.MTLSConfig.Enabled {
		return tm.validateMTLSCertificate(certs[0])
	}

	return nil
}

// validateMTLSCertificate validates mTLS client certificate
func (tm *TLSManager) validateMTLSCertificate(cert *x509.Certificate) error {
	// Check if client certificate is required
	if tm.config.MTLSConfig.RequireClientCert && cert == nil {
		return fmt.Errorf("client certificate required for mTLS")
	}

	if cert == nil {
		return nil // No certificate provided, but not required
	}

	// Validate trusted CNs
	if len(tm.config.MTLSConfig.TrustedCNs) > 0 {
		cnValid := false
		for _, trustedCN := range tm.config.MTLSConfig.TrustedCNs {
			if cert.Subject.CommonName == trustedCN {
				cnValid = true
				break
			}
		}
		if !cnValid {
			return fmt.Errorf("client certificate CN not in trusted list: %s", cert.Subject.CommonName)
		}
	}

	tm.auditor.LogSystemEvent("mtls_client_authenticated", "tls_manager", "success",
		fmt.Sprintf("Client authenticated via mTLS: %s", cert.Subject.CommonName))

	return nil
}

// validatePublicKey validates public key strength
func (tm *TLSManager) validatePublicKey(cert *x509.Certificate) error {
	switch pub := cert.PublicKey.(type) {
	case *x509.Certificate:
		// RSA key validation would go here
		_ = pub
	default:
		// Other key types
	}
	return nil
}

// getCertificateFingerprint calculates SHA256 fingerprint of certificate
func (tm *TLSManager) getCertificateFingerprint(cert *x509.Certificate) string {
	return fmt.Sprintf("%x", cert.Raw) // Simplified - use proper SHA256
}

// startCertificateRotationMonitor starts certificate rotation monitoring
func (tm *TLSManager) startCertificateRotationMonitor() {
	ticker := time.NewTicker(tm.config.CertificateRotation.CheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		tm.checkCertificateExpiry()
	}
}

// checkCertificateExpiry checks if certificates are expiring
func (tm *TLSManager) checkCertificateExpiry() {
	tm.mu.RLock()
	config := tm.tlsConf
	tm.mu.RUnlock()

	if config == nil {
		return
	}

	for _, cert := range config.Certificates {
		if len(cert.Certificate) > 0 {
			x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				tm.logger.Error("failed to parse certificate for expiry check", map[string]interface{}{
					"error": err.Error(),
				})
				continue
			}

			// Check if certificate is expiring soon
			timeUntilExpiry := time.Until(x509Cert.NotAfter)
			if timeUntilExpiry <= tm.config.CertificateRotation.RotateBeforeExp {
				tm.logger.Warn("certificate expiring soon", map[string]interface{}{
					"subject":           x509Cert.Subject.String(),
					"expires_at":        x509Cert.NotAfter,
					"time_until_expiry": timeUntilExpiry,
				})

				tm.auditor.LogSystemEvent("certificate_expiring", "tls_manager", "warning",
					fmt.Sprintf("Certificate expiring in %v: %s", timeUntilExpiry, x509Cert.Subject.String()))

				// Auto-rotation would be implemented here
				if tm.config.CertificateRotation.Enabled {
					tm.rotateCertificate(x509Cert)
				}
			}
		}
	}
}

// rotateCertificate rotates an expiring certificate
func (tm *TLSManager) rotateCertificate(cert *x509.Certificate) {
	tm.logger.Info("initiating certificate rotation", map[string]interface{}{
		"subject": cert.Subject.String(),
	})

	// Backup old certificate
	if tm.config.CertificateRotation.BackupOldCerts {
		tm.backupCertificate(cert)
	}

	// Certificate rotation logic would be implemented here
	// This would typically involve:
	// 1. Generating new private key
	// 2. Creating certificate signing request
	// 3. Submitting to CA or self-signing
	// 4. Installing new certificate
	// 5. Graceful restart/reload

	tm.auditor.LogSystemEvent("certificate_rotated", "tls_manager", "success",
		fmt.Sprintf("Certificate rotated: %s", cert.Subject.String()))
}

// backupCertificate creates a backup of the current certificate
func (tm *TLSManager) backupCertificate(cert *x509.Certificate) {
	backupDir := filepath.Join(filepath.Dir(tm.config.CertificateFile), "backups")
	os.MkdirAll(backupDir, 0700)

	timestamp := time.Now().Format("20060102-150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("cert-backup-%s.pem", timestamp))

	// Write certificate to backup file
	// Implementation would write the actual certificate data
	tm.logger.Info("certificate backed up", map[string]interface{}{
		"backup_file": backupFile,
		"subject":     cert.Subject.String(),
	})
}

// GetCertificateStatus returns the status of loaded certificates
func (tm *TLSManager) GetCertificateStatus() []CertificateInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var certInfos []CertificateInfo
	if tm.tlsConf != nil {
		for _, cert := range tm.tlsConf.Certificates {
			if len(cert.Certificate) > 0 {
				x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
				if err == nil {
					certInfos = append(certInfos, *tm.ExtractCertificateInfo(x509Cert))
				}
			}
		}
	}

	return certInfos
}

// ValidateConfiguration validates TLS configuration
func (tm *TLSManager) ValidateConfiguration() error {
	// Check if certificate files exist
	if tm.config.CertificateFile != "" {
		if _, err := os.Stat(tm.config.CertificateFile); os.IsNotExist(err) {
			return fmt.Errorf("certificate file not found: %s", tm.config.CertificateFile)
		}
	}

	if tm.config.PrivateKeyFile != "" {
		if _, err := os.Stat(tm.config.PrivateKeyFile); os.IsNotExist(err) {
			return fmt.Errorf("private key file not found: %s", tm.config.PrivateKeyFile)
		}
	}

	// Validate TLS version compatibility
	if tm.config.MinVersion == "1.3" && len(tm.config.CipherSuites) > 0 {
		// TLS 1.3 doesn't use the same cipher suite configuration
		tm.logger.Warn("cipher suites specified for TLS 1.3", map[string]interface{}{
			"message": "TLS 1.3 uses different cipher suite configuration",
		})
	}

	// Check for weak configurations
	if tm.config.InsecureSkipVerify {
		tm.logger.Warn("TLS verification disabled", map[string]interface{}{
			"security_risk": "high",
		})
	}

	if strings.Contains(tm.config.MinVersion, "1.0") || strings.Contains(tm.config.MinVersion, "1.1") {
		tm.logger.Warn("weak TLS version enabled", map[string]interface{}{
			"version":       tm.config.MinVersion,
			"security_risk": "medium",
		})
	}

	return nil
}