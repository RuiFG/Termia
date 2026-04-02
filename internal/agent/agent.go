package agent

import (
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
)

type Runtime struct {
	cfg       *config.Config
	db        *db.DB
	responder HITLResponder
}

func NewRuntime(cfg *config.Config, database *db.DB, responder HITLResponder) *Runtime {
	return &Runtime{
		cfg:       cfg,
		db:        database,
		responder: responder,
	}
}
