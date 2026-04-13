package domain_test

import (
	"testing"

	"github.com/JLugagne/scaffor/internal/scaffor/domain"
	"github.com/stretchr/testify/assert"
)

func TestLintError_Error(t *testing.T) {
	tests := []struct {
		name string
		e    domain.LintError
		want string
	}{
		{
			name: "with command",
			e:    domain.LintError{Command: "bootstrap", Field: "variables", Message: `variable "appName" must start with a capital letter`},
			want: `command "bootstrap", field "variables": variable "appName" must start with a capital letter`,
		},
		{
			name: "without command",
			e:    domain.LintError{Field: "manifest", Message: "file not found"},
			want: `field "manifest": file not found`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.e.Error())
		})
	}
}
