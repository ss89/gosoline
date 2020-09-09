package application

import (
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

var DefaultMinimalAppOptions = []Option{
	WithUTCClock(true),
	WithConfigErrorHandlers(defaultErrorHandler),
	WithConfigFile("./config.dist.yml", "yml"),
	WithConfigFileFlag,
	WithConfigEnvKeyReplacer(cfg.DefaultEnvKeyReplacer),
	WithConfigSanitizers(cfg.TimeSanitizer),
	WithLoggerApplicationTag,
	WithLoggerContextFieldsMessageEncoder,
	WithLoggerContextFieldsResolver(log.ContextLoggerFieldsResolver),
	WithLoggerHandlersFromConfig,
	WithKernelSettingsFromConfig,
}

var DefaultServiceAppOptions = append(DefaultMinimalAppOptions, []Option{
	WithMetadataServer,
	WithConsumerMessagesPerRunnerMetrics,
	WithLoggerMetricHandler,
	WithLoggerSentryHandler(log.SentryContextConfigProvider, log.SentryContextEcsMetadataProvider),
	WithApiHealthCheck,
	WithMetricDaemon,
	WithProducerDaemon,
	WithTracing,
}...)

var DefaultCliApp = append(DefaultMinimalAppOptions, []Option{}...)
