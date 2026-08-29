package appconfig

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaultsWhenVariablesAreAbsent(t *testing.T) {
	config, err := Load(mapLookup(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := Config{
		Environment: EnvironmentDevelopment,
		HTTP: HTTPConfig{
			Address:           ":8080",
			ShutdownTimeout:   5 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		Log: LogConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
		},
	}
	if !reflect.DeepEqual(config, want) {
		t.Fatalf("Load() = %#v, want %#v", config, want)
	}
}

func TestLoadAppliesCompleteOverride(t *testing.T) {
	variables := map[string]string{
		environmentVariable:           "production",
		httpAddressVariable:           "127.0.0.1:9090",
		httpShutdownTimeoutVariable:   "10s",
		httpReadHeaderTimeoutVariable: "3s",
		httpReadTimeoutVariable:       "20s",
		httpWriteTimeoutVariable:      "45s",
		httpIdleTimeoutVariable:       "2m",
		logLevelVariable:              "warn",
		logFormatVariable:             "text",
	}

	config, err := Load(mapLookup(variables))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := Config{
		Environment: EnvironmentProduction,
		HTTP: HTTPConfig{
			Address:           "127.0.0.1:9090",
			ShutdownTimeout:   10 * time.Second,
			ReadHeaderTimeout: 3 * time.Second,
			ReadTimeout:       20 * time.Second,
			WriteTimeout:      45 * time.Second,
			IdleTimeout:       2 * time.Minute,
		},
		Log: LogConfig{
			Level:  LogLevelWarn,
			Format: LogFormatText,
		},
	}
	if !reflect.DeepEqual(config, want) {
		t.Fatalf("Load() = %#v, want %#v", config, want)
	}
}

func TestLoadAcceptsEverySupportedEnumValue(t *testing.T) {
	for _, environment := range []Environment{
		EnvironmentDevelopment,
		EnvironmentTest,
		EnvironmentStaging,
		EnvironmentProduction,
	} {
		t.Run("environment_"+string(environment), func(t *testing.T) {
			config, err := Load(mapLookup(map[string]string{environmentVariable: string(environment)}))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.Environment != environment {
				t.Fatalf("environment = %q, want %q", config.Environment, environment)
			}
		})
	}

	for _, level := range []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError} {
		t.Run("log_level_"+string(level), func(t *testing.T) {
			config, err := Load(mapLookup(map[string]string{logLevelVariable: string(level)}))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.Log.Level != level {
				t.Fatalf("log level = %q, want %q", config.Log.Level, level)
			}
		})
	}

	for _, format := range []LogFormat{LogFormatJSON, LogFormatText} {
		t.Run("log_format_"+string(format), func(t *testing.T) {
			config, err := Load(mapLookup(map[string]string{logFormatVariable: string(format)}))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config.Log.Format != format {
				t.Fatalf("log format = %q, want %q", config.Log.Format, format)
			}
		})
	}
}

func TestLoadRejectsPresentEmptyVariables(t *testing.T) {
	variables := []string{
		environmentVariable,
		httpAddressVariable,
		httpShutdownTimeoutVariable,
		httpReadHeaderTimeoutVariable,
		httpReadTimeoutVariable,
		httpWriteTimeoutVariable,
		httpIdleTimeoutVariable,
		logLevelVariable,
		logFormatVariable,
	}

	for _, variable := range variables {
		t.Run(variable, func(t *testing.T) {
			_, err := Load(mapLookup(map[string]string{variable: "   "}))
			if err == nil {
				t.Fatal("Load() error = nil, want empty-value failure")
			}
			if !strings.Contains(err.Error(), variable+" must not be empty") {
				t.Fatalf("Load() error = %q, want variable and empty-value constraint", err)
			}
		})
	}
}

func TestLoadRejectsInvalidValuesWithoutEchoingThem(t *testing.T) {
	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{name: "environment", variable: environmentVariable, value: "TOP_SECRET_ENV"},
		{name: "address without port", variable: httpAddressVariable, value: "TOP_SECRET_HOST"},
		{name: "address with zero port", variable: httpAddressVariable, value: "localhost:0"},
		{name: "shutdown syntax", variable: httpShutdownTimeoutVariable, value: "TOP_SECRET_DURATION"},
		{name: "read header zero", variable: httpReadHeaderTimeoutVariable, value: "00h"},
		{name: "read negative", variable: httpReadTimeoutVariable, value: "-1s"},
		{name: "write over maximum", variable: httpWriteTimeoutVariable, value: "11m"},
		{name: "idle over maximum", variable: httpIdleTimeoutVariable, value: "11m"},
		{name: "log level", variable: logLevelVariable, value: "TOP_SECRET_LEVEL"},
		{name: "log format", variable: logFormatVariable, value: "TOP_SECRET_FORMAT"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(mapLookup(map[string]string{test.variable: test.value}))
			if err == nil {
				t.Fatal("Load() error = nil, want validation failure")
			}
			if !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("Load() error = %q, want variable %s", err, test.variable)
			}
			if strings.Contains(err.Error(), test.value) {
				t.Fatalf("Load() error echoed supplied value: %q", err)
			}
		})
	}
}

func TestLoadReportsMultipleIndependentProblems(t *testing.T) {
	variables := map[string]string{
		environmentVariable:         "local",
		httpAddressVariable:         "localhost",
		httpReadTimeoutVariable:     "00h",
		httpWriteTimeoutVariable:    "invalid-duration",
		logLevelVariable:            "trace",
		logFormatVariable:           "yaml",
		httpIdleTimeoutVariable:     "10m1ns",
		httpShutdownTimeoutVariable: "2m1ns",
	}

	config, err := Load(mapLookup(variables))
	if err == nil {
		t.Fatal("Load() error = nil, want aggregated validation failure")
	}
	if config != (Config{}) {
		t.Fatalf("Load() config = %#v, want zero value on failure", config)
	}
	for variable, value := range variables {
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("Load() error = %q, want variable %s", err, variable)
		}
		if strings.Contains(err.Error(), value) {
			t.Errorf("Load() error echoed supplied value %q", value)
		}
	}
}

func TestLoadRequiresLookupFunction(t *testing.T) {
	_, err := Load(nil)
	if err == nil {
		t.Fatal("Load(nil) error = nil, want failure")
	}
}

func TestValidAddressAcceptsSupportedListenerForms(t *testing.T) {
	addresses := []string{
		":8080",
		"localhost:8080",
		"api.internal.example:443",
		"127.0.0.1:8080",
		"[::1]:8080",
	}
	for _, address := range addresses {
		t.Run(address, func(t *testing.T) {
			if !validAddress(address) {
				t.Fatalf("validAddress(%q) = false, want true", address)
			}
		})
	}
}

func mapLookup(values map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		value, found := values[key]
		return value, found
	}
}
