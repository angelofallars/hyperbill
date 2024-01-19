package route

import "github.com/go-chi/chi/v5"

type Mounter interface {
	Mount(r chi.Router)
}
