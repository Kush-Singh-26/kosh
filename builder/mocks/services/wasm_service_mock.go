package mocks

import (
	"context"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

type MockWasmService struct {
	CheckAndUpdateFunc func(ctx context.Context) (bool, error)
	DeployFunc         func(ctx context.Context, sink fspkg.ArtifactSink) error
	SetDirtyFunc       func(dirty bool)
}

func (m *MockWasmService) CheckAndUpdate(ctx context.Context) (bool, error) {
	if m.CheckAndUpdateFunc != nil {
		return m.CheckAndUpdateFunc(ctx)
	}
	return false, nil
}

func (m *MockWasmService) Deploy(ctx context.Context, sink fspkg.ArtifactSink) error {
	if m.DeployFunc != nil {
		return m.DeployFunc(ctx, sink)
	}
	return nil
}

func (m *MockWasmService) SetSearchSourceDirty(dirty bool) {
	if m.SetDirtyFunc != nil {
		m.SetDirtyFunc(dirty)
	}
}
