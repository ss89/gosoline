package cli

import (
	"context"
	"github.com/applike/gosoline/pkg/application"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/kernel"
	"github.com/applike/gosoline/pkg/mon"
	"strings"
)

type Module interface {
	Boot(config cfg.Config, logger mon.Logger) error
	Run(ctx context.Context) error
}

type cliModule struct {
	kernel.EssentialModule
	kernel.ApplicationStage
	Module
}

func newCliModule(module Module) *cliModule {
	return &cliModule{
		Module: module,
	}
}

func Run(module Module) {
	k := application.New(
		application.WithUTCClock(true),
		application.WithConfigErrorHandlers(defaultErrorHandler),
		application.WithConfigFile("./config.dist.yml", "yml"),
		application.WithConfigFileFlag,
		application.WithConfigEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_")),
		application.WithConfigSanitizers(cfg.TimeSanitizer),
		application.WithLoggerFormat(mon.FormatConsole),
		application.WithLoggerApplicationTag,
		application.WithLoggerTagsFromConfig,
		application.WithLoggerSettingsFromConfig,
		application.WithLoggerContextFieldsMessageEncoder(),
		application.WithLoggerContextFieldsResolver(mon.ContextLoggerFieldsResolver),
		application.WithLoggerSentryHook(mon.SentryExtraConfigProvider, mon.SentryExtraEcsMetadataProvider),
		application.WithKernelSettingsFromConfig,
	)
	k.Add("cli", newCliModule(module))
	k.Run()
}
