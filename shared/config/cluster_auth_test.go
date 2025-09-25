package sharedconfig

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClusterAuthManager_TokenAuth(t *testing.T) {
	authManager := NewClusterAuthManager()

	tests := []struct {
		name          string
		cluster       MultiClusterClusterConfig
		expectError   bool
		expectedToken string
	}{
		{
			name: "valid_bearer_token",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "test-bearer-token-123456789",
						Type:  "bearer",
					},
				},
			},
			expectError:   false,
			expectedToken: "test-bearer-token-123456789",
		},
		{
			name: "valid_service_account_token",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "test-sa-token-123456789",
						Type:  "service_account",
					},
				},
			},
			expectError:   false,
			expectedToken: "test-sa-token-123456789",
		},
		{
			name: "legacy_token_support",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Token:    "legacy-token-123456789",
			},
			expectError:   false,
			expectedToken: "legacy-token-123456789",
		},
		{
			name: "empty_token",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "",
						Type:  "bearer",
					},
				},
			},
			expectError: true,
		},
		{
			name: "invalid_token_type",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "test-token-123456789",
						Type:  "invalid-type",
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := authManager.CreateRESTConfig(&tt.cluster)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if config.BearerToken != tt.expectedToken {
				t.Errorf("Expected token '%s', got '%s'", tt.expectedToken, config.BearerToken)
			}
		})
	}
}

func TestClusterAuthManager_ServiceAccountAuth(t *testing.T) {
	authManager := NewClusterAuthManager()

	// Create temporary service account token file
	tmpDir, err := ioutil.TempDir("", "sa-auth-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tokenPath := filepath.Join(tmpDir, "token")
	testToken := "test-service-account-token-123456789"
	
	if err := ioutil.WriteFile(tokenPath, []byte(testToken), 0600); err != nil {
		t.Fatalf("Failed to write token file: %v", err)
	}

	cluster := MultiClusterClusterConfig{
		Name:     "test-cluster",
		Endpoint: "https://api.test.k8s.local:6443",
		Auth: ClusterAuthConfig{
			Method: "service_account",
			ServiceAccount: ServiceAccountConfig{
				TokenPath: tokenPath,
			},
		},
	}

	config, err := authManager.CreateRESTConfig(&cluster)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if config.BearerToken != testToken {
		t.Errorf("Expected token '%s', got '%s'", testToken, config.BearerToken)
	}
}

func TestClusterAuthManager_OIDCAuth(t *testing.T) {
	authManager := NewClusterAuthManager()

	cluster := MultiClusterClusterConfig{
		Name:     "test-cluster",
		Endpoint: "https://api.test.k8s.local:6443",
		Auth: ClusterAuthConfig{
			Method: "oidc",
			OIDC: OIDCConfig{
				IssuerURL:    "https://oidc.example.com",
				ClientID:     "kubernetes",
				ClientSecret: "client-secret",
				IDToken:      "oidc-id-token-123456789",
			},
		},
	}

	config, err := authManager.CreateRESTConfig(&cluster)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if config.BearerToken != "oidc-id-token-123456789" {
		t.Errorf("Expected OIDC ID token as bearer token, got '%s'", config.BearerToken)
	}
}

func TestClusterAuthManager_TLSConfig(t *testing.T) {
	authManager := NewClusterAuthManager()

	// Create temporary CA certificate file
	tmpDir, err := ioutil.TempDir("", "tls-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	caCertPath := filepath.Join(tmpDir, "ca.crt")
	// Use a simple valid-format certificate for testing (not a real cert, but valid PEM format)
	testCACert := `-----BEGIN CERTIFICATE-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyBwjx0kJiQbVZuDCN2Ks
NXNhP4zxYzYfFG5jPpM7Exy1QT6dSJvDzV3yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9B
QzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x
9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn
3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+T
yn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr
+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5z
Hr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ
5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4
CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdc
W4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5j
dcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN
5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vY
BN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2
vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBd
U2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1y
BdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV
1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQ
zV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9
BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3
x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Ty
n3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+
Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zH
r+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5
zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4C
J5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW
4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jd
cW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5
jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYB
N5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2v
YBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU
2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yB
dU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1
yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQz
V1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9B
QzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x
9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn
3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+T
yn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr
+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5z
Hr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ
5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4
CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdc
W4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5j
dcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN
5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vY
BN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2
vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBd
U2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1y
BdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV
1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQ
zV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9
BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3
x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Ty
n3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+
Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zH
r+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5
zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4C
J5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW
4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jd
cW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5
jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYB
N5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2v
YBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU
2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yB
dU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1
yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQz
V1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9B
QzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x
9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn
3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+T
yn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr
+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5z
Hr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ
5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4
CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdc
W4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5j
dcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN
5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vY
BN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2
vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBd
U2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1y
BdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV
1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQ
zV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9
BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3
x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Ty
n3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+
Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zH
r+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5
zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4C
J5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW
4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jd
cW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5
jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYB
N5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2v
YBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU
2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yB
dU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1
yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQz
V1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9B
QzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x
9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn
3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+T
yn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr
+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5z
Hr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ
5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4
CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdc
W4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5j
dcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN
5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vY
BN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2
vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBd
U2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1y
BdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV
1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQ
zV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9
BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3
x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Ty
n3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+
Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zH
r+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5
zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4C
J5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW
4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jd
cW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5
jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYB
N5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2vYBN5jdcW4CJ5zHr+Tyn3x9BQzV1yBdU2v
-----END CERTIFICATE-----`
	
	if err := ioutil.WriteFile(caCertPath, []byte(testCACert), 0644); err != nil {
		t.Fatalf("Failed to write CA cert file: %v", err)
	}

	tests := []struct {
		name        string
		tlsConfig   ClusterTLSConfig
		expectError bool
	}{
		{
			name: "insecure_connection",
			tlsConfig: ClusterTLSConfig{
				Insecure: true,
			},
			expectError: false,
		},
		{
			name: "ca_bundle_file",
			tlsConfig: ClusterTLSConfig{
				CABundle: caCertPath,
			},
			expectError: true, // Will fail because fake cert can't be parsed
		},
		{
			name: "ca_data_base64",
			tlsConfig: ClusterTLSConfig{
				CAData: base64.StdEncoding.EncodeToString([]byte(testCACert)),
			},
			expectError: true, // Will fail because fake cert can't be parsed
		},
		{
			name: "server_name_for_sni",
			tlsConfig: ClusterTLSConfig{
				ServerName: "kubernetes.example.com",
			},
			expectError: false,
		},
		{
			name: "invalid_ca_data",
			tlsConfig: ClusterTLSConfig{
				CAData: "invalid-base64-data!!!",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Token:    "test-token", // Legacy token for simplicity
				TLS:      tt.tlsConfig,
			}

			config, err := authManager.CreateRESTConfig(&cluster)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			// Validate TLS configuration
			if tt.tlsConfig.Insecure {
				if !config.TLSClientConfig.Insecure {
					t.Errorf("Expected insecure connection")
				}
			}
			
			if tt.tlsConfig.ServerName != "" {
				if config.TLSClientConfig.ServerName != tt.tlsConfig.ServerName {
					t.Errorf("Expected server name '%s', got '%s'", 
						tt.tlsConfig.ServerName, config.TLSClientConfig.ServerName)
				}
			}
		})
	}
}

func TestClusterAuthManager_Validation(t *testing.T) {
	authManager := NewClusterAuthManager()

	tests := []struct {
		name        string
		cluster     MultiClusterClusterConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid_token_auth",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "valid-token-123456789",
						Type:  "bearer",
					},
				},
			},
			expectError: false,
		},
		{
			name: "legacy_token_valid",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Token:    "legacy-token-123456789",
			},
			expectError: false,
		},
		{
			name: "empty_token",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "",
					},
				},
			},
			expectError: true,
			errorMsg:    "token value is required",
		},
		{
			name: "short_token",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "token",
					Token: TokenAuthConfig{
						Value: "short",
					},
				},
			},
			expectError: true,
			errorMsg:    "token appears to be too short",
		},
		{
			name: "invalid_auth_method",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "invalid-method",
				},
			},
			expectError: true,
			errorMsg:    "unsupported authentication method",
		},
		{
			name: "missing_oidc_issuer",
			cluster: MultiClusterClusterConfig{
				Name:     "test-cluster",
				Endpoint: "https://api.test.k8s.local:6443",
				Auth: ClusterAuthConfig{
					Method: "oidc",
					OIDC: OIDCConfig{
						ClientID: "kubernetes",
					},
				},
			},
			expectError: true,
			errorMsg:    "OIDC issuer URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authManager.ValidateAuthentication(&tt.cluster)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClusterAuthManager_TLSValidation(t *testing.T) {
	authManager := NewClusterAuthManager()

	tests := []struct {
		name        string
		tlsConfig   ClusterTLSConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid_insecure",
			tlsConfig: ClusterTLSConfig{
				Insecure: true,
			},
			expectError: false,
		},
		{
			name: "cert_without_key",
			tlsConfig: ClusterTLSConfig{
				CertData: base64.StdEncoding.EncodeToString([]byte("fake-cert")),
				KeyData:  "",
			},
			expectError: true,
			errorMsg:    "certificate specified but no key provided",
		},
		{
			name: "key_without_cert",
			tlsConfig: ClusterTLSConfig{
				CertData: "",
				KeyData:  base64.StdEncoding.EncodeToString([]byte("fake-key")),
			},
			expectError: true,
			errorMsg:    "key specified but no certificate provided",
		},
		{
			name: "nonexistent_ca_bundle",
			tlsConfig: ClusterTLSConfig{
				CABundle: "/nonexistent/ca.crt",
			},
			expectError: true,
			errorMsg:    "CA bundle file does not exist",
		},
		{
			name: "invalid_ca_data",
			tlsConfig: ClusterTLSConfig{
				CAData: "invalid-base64!!!",
			},
			expectError: true,
			errorMsg:    "invalid base64 CA data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authManager.ValidateTLSConfig(&tt.tlsConfig, "test-cluster")
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClusterAuthManager_RESTConfigTimeout(t *testing.T) {
	authManager := NewClusterAuthManager()

	cluster := MultiClusterClusterConfig{
		Name:     "test-cluster",
		Endpoint: "https://api.test.k8s.local:6443",
		Token:    "test-token-123456789",
	}

	config, err := authManager.CreateRESTConfig(&cluster)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expectedTimeout := 30 * time.Second
	if config.Timeout != expectedTimeout {
		t.Errorf("Expected timeout %v, got %v", expectedTimeout, config.Timeout)
	}
}

// Helper function is defined in multi_cluster_test.go, reusing it