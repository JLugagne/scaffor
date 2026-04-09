# Contributing to joist

## Updating Documentation

This documentation is generated using `go-surgeon scaffold`.

### Regenerate after code changes

```bash
# Add a doc page for a new package
go-surgeon scaffold execute godoc-md add_package_doc \
  --set ProjectName="joist" \
  --set PackageName="<name>" \
  --set PackagePath="<path>"

# Then let an AI agent fill it:
# "Read the hint from the scaffold output and populate the doc page"
```

### How it works

1. `go-surgeon graph` lists all packages and symbols
2. `go-surgeon symbol <name> --body` extracts signatures and godoc
3. The scaffold template provides the structure
4. An AI agent (even Haiku) fills in the content

No manual writing needed — the documentation stays in sync with the code.
