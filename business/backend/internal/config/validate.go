package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type field struct {
	name  string
	value string
}

func Validate(cfg *Config) error {
	for _, validate := range []func(*Config) error{validateCommon, validateServer, validateDatabase, validateWorkloadIdentity, validateHTTP, validateGRPC} {
		if err := validate(cfg); err != nil {
			return err
		}
	}
	return nil
}

func validateCommon(cfg *Config) error {
	if err := requireFields([]field{
		{name: "service", value: cfg.Service},
		{name: "log.level", value: cfg.Log.Level},
		{name: "log.format", value: cfg.Log.Format},
	}); err != nil {
		return err
	}
	if cfg.Log.Format != "text" && cfg.Log.Format != "json" {
		return fmt.Errorf("registry: config log.format must be text or json")
	}
	return nil
}

func validateServer(cfg *Config) error {
	return require("server.http", cfg.Server.HTTP)
}

func validateDatabase(cfg *Config) error {
	if err := require("database.url", cfg.Database.URL); err != nil {
		return err
	}
	if cfg.Database.MaxOpenConnections <= 0 || cfg.Database.MaxIdleConnections < 0 || cfg.Database.MaxIdleConnections > cfg.Database.MaxOpenConnections {
		return fmt.Errorf("registry: database connection pool limits are invalid")
	}
	maxLifetime, err := positiveDuration("database.connection_max_lifetime", cfg.Database.ConnectionMaxLifetime)
	if err != nil {
		return err
	}
	maxIdleTime, err := positiveDuration("database.connection_max_idle_time", cfg.Database.ConnectionMaxIdleTime)
	if err != nil {
		return err
	}
	cfg.connectionMaxLifetime = maxLifetime
	cfg.connectionMaxIdleTime = maxIdleTime
	return nil
}

func positiveDuration(name, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("registry: config %s must be a positive duration", name)
	}
	return duration, nil
}

func validateWorkloadIdentity(cfg *Config) error {
	fields := []field{
		{name: "workload_identity.issuer", value: cfg.WorkloadIdentity.Issuer},
		{name: "workload_identity.http.gateway.key_id", value: cfg.WorkloadIdentity.HTTP.Gateway.KeyID},
		{name: "workload_identity.http.gateway.public_key", value: cfg.WorkloadIdentity.HTTP.Gateway.PublicKey},
		{name: "workload_identity.http.gateway.subject", value: cfg.WorkloadIdentity.HTTP.Gateway.Subject},
		{name: "workload_identity.http.release.key_id", value: cfg.WorkloadIdentity.HTTP.Release.KeyID},
		{name: "workload_identity.http.release.public_key", value: cfg.WorkloadIdentity.HTTP.Release.PublicKey},
		{name: "workload_identity.http.release.subject", value: cfg.WorkloadIdentity.HTTP.Release.Subject},
		{name: "workload_identity.http.platform.key_id", value: cfg.WorkloadIdentity.HTTP.Platform.KeyID},
		{name: "workload_identity.http.platform.public_key", value: cfg.WorkloadIdentity.HTTP.Platform.PublicKey},
		{name: "workload_identity.http.platform.subject", value: cfg.WorkloadIdentity.HTTP.Platform.Subject},
		{name: "workload_identity.grpc.gateway.spiffe_id", value: cfg.WorkloadIdentity.GRPC.Gateway.SPIFFEID},
		{name: "workload_identity.grpc.gateway.subject", value: cfg.WorkloadIdentity.GRPC.Gateway.Subject},
		{name: "workload_identity.grpc.identity.spiffe_id", value: cfg.WorkloadIdentity.GRPC.Identity.SPIFFEID},
		{name: "workload_identity.grpc.identity.subject", value: cfg.WorkloadIdentity.GRPC.Identity.Subject},
	}
	if err := requireFields(fields); err != nil {
		return err
	}
	if err := requireSPIFFEID("workload_identity.grpc.gateway.spiffe_id", cfg.WorkloadIdentity.GRPC.Gateway.SPIFFEID); err != nil {
		return err
	}
	if err := requireSPIFFEID("workload_identity.grpc.identity.spiffe_id", cfg.WorkloadIdentity.GRPC.Identity.SPIFFEID); err != nil {
		return err
	}
	if !contains(cfg.WorkloadIdentity.HTTP.Gateway.Permissions, "registry.routes.read") {
		return fmt.Errorf("registry: config workload_identity.http.gateway.permissions must include registry.routes.read")
	}
	if !contains(cfg.WorkloadIdentity.HTTP.Release.Permissions, "registry.release.write") || !contains(cfg.WorkloadIdentity.HTTP.Release.Permissions, "registry.activation.write") {
		return fmt.Errorf("registry: config workload_identity.http.release.permissions must include registry.release.write and registry.activation.write")
	}
	if !contains(cfg.WorkloadIdentity.HTTP.Platform.Permissions, "registry.activation.write") || !contains(cfg.WorkloadIdentity.HTTP.Platform.Permissions, "registry.routes.read") {
		return fmt.Errorf("registry: config workload_identity.http.platform.permissions must include registry.activation.write and registry.routes.read")
	}
	if !exactPermissions(cfg.WorkloadIdentity.GRPC.Gateway.Permissions, "platform.registry.routes.read") {
		return fmt.Errorf("registry: config workload_identity.grpc.gateway.permissions must contain only platform.registry.routes.read")
	}
	if !exactPermissions(cfg.WorkloadIdentity.GRPC.Identity.Permissions, "platform.registry.active-capabilities.read") {
		return fmt.Errorf("registry: config workload_identity.grpc.identity.permissions must contain only platform.registry.active-capabilities.read")
	}
	return nil
}

func validateHTTP(cfg *Config) error {
	if len(cfg.HTTP.AllowedOrigins) == 0 {
		return fmt.Errorf("registry: config http.allowed_origins is required")
	}
	return nil
}

func validateGRPC(cfg *Config) error {
	return requireFields([]field{
		{name: "server.grpc", value: cfg.Server.GRPC},
		{name: "grpc.tls.certificate_file", value: cfg.GRPC.TLS.CertificateFile},
		{name: "grpc.tls.private_key_file", value: cfg.GRPC.TLS.PrivateKeyFile},
		{name: "grpc.tls.client_ca_file", value: cfg.GRPC.TLS.ClientCAFile},
	})
}

func requireFields(fields []field) error {
	for _, item := range fields {
		if err := require(item.name, item.value); err != nil {
			return err
		}
	}
	return nil
}

func require(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("registry: config %s is required", name)
	}
	return nil
}

func requireSPIFFEID(name, value string) error {
	identity, err := url.Parse(value)
	if err != nil || identity.Scheme != "spiffe" || identity.Host == "" || identity.User != nil || identity.RawQuery != "" || identity.Fragment != "" {
		return fmt.Errorf("registry: config %s must be a SPIFFE ID", name)
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func exactPermissions(values []string, expected ...string) bool {
	if len(values) != len(expected) {
		return false
	}
	for _, permission := range expected {
		if !contains(values, permission) {
			return false
		}
	}
	return true
}
