package application

import (
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/mon"
)

var DefaultMinimalAppOptions = []Option{
	WithUTCClock(true),
	WithConfigErrorHandlers(defaultErrorHandler),
	WithConfigFile("./config.dist.yml", "yml"),
	WithConfigFileFlag,
	WithConfigEnvKeyReplacer(cfg.DefaultEnvKeyReplacer),
	WithConfigSanitizers(cfg.TimeSanitizer),
	WithLoggerApplicationTag,
	WithLoggerTagsFromConfig,
	WithLoggerSettingsFromConfig,
	WithLoggerContextFieldsMessageEncoder(),
	WithLoggerContextFieldsResolver(mon.ContextLoggerFieldsResolver),
	WithKernelSettingsFromConfig,
}

var DefaultServiceAppOptions = append(DefaultMinimalAppOptions, []Option{
	WithConfigServer,
	WithConsumerMessagesPerRunnerMetrics,
	WithLoggerMetricHook,
	WithLoggerSentryHook(mon.SentryExtraConfigProvider, mon.SentryExtraEcsMetadataProvider),
	WithApiHealthCheck,
	WithMetricDaemon,
	WithProducerDaemon,
	WithTracing,
}...)

var DefaultCliApp = append(DefaultMinimalAppOptions, []Option{}...)
