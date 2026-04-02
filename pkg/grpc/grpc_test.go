package grpc

import (
	"os"
	"path/filepath"
	"testing"

	"astra/pkg/config"

	gogrpc "google.golang.org/grpc"
)

// TestNewServer_NotNil verifies NewServer returns a non-nil gRPC server.
func TestNewServer_NotNil(t *testing.T) {
	s := NewServer()
	if s == nil {
		t.Fatal("NewServer() returned nil")
	}
}

// TestNewServer_WithExtraOptions verifies NewServer accepts additional ServerOptions without panic.
func TestNewServer_WithExtraOptions(t *testing.T) {
	s := NewServer(gogrpc.MaxRecvMsgSize(4 * 1024 * 1024))
	if s == nil {
		t.Fatal("NewServer(MaxRecvMsgSize) returned nil")
	}
}

// TestNewServerFromConfig_TLSDisabled verifies TLS-disabled config returns a server with no error.
func TestNewServerFromConfig_TLSDisabled(t *testing.T) {
	cfg := &config.Config{TLSEnabled: false}
	s, err := NewServerFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewServerFromConfig(TLSEnabled=false): %v", err)
	}
	if s == nil {
		t.Fatal("NewServerFromConfig returned nil server")
	}
}

// TestNewServerFromConfig_TLSEnabled_MissingCert verifies enabling TLS without files errors.
func TestNewServerFromConfig_TLSEnabled_MissingCert(t *testing.T) {
	cfg := &config.Config{TLSEnabled: true, TLSCertFile: "", TLSKeyFile: ""}
	_, err := NewServerFromConfig(cfg)
	if err == nil {
		t.Fatal("want error for TLS with no cert/key, got nil")
	}
}

// TestNewServerFromConfig_TLSEnabled_BadCertFile verifies nonexistent cert/key produces error.
func TestNewServerFromConfig_TLSEnabled_BadCertFile(t *testing.T) {
	cfg := &config.Config{
		TLSEnabled:  true,
		TLSCertFile: "/nonexistent/cert.pem",
		TLSKeyFile:  "/nonexistent/key.pem",
	}
	_, err := NewServerFromConfig(cfg)
	if err == nil {
		t.Fatal("want error for nonexistent cert/key, got nil")
	}
}

// TestDialOptions_TLSDisabled verifies insecure credentials are returned when TLS is off.
func TestDialOptions_TLSDisabled(t *testing.T) {
	cfg := &config.Config{TLSEnabled: false}
	opts, err := dialOptions(cfg)
	if err != nil {
		t.Fatalf("dialOptions(TLSEnabled=false): %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected at least one DialOption")
	}
}

// TestDialOptions_TLSEnabled_NoCAFile verifies TLS without CA file returns options successfully.
func TestDialOptions_TLSEnabled_NoCAFile(t *testing.T) {
	cfg := &config.Config{
		TLSEnabled:            true,
		TLSCAFile:             "",
		TLSCertFile:           "",
		TLSKeyFile:            "",
		TLSServerName:         "my-server",
		TLSInsecureSkipVerify: true,
	}
	opts, err := dialOptions(cfg)
	if err != nil {
		t.Fatalf("dialOptions(TLS, no CA): %v", err)
	}
	if len(opts) == 0 {
		t.Error("expected at least one DialOption")
	}
}

// TestDialOptions_TLSEnabled_BadCAFile verifies a missing CA file returns an error.
func TestDialOptions_TLSEnabled_BadCAFile(t *testing.T) {
	cfg := &config.Config{TLSEnabled: true, TLSCAFile: "/nonexistent/ca.pem"}
	_, err := dialOptions(cfg)
	if err == nil {
		t.Fatal("want error for bad CA file, got nil")
	}
}

// TestClientTLSConfig_BasicFields verifies ServerName and InsecureSkipVerify are threaded through.
func TestClientTLSConfig_BasicFields(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		skipVerify bool
	}{
		{"server name set", "example.com", false},
		{"insecure skip", "", true},
		{"both set", "srv.local", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				TLSEnabled:            true,
				TLSServerName:         tc.serverName,
				TLSInsecureSkipVerify: tc.skipVerify,
			}
			tlsCfg, err := clientTLSConfig(cfg)
			if err != nil {
				t.Fatalf("clientTLSConfig: %v", err)
			}
			if tlsCfg.ServerName != tc.serverName {
				t.Errorf("ServerName: want %q, got %q", tc.serverName, tlsCfg.ServerName)
			}
			if tlsCfg.InsecureSkipVerify != tc.skipVerify {
				t.Errorf("InsecureSkipVerify: want %v, got %v", tc.skipVerify, tlsCfg.InsecureSkipVerify)
			}
		})
	}
}

// TestClientTLSConfig_NoCerts verifies no client certs are set when cert/key are empty.
func TestClientTLSConfig_NoCerts(t *testing.T) {
	cfg := &config.Config{TLSEnabled: true}
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		t.Fatalf("clientTLSConfig: %v", err)
	}
	if len(tlsCfg.Certificates) != 0 {
		t.Errorf("Certificates: want 0, got %d", len(tlsCfg.Certificates))
	}
}

// TestClientTLSConfig_BadCAFile verifies a missing CA file returns an error.
func TestClientTLSConfig_BadCAFile(t *testing.T) {
	cfg := &config.Config{TLSEnabled: true, TLSCAFile: "/nonexistent/ca.pem"}
	_, err := clientTLSConfig(cfg)
	if err == nil {
		t.Fatal("clientTLSConfig with bad CA: want error, got nil")
	}
}

// TestClientTLSConfig_BadClientCert verifies a missing client cert/key returns an error.
func TestClientTLSConfig_BadClientCert(t *testing.T) {
	cfg := &config.Config{
		TLSEnabled:  true,
		TLSCertFile: "/nonexistent/cert.pem",
		TLSKeyFile:  "/nonexistent/key.pem",
	}
	_, err := clientTLSConfig(cfg)
	if err == nil {
		t.Fatal("clientTLSConfig with bad cert/key: want error, got nil")
	}
}

// TestServerTLSConfig_MissingFiles covers all missing-file combinations.
func TestServerTLSConfig_MissingFiles(t *testing.T) {
	tests := []struct {
		name     string
		certFile string
		keyFile  string
	}{
		{"both empty", "", ""},
		{"cert only", "/some/cert.pem", ""},
		{"key only", "", "/some/key.pem"},
		{"both nonexistent", "/nonexistent/cert.pem", "/nonexistent/key.pem"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				TLSEnabled:  true,
				TLSCertFile: tc.certFile,
				TLSKeyFile:  tc.keyFile,
			}
			_, err := serverTLSConfig(cfg)
			if err == nil {
				t.Errorf("serverTLSConfig(cert=%q, key=%q): want error, got nil", tc.certFile, tc.keyFile)
			}
		})
	}
}

// TestLoadCAPool_NonexistentFile verifies loadCAPool errors on a missing file.
func TestLoadCAPool_NonexistentFile(t *testing.T) {
	_, err := loadCAPool("/nonexistent/ca.pem")
	if err == nil {
		t.Fatal("loadCAPool with nonexistent file: want error, got nil")
	}
}

// TestLoadCAPool_InvalidPEM verifies loadCAPool errors when file has no valid PEM cert.
func TestLoadCAPool_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.pem")
	if err := os.WriteFile(path, []byte("this is not a certificate"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := loadCAPool(path)
	if err == nil {
		t.Fatal("loadCAPool with invalid PEM: want error, got nil")
	}
}
