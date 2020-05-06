package test

import (
	"fmt"
	"github.com/applike/gosoline/pkg/cfg"
	"github.com/applike/gosoline/pkg/mon"
	"github.com/hashicorp/go-multierror"
	"sync"
	"time"
)

type mockComponent interface {
	Boot(config cfg.Config, logger mon.Logger, runner *dockerRunnerLegacy, settings *mockSettings, name string)
	Start() error
}

type mockSettings struct {
	Host        string
	Debug       bool          `cfg:"debug"`
	Component   string        `cfg:"component"`
	ExpireAfter time.Duration `cfg:"expire_after" default:"60s"`
}

type mockComponentBase struct {
	logger mon.Logger
	runner *dockerRunnerLegacy
	name   string
}

type Mocks struct {
	waitGroup    sync.WaitGroup
	dockerRunner *dockerRunnerLegacy
	components   map[string]mockComponent
	logger       mon.Logger
	errors       *multierror.Error
	errorsLock   sync.Mutex
}

func newMocks(logger mon.Logger) *Mocks {
	dockerRunner := NewDockerRunnerLegacy()
	logger = logger.WithChannel("mocks")

	return &Mocks{
		components:   make(map[string]mockComponent),
		dockerRunner: dockerRunner,
		logger:       logger,
	}
}

func (m *Mocks) Boot(config cfg.Config) error {
	m.bootFromConfig(config)

	m.waitGroup.Wait()

	err := m.errors.ErrorOrNil()
	if err != nil {
		return fmt.Errorf("failed to boot at least one test component: %w", err)
	}

	m.logger.Info("test environment up and running")

	return nil
}

func (m *Mocks) bootFromConfig(config cfg.Config) {
	mocks := config.GetStringMap("mocks")

	for name, _ := range mocks {
		settings := &mockSettings{}
		key := fmt.Sprintf("mocks.%s", name)
		config.UnmarshalKey(key, settings)

		component, err := m.createComponent(settings.Component)
		if err != nil {
			m.errorsLock.Lock()
			m.errors = multierror.Append(m.errors, err)
			m.errorsLock.Unlock()
			continue
		}

		m.components[name] = component
		component.Boot(config, m.logger, m.dockerRunner, settings, name)

		m.runComponent(component)
	}
}

func (m *Mocks) createComponent(component string) (mockComponent, error) {
	switch component {
	case "mysql":
		return &mysqlComponentLegacy{}, nil
	case componentSnsSqs:
		return &snsSqsComponent{}, nil
	case "cloudwatch":
		return &cloudwatchComponent{}, nil
	case "dynamodb":
		return &dynamoDbComponent{}, nil
	case "elasticsearch":
		return &elasticsearchComponent{}, nil
	case componentKinesis:
		return &kinesisComponent{}, nil
	case "wiremock":
		return &wiremockComponent{}, nil
	case "redis":
		return &redisComponent{}, nil
	default:
		return nil, fmt.Errorf("unknown component type: %s", component)
	}
}

func (m *Mocks) runComponent(component mockComponent) {
	m.waitGroup.Add(1)

	go func() {
		defer m.waitGroup.Done()

		err := component.Start()
		if err != nil {
			m.errorsLock.Lock()
			m.errors = multierror.Append(m.errors, err)
			m.errorsLock.Unlock()
		}
	}()
}

func (m *Mocks) Shutdown() {
	m.dockerRunner.RemoveAllContainers()
}
