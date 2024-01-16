package api

import (
	"fmt"
	"net/http"

	"github.com/angelofallars/gotemplate/view/component"
	"github.com/angelofallars/htmx-go"
)

func authRequired(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trelloAPIKey := r.Header.Get("X-Trello-Api-Key")
		trelloAPIToken := r.Header.Get("X-Trello-Api-Token")

		if trelloAPIKey == "" || trelloAPIToken == "" {
			component.RenderInfo(w, http.StatusUnauthorized, fmt.Errorf("To use this application, the Trello API key and token need to be supplied in the settings."), func(r htmx.Response) htmx.Response {
				return r.AddTrigger(htmx.Trigger("open-settings"))
			})
			return
		}

		f(w, r)
	}
}
