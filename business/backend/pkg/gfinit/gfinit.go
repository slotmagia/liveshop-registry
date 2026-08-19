// Package gfinit initializes GoFrame configuration and logging for a process.
package gfinit

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gctx"
)

type Options struct {
	Service    string
	ConfigFile string
}

func MustInit(options Options) context.Context {
	if options.ConfigFile == "" {
		panic("gfinit: ConfigFile required")
	}
	directory, file := filepath.Split(options.ConfigFile)
	if directory == "" {
		directory = "."
	}
	adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	if !ok {
		panic("gfinit: config adapter is not *gcfg.AdapterFile")
	}
	if err := adapter.SetPath(directory); err != nil {
		panic(fmt.Sprintf("gfinit: SetPath: %v", err))
	}
	adapter.SetFileName(file)

	ctx := gctx.New()
	service := options.Service
	if service == "" {
		service = g.Cfg().MustGet(ctx, "service").String()
	}
	level := g.Cfg().MustGet(ctx, "log.level").String()
	if level == "" {
		panic("gfinit: config log.level required")
	}
	if err := g.Log().SetLevelStr(level); err != nil {
		panic(fmt.Sprintf("gfinit: log level: %v", err))
	}
	g.Log().Infof(ctx, "[gfinit] service=%s config=%s log=%s", service, options.ConfigFile, level)
	return ctx
}

func Load[T any](ctx context.Context) (*T, error) {
	var config T
	if err := g.Cfg().MustGet(ctx, ".").Scan(&config); err != nil {
		return nil, fmt.Errorf("gfinit load config: %w", err)
	}
	return &config, nil
}
