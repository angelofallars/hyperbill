package api

import (
	"net/http"

	"github.com/angelofallars/gotemplate/internal/header"
	invoiceview "github.com/angelofallars/gotemplate/view/invoice"
	"github.com/angelofallars/htmx-go"
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
				RenderTempl(r.Context(), w,
					invoiceview.SetErrorMessage(
						"To use this application, the Trello API key and token need to be supplied in the settings.",
					))
			return
		}

		f(w, r)
	}
}
