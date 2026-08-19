package main

import (
	"flag"
	"os"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/lvtuopen-ai/kernel-go/lifecycle"
	"github.com/lvtuopen-ai/kernel-go/logctx"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/app"
	"github.com/lvtuopen-ai/liveshop-registry/backend/pkg/gfinit"
)

func main() {
	configPath := flag.String("config", "./configs/registry.yaml", "path to YAML config")
	flag.Parse()
	initialized := gfinit.MustInit(gfinit.Options{Service: "registry", ConfigFile: *configPath})
	logLevel := g.Cfg().MustGet(initialized, "log.level").String()
	logFormat := g.Cfg().MustGet(initialized, "log.format").String()
	logctx.Configure(logctx.Options{Service: "registry", Level: logLevel, JSON: strings.EqualFold(logFormat, "json")})
	ctx, cancel := lifecycle.SignalContext(initialized)
	defer cancel()
	if err := app.Run(ctx); err != nil {
		logctx.FromContext(ctx).Error("registry stopped with an error", "error", err)
		os.Exit(1)
	}
}
