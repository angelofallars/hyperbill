package invoice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	"github.com/angelofallars/hyperbill/app/auth"
	"github.com/angelofallars/hyperbill/app/component"
	"github.com/angelofallars/hyperbill/app/event"
	"github.com/angelofallars/hyperbill/internal/service"
	"github.com/angelofallars/hyperbill/pkg/trello"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type HandlerGroup struct {
	svcInvoice service.Invoice
}

func NewHandlerGroup(svcInvoice service.Invoice) *HandlerGroup {
	return &HandlerGroup{
		svcInvoice: svcInvoice,
	}
}

func (hg *HandlerGroup) Mount(r chi.Router) {
	r.Handle("/", templ.Handler(component.FullPage("Trello Invoice Builder", page())))
	r.Get("/boards", auth.RequireTrelloCredentials(
		handleGetBoards(hg.svcInvoice)),
	)
	r.Post("/invoice", auth.RequireTrelloCredentials(
		handleCreateInvoice(hg.svcInvoice),
	))
}

func handleGetBoards(svc service.Invoice) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		credentials, err := auth.GetTrelloCredentials(r.Context())
		if err != nil {
			showError(w, http.StatusUnauthorized, err)
			return
		}

		client := trello.New(credentials.Key, credentials.Token)

		boards, err := svc.GetBoards(r.Context(), client)
		if err != nil {
			showError(w, http.StatusInternalServerError, err)
		}

		props := make([]BoardProps, 0, len(boards))
		for _, board := range boards {
			props = append(props, BoardProps{
				Name: board.Name,
				ID:   board.ID,
			})
		}

		_ = htmx.NewResponse().
			Retarget("#board-id").
			Reswap(htmx.SwapOuterHTML).
			Reselect("#board-id").
			AddTrigger(
				event.TriggerEnableSubmit,
				event.TriggerSetErrMessage(""),
			).
			RenderTempl(r.Context(), w, Boards(props))
	}
}

func handleCreateInvoice(svc service.Invoice) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := &CreateInvoiceRequest{}
		if err := render.Bind(r, req); err != nil {
			showError(w, http.StatusBadRequest, err)
			return
		}

		credentials, err := auth.GetTrelloCredentials(r.Context())
		if err != nil {
			showError(w, http.StatusUnauthorized, err)
			return
		}
		client := trello.New(credentials.Key, credentials.Token)

		invoice, err := svc.Create(r.Context(), client, service.CreateInvoiceRequest{
			TrelloBoardID: req.TrelloBoardID,
			StartDate:     req.StartDate,
			EndDate:       req.EndDate,
			T5Rate:        req.T5Rate,
			T4Rate:        req.T4Rate,
			T3Rate:        req.T3Rate,
			T2Rate:        req.T2Rate,
			T1Rate:        req.T1Rate,
		})
		if err != nil {
			showError(w, http.StatusInternalServerError, err)
		}

		clearError(w)

		_ = Invoice(invoice).Render(context.Background(), w)
	}
}

type CreateInvoiceRequest struct {
	TrelloBoardID string    `form:"board-id"`
	StartDate     time.Time `form:"start-date"`
	EndDate       time.Time `form:"end-date"`
	T5Rate        float64   `form:"t5"`
	T4Rate        float64   `form:"t4"`
	T3Rate        float64   `form:"t3"`
	T2Rate        float64   `form:"t2"`
	T1Rate        float64   `form:"t1"`
}

// createInvoiceRequest satisfies [render.Binder]
func (cir *CreateInvoiceRequest) Bind(r *http.Request) error {
	if matched, _ := regexp.MatchString("^[0-9a-fA-F]{24}$", cir.TrelloBoardID); !matched {
		return fmt.Errorf("Invalid trello board ID: %s", cir.TrelloBoardID)
	}

	if cir.StartDate.UnixMicro() >= cir.EndDate.UnixMicro() {
		return errors.New("Start date must be earlier than end date.")
	}

	if cir.T5Rate < 0 {
		return errors.New("T5 rate cannot be less than zero")
	}

	if cir.T4Rate < 0 {
		return errors.New("T4 rate cannot be less than zero")
	}
	if cir.T3Rate < 0 {
		return errors.New("T3 rate cannot be less than zero")
	}
	if cir.T2Rate < 0 {
		return errors.New("T2 rate cannot be less than zero")
	}
	if cir.T1Rate < 0 {
		return errors.New("T1 rate cannot be less than zero")
	}

	return nil
}

func showError(w http.ResponseWriter, code int, err error) {
	_ = htmx.NewResponse().
		StatusCode(code).
		Reswap(htmx.SwapNone).
		AddTrigger(event.TriggerSetErrMessage(err.Error())).
		Write(w)
}

func clearError(w http.ResponseWriter) {
	_ = htmx.NewResponse().
		AddTrigger(event.TriggerSetErrMessage("")).
		Write(w)
}
