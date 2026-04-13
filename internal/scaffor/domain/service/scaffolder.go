package service

import (
	"context"

	"github.com/JLugagne/scaffor/internal/scaffor/domain"
)

type ScaffolderCommands interface {
	ListTemplates(ctx context.Context) ([]domain.Template, error)
	GetTemplate(ctx context.Context, templateName string) (domain.Template, error)
	Execute(ctx context.Context, templateName, commandName string, params map[string]string, opts domain.ExecuteOptions) ([]domain.FileEvent, error)
	Lint(ctx context.Context, templateName string, templateDir string) []domain.LintError
	Test(ctx context.Context, templateName string) error
}
