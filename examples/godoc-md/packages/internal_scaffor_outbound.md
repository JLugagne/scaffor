# internal/scaffor/outbound

Driven adapters that implement persistence and external integrations.

## Overview

The `outbound` package contains adapters that provide implementations for the interfaces defined in the domain layer. It handles interactions with external systems like the filesystem.

## Subpackages

- `filesystem` — Filesystem adapter for reading/writing files

## Architecture

Outbound adapters are responsible for:
1. Implementing domain repository interfaces
2. Handling file I/O, network calls, or database operations
3. Error translation between external systems and domain concepts
4. Resource lifecycle management (cleanup, connections, etc.)

They should be the only place where we directly interact with external systems. The rest of the application code should only care about domain interfaces, not concrete implementations.
