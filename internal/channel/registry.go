package channel

import (
	"fmt"
	"sort"
	"strings"

	cfgpkg "codecli-channels/internal/config"
)

type Factory func(id string, cfg cfgpkg.ChannelConfig, dataDir string) (Driver, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

func (r *Registry) Register(kind string, factory Factory) {
	if r == nil || factory == nil {
		return
	}
	if r.factories == nil {
		r.factories = map[string]Factory{}
	}
	r.factories[strings.ToLower(strings.TrimSpace(kind))] = factory
}

func (r *Registry) Build(id string, cfg cfgpkg.ChannelConfig, dataDir string) (Driver, error) {
	if r == nil {
		return nil, fmt.Errorf("channel registry 未初始化")
	}
	kind := strings.ToLower(strings.TrimSpace(cfg.Type))
	factory, ok := r.factories[kind]
	if !ok {
		available := make([]string, 0, len(r.factories))
		for item := range r.factories {
			available = append(available, item)
		}
		sort.Strings(available)
		return nil, fmt.Errorf("channel type %q 未注册，可用类型：%v", kind, available)
	}
	return factory(id, cfg, dataDir)
}
