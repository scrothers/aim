package httpapi

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// OpenAPI returns the OpenAPI 3.1 document of the API as JSON without serving
// anything.
func OpenAPI(version string) ([]byte, error) {
	api := humachi.New(chi.NewRouter(), config(version))
	register(api, Deps{Version: version})
	b, err := api.OpenAPI().MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("httpapi: marshal openapi: %w", err)
	}
	return b, nil
}
