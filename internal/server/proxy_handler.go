package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"

	"docker-socket-proxy/internal/logging"
	"docker-socket-proxy/internal/proxy/config"
)

type ProxyHandler struct {
	dockerSocket string
	configs      map[string]*config.SocketConfig
	configMu     *sync.RWMutex
}

func NewProxyHandler(dockerSocket string, configs map[string]*config.SocketConfig, mu *sync.RWMutex) *ProxyHandler {
	return &ProxyHandler{
		dockerSocket: dockerSocket,
		configs:      configs,
		configMu:     mu,
	}
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, socketPath string) {
	log := logging.GetLogger()

	if allowed, reason := h.checkACLRules(socketPath, r); !allowed {
		log.Info("Request denied by ACL",
			"method", r.Method,
			"path", r.URL.Path,
			"reason", reason)
		http.Error(w, reason, http.StatusForbidden)
		return
	}

	h.configMu.RLock()
	config := h.configs[socketPath]
	h.configMu.RUnlock()

	// Apply both propagation and rewrite rules
	if config != nil {
		// First apply propagation rules
		if rules := config.GetPropagationRules(); len(rules) > 0 {
			if err := h.applyRules(r, rules); err != nil {
				log.Error("Failed to apply socket propagation rules",
					"error", err,
					"path", r.URL.Path)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		// Then apply regular rewrite rules
		if len(config.Rules.Rewrites) > 0 {
			if err := h.applyRules(r, config.Rules.Rewrites); err != nil {
				log.Error("Failed to apply rewrite rules",
					"error", err,
					"path", r.URL.Path)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "unix-socket"
		},
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", h.dockerSocket)
			},
		},
	}

	proxy.ServeHTTP(w, r)
}

func (h *ProxyHandler) checkACLRules(socketPath string, r *http.Request) (bool, string) {
	h.configMu.RLock()
	config, exists := h.configs[socketPath]
	h.configMu.RUnlock()

	if !exists || config == nil {
		return true, "" // No config
	}

	// Check each rule
	for _, rule := range config.Rules.ACLs {
		if matchesRule(r, rule.Match) {
			if rule.Action == "deny" {
				return false, rule.Reason
			}
		}
	}

	return true, ""
}

// applyRules applies the given rules to the request
func (h *ProxyHandler) applyRules(r *http.Request, rules []config.RewriteRule) error {
	log := logging.GetLogger()

	// Only handle JSON content
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		log.Debug("Skipping non-JSON request",
			"content-type", r.Header.Get("Content-Type"),
			"path", r.URL.Path)
		return nil
	}

	// Skip if no body
	if r.Body == nil || r.ContentLength == 0 {
		log.Debug("Skipping empty body request",
			"path", r.URL.Path)
		return nil
	}

	// Read and parse body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var requestBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &requestBody); err != nil {
		log.Debug("Failed to parse JSON body",
			"error", err,
			"path", r.URL.Path)
		return err
	}

	log.Debug("Processing rules",
		"path", r.URL.Path,
		"method", r.Method,
		"rules_count", len(rules))

	modified := false
	for _, rule := range rules {
		if matchesRule(r, rule.Match) {
			log.Debug("Rule matched",
				"path_pattern", rule.Match.Path,
				"method", rule.Match.Method)

			for _, pattern := range rule.Patterns {
				if rewritten := applyPatternToRequest(requestBody, pattern); rewritten {
					modified = true
				}
			}
		} else {
			log.Debug("Rule did not match",
				"path_pattern", rule.Match.Path,
				"method", rule.Match.Method,
				"actual_path", r.URL.Path,
				"actual_method", r.Method)
		}
	}

	// If modified, update request body
	if modified {
		newBody, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		r.Body = io.NopCloser(bytes.NewBuffer(newBody))
		r.ContentLength = int64(len(newBody))
		log.Debug("Request body modified",
			"new_body", string(newBody))
	} else {
		log.Debug("No modifications made to request body")
	}

	return nil
}

func applyPatternToRequest(data map[string]interface{}, pattern config.Pattern) bool {
	log := logging.GetLogger()
	parts := strings.Split(pattern.Field, ".")

	// Navigate to the parent object
	current := data
	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[parts[i]] = next
		}
		current = next
	}

	lastPart := parts[len(parts)-1]
	switch pattern.Action {
	case "upsert":
		switch v := pattern.Value.(type) {
		case []interface{}:
			existing, ok := current[lastPart].([]interface{})
			if !ok {
				current[lastPart] = v
			} else {
				current[lastPart] = append(existing, v...)
			}
		case string:
			existing, ok := current[lastPart].([]interface{})
			if !ok {
				current[lastPart] = []interface{}{v}
			} else {
				current[lastPart] = append(existing, v)
			}
		default:
			current[lastPart] = pattern.Value
		}
		log.Debug("Upsert operation completed",
			"field", pattern.Field,
			"value", pattern.Value,
			"result", current[lastPart])
		return true
	}
	return false
}
