package httpapi

import (
	"io/fs"

	"github.com/go-chi/chi/v5"
)

func mountStatic(_ chi.Router, _ fs.FS) {}
