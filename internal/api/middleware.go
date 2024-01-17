package api

import (
	"net/http"

	"github.com/angelofallars/htmx-go"
	"github.com/angelofallars/hyperbill/internal/header"
)

func authRequired(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trelloAPIKey := r.Header.Get(header.TrelloAuthKey)
		trelloAPIToken := r.Header.Get(header.TrelloAuthToken)

		if trelloAPIKey == "" || trelloAPIToken == "" {
			_ = htmx.NewResponse().
				StatusCode(http.StatusUnauthorized).
				Reswap(htmx.SwapNone).
				AddTrigger(htmx.Trigger("disable-submit")).
				AddTrigger(htmx.Trigger("open-settings")).
				AddTrigger(
					setErrorMessage("To use this application, the Trello API key and token need to be supplied in the settings."),
				).
				Write(w)
			return
		}

		f(w, r)
	}
}
