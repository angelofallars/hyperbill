// Package event provides definitions for global DOM
// events that are dispatched by the `HX-Trigger`
// header in HTMX requests.
package event

import (
	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
)

// Event is a client-side event that can be triggered
// on the server.
//
// Event names should be snake-case so Alpine.js
// can parse them correctly.
type Event string

// Event satisfies [fmt.Stringer]
func (e Event) String() string { return string(e) }

// Listen returns an Alpine.js x-on attribute with
// the provided JavaScript callback text.
//
// Format:
//
//	x-on:<eventName>.window="<code>"
func (e Event) Listen(jsCode string) templ.Attributes {
	return templ.Attributes{
		"x-on:" + string(e) + ".window": jsCode,
	}
}

const SetErrMessage Event = "set-err-message"

func TriggerSetErrMessage(message string) htmx.EventTrigger {
	return htmx.TriggerDetail(SetErrMessage.String(), message)
}

const DisableSubmit Event = "disable-submit"

var TriggerDisableSubmit = htmx.Trigger(DisableSubmit.String())

const EnableSubmit Event = "enable-submit"

var TriggerEnableSubmit = htmx.Trigger(EnableSubmit.String())

const OpenSettings Event = "open-settings"

var TriggerOpenSettings = htmx.Trigger(OpenSettings.String())
