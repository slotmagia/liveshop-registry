// Package config defines shared YAML configuration contracts.
package config

type Common struct {
	Service    string `yaml:"service"`
	InstanceID string `yaml:"instance_id"`
	Log        Log    `yaml:"log"`
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Server struct {
	HTTP string `yaml:"http"`
	GRPC string `yaml:"grpc"`
}
