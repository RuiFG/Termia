package agentapp

import (
	"context"
	"fmt"
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"
)

type RunContext struct {
	SessionID        string
	Query            string
	Cwd              string
	State            SessionState
	SelectedCommands []runtimeagent.Command
}

type RunSummary struct {
	SawCommand bool
}

type RunDirective struct {
	Continue  bool
	NextQuery string
	EmitText  string
}

type Middleware interface {
	BeforeRun(context.Context, *RunContext) error
	AfterRun(context.Context, *RunContext, RunSummary) (RunDirective, error)
}

type MiddlewareFactory func(MiddlewareActivation) (Middleware, error)

type MiddlewareSpec struct {
	Name        string
	Description string
	Scope       MiddlewareScope
	Factory     MiddlewareFactory
}

type Registry struct {
	specs map[string]MiddlewareSpec
}

func DefaultMiddlewareSpecs() []MiddlewareSpec {
	return []MiddlewareSpec{
		{
			Name:        "ralph-loop",
			Description: "Repeat the run when a command was executed, otherwise emit completion.",
			Scope:       MiddlewareScopeRun,
			Factory: func(MiddlewareActivation) (Middleware, error) {
				return ralphLoopMiddleware{}, nil
			},
		},
	}
}

func NewRegistry(specs ...MiddlewareSpec) *Registry {
	registry := &Registry{specs: map[string]MiddlewareSpec{}}
	for _, spec := range specs {
		registry.register(spec)
	}
	return registry
}

func (r *Registry) register(spec MiddlewareSpec) {
	if r == nil {
		return
	}
	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name == "" {
		return
	}
	if r.specs == nil {
		r.specs = map[string]MiddlewareSpec{}
	}
	r.specs[spec.Name] = spec
}

func (r *Registry) Build(activation MiddlewareActivation) (Middleware, error) {
	if r == nil {
		return nil, fmt.Errorf("middleware registry is nil")
	}

	name := strings.TrimSpace(activation.Name)
	if name == "" {
		return nil, fmt.Errorf("middleware name is required")
	}

	spec, ok := r.specs[name]
	if !ok {
		return nil, fmt.Errorf("unknown middleware %q", name)
	}
	if activation.Scope != spec.Scope {
		return nil, fmt.Errorf("middleware %q scope mismatch: want %s got %s", name, spec.Scope, activation.Scope)
	}
	if spec.Factory == nil {
		return nil, fmt.Errorf("middleware %q has no factory", name)
	}

	instance, err := spec.Factory(activation)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, fmt.Errorf("middleware %q factory returned nil", name)
	}
	return instance, nil
}

type ralphLoopMiddleware struct{}

func (ralphLoopMiddleware) BeforeRun(_ context.Context, _ *RunContext) error {
	return nil
}

func (ralphLoopMiddleware) AfterRun(_ context.Context, _ *RunContext, summary RunSummary) (RunDirective, error) {
	if summary.SawCommand {
		return RunDirective{
			Continue:  true,
			NextQuery: "请继续处理上一个命令的结果。",
		}, nil
	}
	return RunDirective{
		EmitText: "已完成",
	}, nil
}
