package grpcserver

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc/credentials"
)

type TLSConfig struct {
	CertificateFile string
	PrivateKeyFile  string
	ClientCAFile    string
}

func serverCredentials(config TLSConfig) (credentials.TransportCredentials, error) {
	certificateFile := strings.TrimSpace(config.CertificateFile)
	privateKeyFile := strings.TrimSpace(config.PrivateKeyFile)
	clientCAFile := strings.TrimSpace(config.ClientCAFile)
	if certificateFile == "" || privateKeyFile == "" || clientCAFile == "" {
		return nil, fmt.Errorf("registry gRPC TLS certificate_file, private_key_file and client_ca_file are required")
	}
	certificate, err := tls.LoadX509KeyPair(certificateFile, privateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("registry gRPC load server certificate: %w", err)
	}
	clientCAPEM, err := os.ReadFile(clientCAFile)
	if err != nil {
		return nil, fmt.Errorf("registry gRPC read client CA: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(clientCAPEM) {
		return nil, fmt.Errorf("registry gRPC client CA does not contain a certificate")
	}
	return credentials.NewTLS(&tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
	}), nil
}
