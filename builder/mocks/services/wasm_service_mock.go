package mocks

import (
	"context"
)

type MockWasmService struct {
	CheckAndUpdateFunc func(ctx context.Context) error
	DeployFunc         func(ctx context.Context, stagingDir string) error
	SetDirtyFunc       func(dirty bool)
}

func (m *MockWasmService) CheckAndUpdate(ctx context.Context) error {
	if m.CheckAndUpdateFunc != nil {
		return m.CheckAndUpdateFunc(ctx)
	}
	return nil
}

func (m *MockWasmService) Deploy(ctx context.Context, stagingDir string) error {
	if m.DeployFunc != nil {
		return m.DeployFunc(ctx, stagingDir)
	}
	return nil
}

func (m *MockWasmService) SetSearchSourceDirty(dirty bool) {
	if m.SetDirtyFunc != nil {
		m.SetDirtyFunc(dirty)
	}
}
