package sharedconfig

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

// ClusterAuthManager handles authentication for Kubernetes clusters
type ClusterAuthManager struct{}

// NewClusterAuthManager creates a new cluster authentication manager
func NewClusterAuthManager() *ClusterAuthManager {
	return &ClusterAuthManager{}
}

// CreateRESTConfig creates a Kubernetes REST config from cluster configuration
func (cam *ClusterAuthManager) CreateRESTConfig(cluster *MultiClusterClusterConfig) (*rest.Config, error) {
	config := &rest.Config{
		Host: cluster.Endpoint,
	}

	// Apply authentication configuration
	if err := cam.applyAuthentication(config, &cluster.Auth, cluster); err != nil {
		return nil, fmt.Errorf("failed to configure authentication: %w", err)
	}

	// Apply TLS configuration
	if err := cam.applyTLSConfig(config, &cluster.TLS); err != nil {
		return nil, fmt.Errorf("failed to configure TLS: %w", err)
	}

	// Set reasonable timeouts
	config.Timeout = 30 * time.Second

	return config, nil
}

// applyAuthentication configures authentication for the REST config
func (cam *ClusterAuthManager) applyAuthentication(config *rest.Config, auth *ClusterAuthConfig, cluster *MultiClusterClusterConfig) error {
	// Handle legacy token configuration for backward compatibility
	if cluster.Token != "" && auth.Method == "" {
		config.BearerToken = cluster.Token
		return nil
	}

	switch auth.Method {
	case "token", "":
		return cam.configureTokenAuth(config, &auth.Token)
	case "service_account":
		return cam.configureServiceAccountAuth(config, &auth.ServiceAccount)
	case "oidc":
		return cam.configureOIDCAuth(config, &auth.OIDC)
	case "exec":
		return cam.configureExecAuth(config, &auth.Exec)
	default:
		return fmt.Errorf("unsupported authentication method: %s", auth.Method)
	}
}

// configureTokenAuth configures token-based authentication
func (cam *ClusterAuthManager) configureTokenAuth(config *rest.Config, tokenConfig *TokenAuthConfig) error {
	if tokenConfig.Value == "" {
		return fmt.Errorf("token value is required for token authentication")
	}

	switch tokenConfig.Type {
	case "bearer", "":
		config.BearerToken = tokenConfig.Value
	case "service_account":
		// Service account tokens can be used directly as bearer tokens
		config.BearerToken = tokenConfig.Value
	default:
		return fmt.Errorf("unsupported token type: %s", tokenConfig.Type)
	}

	return nil
}

// configureServiceAccountAuth configures service account authentication
func (cam *ClusterAuthManager) configureServiceAccountAuth(config *rest.Config, saConfig *ServiceAccountConfig) error {
	// Read token from file
	tokenBytes, err := ioutil.ReadFile(saConfig.TokenPath)
	if err != nil {
		return fmt.Errorf("failed to read service account token from %s: %w", saConfig.TokenPath, err)
	}

	config.BearerToken = string(tokenBytes)

	// The CA cert will be handled by TLS configuration
	return nil
}

// configureOIDCAuth configures OIDC authentication
func (cam *ClusterAuthManager) configureOIDCAuth(config *rest.Config, oidcConfig *OIDCConfig) error {
	if oidcConfig.IDToken == "" {
		return fmt.Errorf("ID token is required for OIDC authentication")
	}

	// Use the ID token as bearer token for API requests
	config.BearerToken = oidcConfig.IDToken

	// TODO: Implement OIDC token refresh logic
	// This would involve using the refresh token to get new ID tokens
	
	return nil
}

// configureExecAuth configures exec-based authentication
func (cam *ClusterAuthManager) configureExecAuth(config *rest.Config, execConfig *ExecConfig) error {
	if execConfig.Command == "" {
		return fmt.Errorf("command is required for exec authentication")
	}

	// Execute the command to get the token
	token, err := cam.executeTokenCommand(execConfig)
	if err != nil {
		return fmt.Errorf("failed to execute token command: %w", err)
	}

	config.BearerToken = token
	return nil
}

// executeTokenCommand executes the configured command to retrieve an authentication token
func (cam *ClusterAuthManager) executeTokenCommand(execConfig *ExecConfig) (string, error) {
	cmd := exec.Command(execConfig.Command, execConfig.Args...)
	
	// Set environment variables
	if len(execConfig.Env) > 0 {
		cmd.Env = append(os.Environ(), execConfig.Env...)
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w", err)
	}

	// Assume the command outputs the token directly
	// More sophisticated commands might output JSON that needs parsing
	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("command returned empty token")
	}

	return token, nil
}

// applyTLSConfig configures TLS settings for the REST config
func (cam *ClusterAuthManager) applyTLSConfig(config *rest.Config, tlsConfig *ClusterTLSConfig) error {
	if tlsConfig.Insecure {
		config.TLSClientConfig.Insecure = true
		return nil
	}

	// Configure CA certificate
	if err := cam.configureCAData(config, tlsConfig); err != nil {
		return fmt.Errorf("failed to configure CA data: %w", err)
	}

	// Configure client certificates
	if err := cam.configureClientCerts(config, tlsConfig); err != nil {
		return fmt.Errorf("failed to configure client certificates: %w", err)
	}

	// Set server name for SNI
	if tlsConfig.ServerName != "" {
		config.TLSClientConfig.ServerName = tlsConfig.ServerName
	}

	return nil
}

// configureCAData configures the certificate authority data
func (cam *ClusterAuthManager) configureCAData(config *rest.Config, tlsConfig *ClusterTLSConfig) error {
	var caData []byte
	var err error

	// Priority: CAData > CABundle > service account CA (for service account auth)
	if tlsConfig.CAData != "" {
		caData, err = base64.StdEncoding.DecodeString(tlsConfig.CAData)
		if err != nil {
			return fmt.Errorf("failed to decode CA data: %w", err)
		}
	} else if tlsConfig.CABundle != "" {
		caData, err = ioutil.ReadFile(tlsConfig.CABundle)
		if err != nil {
			return fmt.Errorf("failed to read CA bundle from %s: %w", tlsConfig.CABundle, err)
		}
	}

	if len(caData) > 0 {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caData) {
			return fmt.Errorf("failed to parse CA certificates")
		}
		config.TLSClientConfig.CAData = caData
	}

	return nil
}

// configureClientCerts configures client certificate authentication
func (cam *ClusterAuthManager) configureClientCerts(config *rest.Config, tlsConfig *ClusterTLSConfig) error {
	var certData, keyData []byte
	var err error

	// Handle certificate data (base64 encoded or file path)
	if tlsConfig.CertData != "" && tlsConfig.KeyData != "" {
		certData, err = base64.StdEncoding.DecodeString(tlsConfig.CertData)
		if err != nil {
			return fmt.Errorf("failed to decode certificate data: %w", err)
		}
		keyData, err = base64.StdEncoding.DecodeString(tlsConfig.KeyData)
		if err != nil {
			return fmt.Errorf("failed to decode key data: %w", err)
		}
	} else if tlsConfig.CertFile != "" && tlsConfig.KeyFile != "" {
		certData, err = ioutil.ReadFile(tlsConfig.CertFile)
		if err != nil {
			return fmt.Errorf("failed to read certificate file %s: %w", tlsConfig.CertFile, err)
		}
		keyData, err = ioutil.ReadFile(tlsConfig.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to read key file %s: %w", tlsConfig.KeyFile, err)
		}
	}

	if len(certData) > 0 && len(keyData) > 0 {
		// Validate that the certificate and key can be loaded
		_, err := tls.X509KeyPair(certData, keyData)
		if err != nil {
			return fmt.Errorf("invalid client certificate/key pair: %w", err)
		}
		
		config.TLSClientConfig.CertData = certData
		config.TLSClientConfig.KeyData = keyData
	}

	return nil
}

// ValidateAuthentication validates the authentication configuration
func (cam *ClusterAuthManager) ValidateAuthentication(cluster *MultiClusterClusterConfig) error {
	// Check for legacy token configuration
	if cluster.Token != "" && cluster.Auth.Method == "" {
		if len(cluster.Token) < 10 {
			return fmt.Errorf("cluster %s: token appears to be too short", cluster.Name)
		}
		return nil
	}

	switch cluster.Auth.Method {
	case "token":
		return cam.validateTokenAuth(&cluster.Auth.Token, cluster.Name)
	case "service_account":
		return cam.validateServiceAccountAuth(&cluster.Auth.ServiceAccount, cluster.Name)
	case "oidc":
		return cam.validateOIDCAuth(&cluster.Auth.OIDC, cluster.Name)
	case "exec":
		return cam.validateExecAuth(&cluster.Auth.Exec, cluster.Name)
	case "":
		return fmt.Errorf("cluster %s: authentication method is required", cluster.Name)
	default:
		return fmt.Errorf("cluster %s: unsupported authentication method '%s'", cluster.Name, cluster.Auth.Method)
	}
}

// validateTokenAuth validates token authentication configuration
func (cam *ClusterAuthManager) validateTokenAuth(tokenConfig *TokenAuthConfig, clusterName string) error {
	if tokenConfig.Value == "" {
		return fmt.Errorf("cluster %s: token value is required", clusterName)
	}
	
	if len(tokenConfig.Value) < 10 {
		return fmt.Errorf("cluster %s: token appears to be too short", clusterName)
	}

	if tokenConfig.Type != "" && tokenConfig.Type != "bearer" && tokenConfig.Type != "service_account" {
		return fmt.Errorf("cluster %s: invalid token type '%s'", clusterName, tokenConfig.Type)
	}

	return nil
}

// validateServiceAccountAuth validates service account authentication configuration
func (cam *ClusterAuthManager) validateServiceAccountAuth(saConfig *ServiceAccountConfig, clusterName string) error {
	if saConfig.TokenPath == "" {
		return fmt.Errorf("cluster %s: service account token path is required", clusterName)
	}

	// Check if token file exists and is readable
	if _, err := os.Stat(saConfig.TokenPath); os.IsNotExist(err) {
		return fmt.Errorf("cluster %s: service account token file does not exist: %s", clusterName, saConfig.TokenPath)
	}

	return nil
}

// validateOIDCAuth validates OIDC authentication configuration
func (cam *ClusterAuthManager) validateOIDCAuth(oidcConfig *OIDCConfig, clusterName string) error {
	if oidcConfig.IssuerURL == "" {
		return fmt.Errorf("cluster %s: OIDC issuer URL is required", clusterName)
	}

	if oidcConfig.ClientID == "" {
		return fmt.Errorf("cluster %s: OIDC client ID is required", clusterName)
	}

	if oidcConfig.IDToken == "" && oidcConfig.RefreshToken == "" {
		return fmt.Errorf("cluster %s: either OIDC ID token or refresh token is required", clusterName)
	}

	return nil
}

// validateExecAuth validates exec authentication configuration
func (cam *ClusterAuthManager) validateExecAuth(execConfig *ExecConfig, clusterName string) error {
	if execConfig.Command == "" {
		return fmt.Errorf("cluster %s: exec command is required", clusterName)
	}

	// Check if the command exists
	if _, err := exec.LookPath(execConfig.Command); err != nil {
		return fmt.Errorf("cluster %s: exec command not found: %s", clusterName, execConfig.Command)
	}

	return nil
}

// ValidateTLSConfig validates TLS configuration
func (cam *ClusterAuthManager) ValidateTLSConfig(tlsConfig *ClusterTLSConfig, clusterName string) error {
	// If insecure is true, no other TLS validation is needed
	if tlsConfig.Insecure {
		return nil
	}

	// Validate CA configuration
	if tlsConfig.CAData != "" {
		_, err := base64.StdEncoding.DecodeString(tlsConfig.CAData)
		if err != nil {
			return fmt.Errorf("cluster %s: invalid base64 CA data", clusterName)
		}
	} else if tlsConfig.CABundle != "" {
		if _, err := os.Stat(tlsConfig.CABundle); os.IsNotExist(err) {
			return fmt.Errorf("cluster %s: CA bundle file does not exist: %s", clusterName, tlsConfig.CABundle)
		}
	}

	// Validate client certificate configuration
	if (tlsConfig.CertData != "" || tlsConfig.CertFile != "") && (tlsConfig.KeyData == "" && tlsConfig.KeyFile == "") {
		return fmt.Errorf("cluster %s: client certificate specified but no key provided", clusterName)
	}

	if (tlsConfig.KeyData != "" || tlsConfig.KeyFile != "") && (tlsConfig.CertData == "" && tlsConfig.CertFile == "") {
		return fmt.Errorf("cluster %s: client key specified but no certificate provided", clusterName)
	}

	if tlsConfig.CertFile != "" {
		if _, err := os.Stat(tlsConfig.CertFile); os.IsNotExist(err) {
			return fmt.Errorf("cluster %s: client certificate file does not exist: %s", clusterName, tlsConfig.CertFile)
		}
	}

	if tlsConfig.KeyFile != "" {
		if _, err := os.Stat(tlsConfig.KeyFile); os.IsNotExist(err) {
			return fmt.Errorf("cluster %s: client key file does not exist: %s", clusterName, tlsConfig.KeyFile)
		}
	}

	return nil
}