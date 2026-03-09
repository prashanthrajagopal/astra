package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"astra/pkg/config"

	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func NewServerFromConfig(cfg *config.Config) (*gogrpc.Server, error) {
	if !cfg.TLSEnabled {
		return NewServer(), nil
	}
	tlsCfg, err := serverTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewServer(gogrpc.Creds(credentials.NewTLS(tlsCfg))), nil
}

func Dial(ctx context.Context, target string, cfg *config.Config) (*gogrpc.ClientConn, error) {
	opts, err := dialOptions(cfg)
	if err != nil {
		return nil, err
	}
	return gogrpc.NewClient(target, opts...)
}

func dialOptions(cfg *config.Config) ([]gogrpc.DialOption, error) {
	if !cfg.TLSEnabled {
		return []gogrpc.DialOption{gogrpc.WithTransportCredentials(insecure.NewCredentials())}, nil
	}
	tlsCfg, err := clientTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	return []gogrpc.DialOption{gogrpc.WithTransportCredentials(credentials.NewTLS(tlsCfg))}, nil
}

func serverTLSConfig(cfg *config.Config) (*tls.Config, error) {
	if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
		return nil, fmt.Errorf("TLS enabled but ASTRA_TLS_CERT_FILE or ASTRA_TLS_KEY_FILE missing")
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load tls keypair: %w", err)
	}
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}
	if cfg.TLSCAFile != "" {
		pool, err := loadCAPool(cfg.TLSCAFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.ClientCAs = pool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return tlsCfg, nil
}

func clientTLSConfig(cfg *config.Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
		ServerName:         cfg.TLSServerName,
	}
	if cfg.TLSCAFile != "" {
		pool, err := loadCAPool(cfg.TLSCAFile)
		if err != nil {
			return nil, err
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
	return tlsCfg, nil
}

func loadCAPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tls ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("append tls ca cert failed")
	}
	return pool, nil
}
