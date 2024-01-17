package api

import "github.com/angelofallars/htmx-go"

func setErrorMessage(message string) htmx.EventTrigger {
	return htmx.TriggerDetail("set-error-message", message)
}
