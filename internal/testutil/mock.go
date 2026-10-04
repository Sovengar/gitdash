package testutil

import (
	"fmt"
	"strings"
	"testing"
)

// A double that records failures instead of killing the test: against a real *testing.T the helpers' error branches are unreachable, because the covering test would kill itself first.

type MockTB struct {
	Failures []string
	Temp     testing.TB
}

// Helper is a no-op: the double takes no part in a real test's line attribution, so calling it would point the helper's stack trace here.
func (m *MockTB) Helper() {}

func (m *MockTB) Fatal(args ...any) {
	m.Failures = append(m.Failures, strings.TrimSpace(fmt.Sprint(args...)))
}

func (m *MockTB) Fatalf(format string, args ...any) {
	m.Failures = append(m.Failures, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

// With no test to delegate to there is no temp dir to return, and the helpers using it would fail for another reason.
func (m *MockTB) TempDir() string {
	if m.Temp == nil {
		return ""
	}
	return m.Temp.TempDir()
}

func (m *MockTB) Failed() bool { return len(m.Failures) > 0 }
