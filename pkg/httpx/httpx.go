package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"astra/pkg/config"
)

func ListenAndServe(srv *http.Server, cfg *config.Config) error {
	if cfg.TLSEnabled {
		if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
			return fmt.Errorf("TLS enabled but cert/key files are missing")
		}
		return srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	}
	return srv.ListenAndServe()
}

func NewClient(cfg *config.Config, timeout time.Duration) (*http.Client, error) {
	transport := &http.Transport{}
	if cfg.TLSEnabled {
		tlsCfg := &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
			ServerName:         cfg.TLSServerName,
		}
		if cfg.TLSCAFile != "" {
			pem, err := os.ReadFile(cfg.TLSCAFile)
			if err != nil {
				return nil, fmt.Errorf("read tls ca file: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("append tls ca cert failed")
			}
			tlsCfg.RootCAs = pool
		}
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
			if err != nil {
				return nil, fmt.Errorf("load tls keypair: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		transport.TLSClientConfig = tlsCfg
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}
