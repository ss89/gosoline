package metric

import (
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

type noopWriter struct {
}

func NewNoopWriter(config cfg.Config, logger log.Logger) (*noopWriter, error) {
	return &noopWriter{}, nil
}

func (w noopWriter) GetPriority() int {
	return PriorityLow
}

func (w noopWriter) Write(batch Data) {
	return
}

func (w noopWriter) WriteOne(data *Datum) {
	return
}
