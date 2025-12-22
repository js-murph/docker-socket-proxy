package matcher

import (
	"docker-socket-proxy/internal/domain"
	"testing"
)

func TestUnit_StringMatcher_GivenExactMatch_WhenMatching_ThenReturnsTrue(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		input    string
		expected bool
	}{
		{"exact match", "test", "test", true},
		{"no match", "test", "other", false},
		{"empty pattern", "", "test", true},
		{"empty input", "test", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewStringMatcher(tt.pattern)
			result := matcher.Matches(tt.input)
			if result != tt.expected {
				t.Errorf("StringMatcher.Matches() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_StringMatcher_GivenRegexPattern_WhenMatching_ThenReturnsExpected(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		input    string
		expected bool
	}{
		{"wildcard pattern", ".*", "anything", true},
		{"simple wildcard", "test.*", "test123", true},
		{"no match wildcard", "test.*", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewStringMatcher(tt.pattern)
			result := matcher.Matches(tt.input)
			if result != tt.expected {
				t.Errorf("StringMatcher.Matches() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_ArrayMatcher_GivenValidArray_WhenMatching_ThenReturnsTrue(t *testing.T) {
	tests := []struct {
		name     string
		expected []interface{}
		input    []interface{}
		want     bool
	}{
		{
			"exact match",
			[]interface{}{"item1", "item2"},
			[]interface{}{"item1", "item2"},
			true,
		},
		{
			"subset match",
			[]interface{}{"item1"},
			[]interface{}{"item1", "item2"},
			true,
		},
		{
			"no match",
			[]interface{}{"item3"},
			[]interface{}{"item1", "item2"},
			false,
		},
		{
			"empty expected",
			[]interface{}{},
			[]interface{}{"item1", "item2"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewArrayMatcher(tt.expected)
			result := matcher.Matches(tt.input)
			if result != tt.want {
				t.Errorf("ArrayMatcher.Matches() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestUnit_ObjectMatcher_GivenValidObject_WhenMatching_ThenReturnsTrue(t *testing.T) {
	tests := []struct {
		name     string
		expected map[string]interface{}
		input    map[string]interface{}
		want     bool
	}{
		{
			"exact match",
			map[string]interface{}{"key1": "value1", "key2": "value2"},
			map[string]interface{}{"key1": "value1", "key2": "value2"},
			true,
		},
		{
			"subset match",
			map[string]interface{}{"key1": "value1"},
			map[string]interface{}{"key1": "value1", "key2": "value2"},
			true,
		},
		{
			"no match",
			map[string]interface{}{"key3": "value3"},
			map[string]interface{}{"key1": "value1", "key2": "value2"},
			false,
		},
		{
			"empty expected",
			map[string]interface{}{},
			map[string]interface{}{"key1": "value1"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewObjectMatcher(tt.expected)
			result := matcher.Matches(tt.input)
			if result != tt.want {
				t.Errorf("ObjectMatcher.Matches() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestUnit_PathMatcher_GivenValidPath_WhenMatching_ThenReturnsTrue(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		path     string
		expected bool
	}{
		{"exact match", "/v1.42/containers/json", "/v1.42/containers/json", true},
		{"wildcard match", "/v1.*/containers/.*", "/v1.42/containers/json", true},
		{"no match", "/v1.*/networks/.*", "/v1.42/containers/json", false},
		{"empty pattern", "", "/v1.42/containers/json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewPathMatcher(tt.pattern)
			req := domain.Request{Path: tt.path}
			result := matcher.MatchesRequest(req)
			if result != tt.expected {
				t.Errorf("PathMatcher.MatchesRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_MethodMatcher_GivenValidMethod_WhenMatching_ThenReturnsTrue(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		method   string
		expected bool
	}{
		{"exact match", "GET", "GET", true},
		{"wildcard match", "GET|POST", "GET", true},
		{"no match", "POST", "GET", false},
		{"empty pattern", "", "GET", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewMethodMatcher(tt.pattern)
			req := domain.Request{Method: tt.method}
			result := matcher.MatchesRequest(req)
			if result != tt.expected {
				t.Errorf("MethodMatcher.MatchesRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUnit_BodyMatcher_GivenValidBody_WhenMatching_ThenReturnsTrue(t *testing.T) {
	tests := []struct {
		name     string
		expected map[string]interface{}
		body     map[string]interface{}
		want     bool
	}{
		{
			"exact match",
			map[string]interface{}{"key1": "value1"},
			map[string]interface{}{"key1": "value1", "key2": "value2"},
			true,
		},
		{
			"no match",
			map[string]interface{}{"key3": "value3"},
			map[string]interface{}{"key1": "value1", "key2": "value2"},
			false,
		},
		{
			"empty expected",
			map[string]interface{}{},
			map[string]interface{}{"key1": "value1"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewBodyMatcher(tt.expected)
			req := domain.Request{Body: tt.body}
			result := matcher.MatchesRequest(req)
			if result != tt.want {
				t.Errorf("BodyMatcher.MatchesRequest() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestUnit_CompositeRequestMatcher_GivenMultipleMatchers_WhenMatching_ThenReturnsExpected(t *testing.T) {
	tests := []struct {
		name     string
		matchers []RequestMatcher
		req      domain.Request
		expected bool
	}{
		{
			"all match",
			[]RequestMatcher{
				NewPathMatcher("/v1.*/containers/.*"),
				NewMethodMatcher("GET"),
			},
			domain.Request{Path: "/v1.42/containers/json", Method: "GET"},
			true,
		},
		{
			"path matches, method doesn't",
			[]RequestMatcher{
				NewPathMatcher("/v1.*/containers/.*"),
				NewMethodMatcher("POST"),
			},
			domain.Request{Path: "/v1.42/containers/json", Method: "GET"},
			false,
		},
		{
			"no matchers",
			[]RequestMatcher{},
			domain.Request{Path: "/v1.42/containers/json", Method: "GET"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := NewCompositeRequestMatcher(tt.matchers...)
			result := matcher.MatchesRequest(tt.req)
			if result != tt.expected {
				t.Errorf("CompositeRequestMatcher.MatchesRequest() = %v, want %v", result, tt.expected)
			}
		})
	}
}
