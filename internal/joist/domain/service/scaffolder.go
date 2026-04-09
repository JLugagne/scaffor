package service

import (
	"context"

	"github.com/JLugagne/joist/internal/joist/domain"
)

type ScaffolderCommands interface {
	ListTemplates(ctx context.Context) ([]domain.Template, error)
	GetTemplate(ctx context.Context, templateName string) (domain.Template, error)
	Execute(ctx context.Context, templateName, commandName string, params map[string]string, runCommands bool) error
	Lint(ctx context.Context, templateName string, templateDir string) []domain.LintError
}
