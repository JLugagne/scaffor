package domain

// ExecuteOptions controls file-conflict behaviour during scaffolding.
// ExecuteOptions controls file-conflict behaviour during scaffolding.
type ExecuteOptions struct {
	// DryRun prints shell_commands instead of executing them.
	DryRun bool
	// Skip silently skips files that already exist instead of failing.
	Skip bool
	// Force overwrites files that already exist instead of failing.
	Force bool
}

// FileEvent records what happened to a single file during execution.
type FileEvent struct {
	Path   string // target file path
	Action string // "created", "overwritten", "skipped"
}
