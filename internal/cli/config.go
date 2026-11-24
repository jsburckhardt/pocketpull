package cli

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"
)

type configuration struct {
	rawDir       string
	apiKeyFile   string
	stateDir     string
	refreshAfter time.Duration
	logFormat    string
	logLevel     slog.Level
	apiKey       string
}

type environment func(string) (string, bool)

// configure owns discovery, precedence, path resolution and credential loading.
// Errors deliberately contain only fixed categories, never parser input or paths.
func configure(args []string, lookup environment) (configuration, string, error) {
	var config configuration
	flags := flag.NewFlagSet("pocketpull", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	values := make(map[string]*string)
	for _, name := range []string{"config", "raw-dir", "api-key-file", "state-dir", "refresh-after", "log-format", "log-level"} {
		values[name] = flags.String(name, "", "")
	}
	help := flags.Bool("help", false, "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config, usage, nil
		}
		return config, "", errors.New("invalid arguments")
	}
	if *help {
		return config, usage, nil
	}
	if *showVersion {
		return config, "pocketpull " + version + "\n", nil
	}
	if flags.NArg() != 0 {
		return config, "", errors.New("unexpected positional arguments")
	}
	explicit := make(map[string]bool)
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	cwd, err := os.Getwd()
	if err != nil {
		return config, "", errors.New("working directory unavailable")
	}
	config = configuration{refreshAfter: 24 * time.Hour, logFormat: "json", logLevel: slog.LevelInfo}
	path, err := selectConfig(cwd, *values["config"], explicit["config"], lookup)
	if err != nil {
		return config, "", err
	}
	if path != "" {
		fileValues, err := readConfig(path)
		if err != nil {
			return config, "", err
		}
		for name, value := range fileValues {
			if err := config.apply(name, value, filepath.Dir(path)); err != nil {
				return config, "", err
			}
		}
	}
	for _, name := range []string{"raw-dir", "api-key-file", "state-dir", "refresh-after", "log-format", "log-level"} {
		if explicit[name] {
			if err := config.apply(strings.ReplaceAll(name, "-", "_"), *values[name], cwd); err != nil {
				return config, "", err
			}
		}
	}
	if config.rawDir == "" {
		return config, "", errors.New("raw directory required")
	}
	if config.stateDir == "" {
		base, _ := lookup("XDG_STATE_HOME")
		if base == "" {
			home, _ := lookup("HOME")
			if home == "" {
				return config, "", errors.New("state directory or home required")
			}
			base = filepath.Join(home, ".local", "state")
		}
		config.stateDir = resolvePath(cwd, filepath.Join(base, "pocketpull"))
	}
	config.apiKey, err = readCredential(config.apiKeyFile, lookup)
	return config, "", err
}

func selectConfig(cwd, explicitPath string, explicit bool, lookup environment) (string, error) {
	if explicit {
		if strings.TrimSpace(explicitPath) == "" {
			return "", errors.New("config path must be nonempty")
		}
		return resolvePath(cwd, explicitPath), nil
	}
	paths := []string{filepath.Join(cwd, "pocketpull.yaml")}
	base, _ := lookup("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := lookup("HOME")
		if home != "" {
			base = filepath.Join(home, ".config")
		}
	}
	if base != "" {
		paths = append(paths, resolvePath(cwd, filepath.Join(base, "pocketpull", "config.yaml")))
	}
	for _, path := range paths {
		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}
		if !os.IsNotExist(err) {
			return "", errors.New("discovered configuration unavailable")
		}
		// A dangling symlink is a found but unusable file, not permission to fall back.
		if _, linkErr := os.Lstat(path); linkErr == nil || !os.IsNotExist(linkErr) {
			return "", errors.New("discovered configuration unavailable")
		}
	}
	return "", nil
}

func readConfig(path string) (map[string]string, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("selected configuration unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("selected configuration must be a regular file")
	}
	decoder := yaml.NewDecoder(file)
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.New("invalid YAML configuration")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("configuration must contain exactly one YAML document")
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode || document.Content[0].Tag != "!!map" {
		return nil, errors.New("configuration must be a YAML mapping")
	}
	values := make(map[string]string)
	nodes := document.Content[0].Content
	for i := 0; i < len(nodes); i += 2 {
		key, value := nodes[i], nodes[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return nil, errors.New("configuration keys must be strings")
		}
		switch key.Value {
		case "raw_dir", "api_key_file", "state_dir", "refresh_after", "log_format", "log_level":
		default:
			return nil, errors.New("unknown configuration key")
		}
		if _, exists := values[key.Value]; exists {
			return nil, errors.New("duplicate configuration key")
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return nil, errors.New("configuration values must be strings")
		}
		values[key.Value] = value.Value
	}
	return values, nil
}

func (config *configuration) apply(name, value, base string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("configuration values must be nonempty")
	}
	if (name == "raw_dir" || name == "api_key_file" || name == "state_dir") && strings.ContainsRune(value, 0) {
		return errors.New("configuration path contains an invalid character")
	}
	switch name {
	case "raw_dir":
		config.rawDir = resolvePath(base, value)
	case "api_key_file":
		config.apiKeyFile = resolvePath(base, value)
	case "state_dir":
		config.stateDir = resolvePath(base, value)
	case "refresh_after":
		duration, err := time.ParseDuration(value)
		if err != nil || duration < 0 {
			return errors.New("refresh duration must be valid and nonnegative")
		}
		config.refreshAfter = duration
	case "log_format":
		if value != "json" && value != "text" {
			return errors.New("log format must be json or text")
		}
		config.logFormat = value
	case "log_level":
		switch value {
		case "debug":
			config.logLevel = slog.LevelDebug
		case "info":
			config.logLevel = slog.LevelInfo
		case "warn":
			config.logLevel = slog.LevelWarn
		case "error":
			config.logLevel = slog.LevelError
		default:
			return errors.New("log level must be debug, info, warn or error")
		}
	default:
		return errors.New("unknown configuration key")
	}
	return nil
}

func resolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func readCredential(path string, lookup environment) (string, error) {
	value, present := lookup("POCKETPULL_API_KEY")
	if present && path != "" {
		return "", errors.New("conflicting credential sources")
	}
	if !present && path == "" {
		return "", errors.New("one credential source required")
	}
	if path != "" {
		info, err := os.Stat(path)
		if err != nil || validateSecretInfo(info, os.Geteuid()) != nil {
			return "", errors.New("secret file must be regular, user-owned and unreadable by group or others")
		}
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return "", errors.New("secret file unavailable")
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || validateSecretInfo(info, os.Geteuid()) != nil {
			return "", errors.New("secret file must be regular, user-owned and unreadable by group or others")
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return "", errors.New("secret file unreadable")
		}
		value = string(data)
	}
	key := strings.TrimSpace(value)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return "", errors.New("credential must be nonempty and single-line")
	}
	return key, nil
}

func validateSecretInfo(info os.FileInfo, euid int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o044 != 0 || !ok || stat.Uid != uint32(euid) {
		return errors.New("unsafe secret file")
	}
	return nil
}
