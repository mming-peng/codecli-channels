package channel

import (
	"context"
	"testing"

	cfgpkg "codecli-channels/internal/config"
)

type testDriver struct {
	id       string
	platform string
}

func (d *testDriver) ID() string { return d.id }

func (d *testDriver) Platform() string { return d.platform }

func (d *testDriver) Start(context.Context, MessageSink) error { return nil }

func (d *testDriver) Reply(context.Context, any, string) error { return nil }

func (d *testDriver) Send(context.Context, any, string) error { return nil }

func (d *testDriver) Stop(context.Context) error { return nil }

func TestRegistryCreatesDriverByID(t *testing.T) {
	reg := NewRegistry()
	reg.Register("fake", func(id string, cfg cfgpkg.ChannelConfig, dataDir string) (Driver, error) {
		if cfg.Type != "fake" {
			t.Fatalf("unexpected channel type: %s", cfg.Type)
		}
		if dataDir == "" {
			t.Fatal("expected data dir")
		}
		return &testDriver{id: id, platform: cfg.Type}, nil
	})

	driver, err := reg.Build("demo", cfgpkg.ChannelConfig{Type: "fake", Enabled: true}, t.TempDir())
	if err != nil {
		t.Fatalf("Build error: %v", err)
	}
	if driver.ID() != "demo" {
		t.Fatalf("unexpected driver id: %s", driver.ID())
	}
	if driver.Platform() != "fake" {
		t.Fatalf("unexpected platform: %s", driver.Platform())
	}
}

func TestRegistryRejectsUnknownType(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Build("demo", cfgpkg.ChannelConfig{Type: "missing"}, t.TempDir())
	if err == nil {
		t.Fatal("expected unknown type error")
	}
}
