package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPFetch                     HTTPFetchPolicy
	OpenAIAPIKey                  string
	OpenAIModel                   string
	OpenAIReasoningEffort         string
	TavilyAPIKey                  string
	DiscordToken                  string
	PromptsDir                    string
	SessionTimeoutMinutes         int
	ConversationHistoryWindowSize int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		OpenAIAPIKey:                  os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:                   envOrDefault("OPENAI_MODEL", "gpt-5.5"),
		OpenAIReasoningEffort:         envOrDefault("OPENAI_REASONING_EFFORT", "none"),
		TavilyAPIKey:                  os.Getenv("TAVILY_API_KEY"),
		DiscordToken:                  os.Getenv("DISCORD_TOKEN"),
		PromptsDir:                    os.Getenv("PROMPTS_DIR"),
		SessionTimeoutMinutes:         envIntOrDefault("SESSION_TIMEOUT_MINUTES", 30),
		ConversationHistoryWindowSize: envIntOrDefault("CONVERSATION_HISTORY_WINDOW_SIZE", 20),
	}

	var err error
	cfg.HTTPFetch, err = loadHTTPFetchPolicy()
	if err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required")
	}
	if c.TavilyAPIKey == "" {
		return fmt.Errorf("TAVILY_API_KEY is required")
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// HTTPFetchPolicy configures public research requests. Network and header
// restrictions are mandatory and cannot be disabled through configuration.
type HTTPFetchPolicy struct {
	Enabled              bool
	AllowedHosts         []string
	DeniedHosts          []string
	AllowedPorts         []int
	AllowMutations       bool
	MutationAllowedHosts []string
	MaxTimeoutMS         int
	MaxResponseBytes     int
	MaxTextBytes         int
	MaxRedirects         int
}

func DefaultHTTPFetchPolicy() HTTPFetchPolicy {
	return HTTPFetchPolicy{Enabled: true, AllowedPorts: []int{80, 443}, MaxTimeoutMS: 30000,
		MaxResponseBytes: 524288, MaxTextBytes: 65536, MaxRedirects: 5}
}

// NormalizeHTTPFetchHost accepts ASCII DNS names or canonical IP literals,
// optionally with a single leading wildcard. Wildcards match subdomains only.
func NormalizeHTTPFetchHost(host string, pattern bool) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	wildcard := pattern && strings.HasPrefix(host, "*.")
	if wildcard {
		host = strings.TrimPrefix(host, "*.")
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.ContainsAny(host, "%:/\\@?#[]") {
		// Colons are permitted only in valid, unzoned IPv6 literals below.
		if ip, err := netip.ParseAddr(host); err == nil && ip.Zone() == "" && !wildcard {
			return ip.Unmap().String(), nil
		}
		return "", fmt.Errorf("invalid hostname")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if wildcard {
			return "", fmt.Errorf("invalid wildcard")
		}
		return ip.Unmap().String(), nil
	}
	if len(host) > 253 {
		return "", fmt.Errorf("invalid hostname")
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid hostname")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", fmt.Errorf("invalid hostname")
			}
		}
	}
	// Reject legacy integer/octal/hex IP notation and numeric TLDs.
	last := labels[len(labels)-1]
	if last[0] >= '0' && last[0] <= '9' {
		return "", fmt.Errorf("ambiguous numeric hostname")
	}
	if wildcard {
		return "*." + host, nil
	}
	return host, nil
}

func (p HTTPFetchPolicy) Validate() error {
	if len(p.AllowedPorts) == 0 {
		return fmt.Errorf("HTTP_FETCH_ALLOWED_PORTS must not be empty")
	}
	for _, port := range p.AllowedPorts {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid HTTP_FETCH_ALLOWED_PORTS")
		}
	}
	for name, hosts := range map[string][]string{"HTTP_FETCH_ALLOWED_HOSTS": p.AllowedHosts, "HTTP_FETCH_DENIED_HOSTS": p.DeniedHosts, "HTTP_FETCH_MUTATION_ALLOWED_HOSTS": p.MutationAllowedHosts} {
		for _, host := range hosts {
			if _, err := NormalizeHTTPFetchHost(host, true); err != nil {
				return fmt.Errorf("invalid %s", name)
			}
		}
	}
	if p.AllowMutations && len(p.MutationAllowedHosts) == 0 {
		return fmt.Errorf("HTTP_FETCH_MUTATION_ALLOWED_HOSTS required when mutations are enabled")
	}
	if p.MaxTimeoutMS <= 0 || int64(p.MaxTimeoutMS) > int64((1<<63-1)/time.Millisecond) {
		return fmt.Errorf("invalid HTTP_FETCH_MAX_TIMEOUT_MS")
	}
	maxInt := int(^uint(0) >> 1)
	if p.MaxResponseBytes <= 0 || p.MaxResponseBytes >= maxInt {
		return fmt.Errorf("invalid HTTP_FETCH_MAX_RESPONSE_BYTES")
	}
	if p.MaxTextBytes <= 0 || p.MaxTextBytes >= maxInt {
		return fmt.Errorf("invalid HTTP_FETCH_MAX_TEXT_BYTES")
	}
	if p.MaxRedirects < 0 {
		return fmt.Errorf("invalid HTTP_FETCH_MAX_REDIRECTS")
	}
	return nil
}

func loadHTTPFetchPolicy() (HTTPFetchPolicy, error) {
	p := DefaultHTTPFetchPolicy()
	for name, target := range map[string]*bool{"HTTP_FETCH_ENABLED": &p.Enabled, "HTTP_FETCH_ALLOW_MUTATIONS": &p.AllowMutations} {
		if value, ok := os.LookupEnv(name); ok { // Empty booleans/numbers are errors, not implicit defaults.
			if value != "true" && value != "false" {
				return p, fmt.Errorf("invalid %s: expected true or false", name)
			}
			*target = value == "true"
		}
	}
	for name, target := range map[string]*int{"HTTP_FETCH_MAX_TIMEOUT_MS": &p.MaxTimeoutMS, "HTTP_FETCH_MAX_RESPONSE_BYTES": &p.MaxResponseBytes, "HTTP_FETCH_MAX_TEXT_BYTES": &p.MaxTextBytes, "HTTP_FETCH_MAX_REDIRECTS": &p.MaxRedirects} {
		if value, ok := os.LookupEnv(name); ok {
			n, err := strconv.Atoi(value)
			if err != nil || strconv.Itoa(n) != value {
				return p, fmt.Errorf("invalid %s", name)
			}
			*target = n
		}
	}
	for name, target := range map[string]*[]string{"HTTP_FETCH_ALLOWED_HOSTS": &p.AllowedHosts, "HTTP_FETCH_DENIED_HOSTS": &p.DeniedHosts, "HTTP_FETCH_MUTATION_ALLOWED_HOSTS": &p.MutationAllowedHosts} {
		if value := os.Getenv(name); value != "" {
			for _, entry := range strings.Split(value, ",") {
				host, err := NormalizeHTTPFetchHost(entry, true)
				if err != nil {
					return p, fmt.Errorf("invalid %s", name)
				}
				*target = append(*target, host)
			}
		}
	}
	if value, ok := os.LookupEnv("HTTP_FETCH_ALLOWED_PORTS"); ok {
		p.AllowedPorts = nil
		for _, entry := range strings.Split(value, ",") {
			entry = strings.TrimSpace(entry)
			port, err := strconv.Atoi(entry)
			if err != nil || strconv.Itoa(port) != entry {
				return p, fmt.Errorf("invalid HTTP_FETCH_ALLOWED_PORTS")
			}
			p.AllowedPorts = append(p.AllowedPorts, port)
		}
	}
	return p, p.Validate()
}
