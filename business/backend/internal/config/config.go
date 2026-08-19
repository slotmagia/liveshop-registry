package config

import (
	"context"
	"fmt"
	"time"

	commonconfig "github.com/lvtuopen-ai/liveshop-registry/backend/pkg/config"
	"github.com/lvtuopen-ai/liveshop-registry/backend/pkg/gfinit"
)

type BearerWorkload struct {
	KeyID       string   `yaml:"key_id"`
	PublicKey   string   `yaml:"public_key"`
	Subject     string   `yaml:"subject"`
	Permissions []string `yaml:"permissions"`
}

type MTLSWorkload struct {
	SPIFFEID    string   `yaml:"spiffe_id"`
	Subject     string   `yaml:"subject"`
	Permissions []string `yaml:"permissions"`
}

type Config struct {
	commonconfig.Common `yaml:",inline"`
	Server              commonconfig.Server `yaml:"server"`
	Database            struct {
		URL                   string `yaml:"url"`
		MaxOpenConnections    int    `yaml:"max_open_connections"`
		MaxIdleConnections    int    `yaml:"max_idle_connections"`
		ConnectionMaxLifetime string `yaml:"connection_max_lifetime"`
		ConnectionMaxIdleTime string `yaml:"connection_max_idle_time"`
	} `yaml:"database"`
	WorkloadIdentity struct {
		Issuer string `yaml:"issuer"`
		HTTP   struct {
			Gateway  BearerWorkload `yaml:"gateway"`
			Release  BearerWorkload `yaml:"release"`
			Platform BearerWorkload `yaml:"platform"`
		} `yaml:"http"`
		GRPC struct {
			Gateway  MTLSWorkload `yaml:"gateway"`
			Identity MTLSWorkload `yaml:"identity"`
		} `yaml:"grpc"`
	} `yaml:"workload_identity"`
	HTTP struct {
		AllowedOrigins []string `yaml:"allowed_origins"`
	} `yaml:"http"`
	GRPC struct {
		TLS struct {
			CertificateFile string `yaml:"certificate_file"`
			PrivateKeyFile  string `yaml:"private_key_file"`
			ClientCAFile    string `yaml:"client_ca_file"`
		} `yaml:"tls"`
	} `yaml:"grpc"`

	connectionMaxLifetime time.Duration
	connectionMaxIdleTime time.Duration
}

func (c *Config) ConnectionMaxLifetime() time.Duration { return c.connectionMaxLifetime }
func (c *Config) ConnectionMaxIdleTime() time.Duration { return c.connectionMaxIdleTime }

func Load(ctx context.Context) (*Config, error) {
	cfg, err := gfinit.Load[Config](ctx)
	if err != nil {
		return nil, fmt.Errorf("registry: load config: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
