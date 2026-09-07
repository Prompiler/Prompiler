//go:build wireinject

package main

import (
	"github.com/Jh123x/prompiler/internal/adapters/source"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/types"
	"github.com/google/wire"
)

// initializeApplication wires the application shell (composition root).
func initializeApplication() (*domain.Application, error) {
	wire.Build(
		source.NewFSSource,
		builtin.NewRegistry,
		wire.Bind(new(types.Builtins), new(*builtin.Registry)),
		domain.NewApplication,
	)
	return nil, nil
}
