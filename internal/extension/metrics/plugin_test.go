package metrics_test

import (
	"net/http"
	"testing"

	mbtrics "moonbridge/internal/extension/metrics"
	"moonbridge/internal/extension/plugin"
	"moonbridge/internal/foundation/config"
	"moonbridge/internal/foundation/db"
)

func TestName(t *testing.T) {
	p := mbtrics.NewPlugin()
	if p.Name() != "metrics" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "metrics")
	}
}

func TestEnabledForModel(t *testing.T) {
	p := mbtrics.NewPlugin()
	// EnabledForModel should be false when disabled (no AppConfig set)
	if p.EnabledForModel("any-model") {
		t.Fatal("EnabledForModel should be false when disabled via config")
	}
}

func TestConfigSpecs(t *testing.T) {
	specs := mbtrics.ConfigSpecs()
	if len(specs) != 1 {
		t.Fatalf("ConfigSpecs returned %d specs, want 1", len(specs))
	}
	spec := specs[0]
	if spec.Name != "metrics" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "metrics")
	}
	if spec.Factory == nil {
		t.Fatal("spec.Factory should not be nil")
	}
	cfg := spec.Factory()
	if _, ok := cfg.(*mbtrics.Config); !ok {
		t.Fatalf("Factory returned %T, want *Config", cfg)
	}
}

func TestDBConsumerNilWhenDisabled(t *testing.T) {
	p := mbtrics.NewPlugin()
	if p.DBConsumer() != nil {
		t.Fatal("DBConsumer() should be nil when extension is not enabled in config")
	}
}

func TestTables(t *testing.T) {
	tables := mbtrics.MetricsTable()
	if tables.Name != "request_metrics" {
		t.Fatalf("Table name = %q, want %q", tables.Name, "request_metrics")
	}
	if tables.Schema == "" {
		t.Fatal("Schema should not be empty")
	}
	if len(tables.Indexes) != 3 {
		t.Fatalf("expected 3 indexes, got %d", len(tables.Indexes))
	}
}

func TestInitNoError(t *testing.T) {
	p := mbtrics.NewPlugin()
	ctx := plugin.PluginContext{
		AppConfig: config.Config{},
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
}

func TestShutdownNoError(t *testing.T) {
	p := mbtrics.NewPlugin()
	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestInterfaceCompliance(t *testing.T) {
	p := mbtrics.NewPlugin()
	var _ plugin.Plugin = p
	var _ plugin.ConfigSpecProvider = p
	var _ plugin.RequestCompletionHook = p
	var _ plugin.RouteRegistrar = p
	var _ plugin.DBConsumer = p
}

func TestOnRequestCompletedNilStore(t *testing.T) {
	p := mbtrics.NewPlugin()
	p.OnRequestCompleted(nil, plugin.RequestResult{
		Model:       "test",
		InputTokens: 100,
		Status:      "success",
	})
}

func TestRegisterRoutesNilStore(t *testing.T) {
	p := mbtrics.NewPlugin()
	called := false
	p.RegisterRoutes(func(pattern string, handler http.Handler) {
		called = true
	})
	if called {
		t.Fatal("RegisterRoutes should not register when store is nil")
	}
}

func TestDisablePersistence(t *testing.T) {
	p := mbtrics.NewPlugin()
	p.DisablePersistence(db.ErrNoProvider)
	// EnabledForModel should now return false
	if p.EnabledForModel("test") {
		t.Fatal("EnabledForModel should be false after DisablePersistence")
	}
}
