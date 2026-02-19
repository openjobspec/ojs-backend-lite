package memory

import (
"github.com/openjobspec/ojs-backend-lite/internal/core"
)

var _ core.Backend = (*MemoryBackend)(nil)
