package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/angelofallars/htmx-go"
	"github.com/angelofallars/hyperbill/app/event"
	"github.com/angelofallars/hyperbill/app/header"
)

func RequireTrelloCredentials(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trelloAPIKey := r.Header.Get(header.TrelloAuthKey)
		trelloAPIToken := r.Header.Get(header.TrelloAuthToken)

		if trelloAPIKey == "" || trelloAPIToken == "" {
			_ = htmx.NewResponse().
				StatusCode(http.StatusUnauthorized).
				Reswap(htmx.SwapNone).
				AddTrigger(
					event.TriggerDisableSubmit,
					event.TriggerOpenSettings,
					event.TriggerSetErrMessage(
						"To use this application, the Trello API key and token need to be supplied in the settings.",
					),
				).
				Write(w)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), authKey,
			&TrelloCredentials{Key: trelloAPIKey, Token: trelloAPIToken},
		))

		f(w, r)
	}
}

func GetTrelloCredentials(c context.Context) (*TrelloCredentials, error) {
	credentials, ok := c.Value(authKey).(*TrelloCredentials)
	if !ok {
		return nil, errors.New("Trello credentials not found")
	}
	return credentials, nil
}

type TrelloCredentials struct {
	Key   string
	Token string
}

type key struct{}

var authKey = key{}
