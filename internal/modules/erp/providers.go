package erp

// Linking the providers into the registry happens through this blank import:
// the package (and everything that imports it) sees one registered provider
// per ERP system linked here (trap 28: the registration line lives in
// provider/all, the DefaultProvider constant in provider/registry.go).
import (
	_ "github.com/moqasem061-dot/dawa24/internal/modules/erp/provider/all"
)
