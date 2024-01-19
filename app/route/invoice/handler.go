package invoice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/angelofallars/htmx-go"
	"github.com/angelofallars/hyperbill/app/auth"
	"github.com/angelofallars/hyperbill/app/component"
	"github.com/angelofallars/hyperbill/app/event"
	"github.com/angelofallars/hyperbill/internal/domain"
	"github.com/angelofallars/hyperbill/pkg/trello"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

type HandlerGroup struct{}

func NewHandlerGroup() *HandlerGroup {
	return &HandlerGroup{}
}

func (hg *HandlerGroup) Mount(r chi.Router) {
	r.Handle("/", templ.Handler(component.FullPage("Trello Invoice Builder", page())))
	r.Get("/boards", auth.RequireTrelloCredentials(handleGetBoards))
	r.Post("/invoice", auth.RequireTrelloCredentials(handleCreateInvoice))
}

func handleGetBoards(w http.ResponseWriter, r *http.Request) {
	credentials, err := auth.GetTrelloCredentials(r.Context())
	if err != nil {
		showError(w, http.StatusUnauthorized, err)
		return
	}

	client := trello.New(credentials.Key, credentials.Token)

	boards, err := client.GetBoards()
	if err != nil {
		hasInvalidKey := errors.Is(err, trello.ErrInvalidKey)
		hasInvalidToken := errors.Is(err, trello.ErrInvalidToken)
		shouldDisableSubmit := hasInvalidKey || hasInvalidToken

		resp := htmx.NewResponse().Reswap(htmx.SwapNone)

		if shouldDisableSubmit {
			var errMessage string
			switch {
			case hasInvalidKey:
				errMessage = "Invalid Trello API key. Make sure it is correct and try again."
			case hasInvalidToken:
				errMessage = "Invalid Trello API token. Make sure it is correct and try again."
			}

			_ = resp.
				StatusCode(http.StatusUnauthorized).
				AddTrigger(
					event.TriggerDisableSubmit,
					event.TriggerSetErrMessage(errMessage),
				).
				Write(w)
			return
		}

		_ = resp.
			StatusCode(http.StatusInternalServerError).
			AddTrigger(event.TriggerSetErrMessage(err.Error())).
			Write(w)
		return
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

func handleCreateInvoice(w http.ResponseWriter, r *http.Request) {
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

	invoice, err := createInvoice(client, req)
	if err != nil {
		showError(w, http.StatusInternalServerError, err)
	}

	clearError(w)

	_ = Invoice(invoice).Render(context.Background(), w)
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

func createInvoice(client *trello.Client, req *CreateInvoiceRequest) (*domain.Invoice, error) {
	type inProgressSession struct {
		startDate time.Time
		duration  time.Duration
	}

	type cardHistory struct {
		Category           domain.Category
		InProgressSessions []inProgressSession
	}

	cards, err := client.GetCards(req.TrelloBoardID)
	if err != nil {
		return nil, err
	}

	cardHistories := []cardHistory{}

	for _, card := range cards {
		actions, err := client.GetCardActions(card.ID)
		if err != nil {
			return nil, err
		}

		slices.SortFunc(actions, func(a trello.Action, b trello.Action) int {
			aDate, _ := time.Parse(time.RFC3339Nano, a.Date)
			bDate, _ := time.Parse(time.RFC3339Nano, b.Date)
			if aDate.Unix() > bDate.Unix() {
				return 1
			} else if aDate.Unix() < bDate.Unix() {
				return -1
			} else {
				return 0
			}
		})

		inProgressSessions := []inProgressSession{}

		inProgress := false
		var inProgressStart time.Time
		var actionDate time.Time

		for _, action := range actions {
			actionDate, err = time.Parse(time.RFC3339Nano, action.Date)
			if err != nil {
				return nil, err
			}

			unixDate := actionDate.UnixNano()
			if unixDate < req.StartDate.UnixNano() || unixDate > req.EndDate.UnixNano() {
				continue
			}

			switch action.Type {
			case "updateCard":
				listBefore := action.Data["listBefore"].(map[string]any)["name"].(string)
				listAfter := action.Data["listAfter"].(map[string]any)["name"].(string)
				if !strings.Contains(listBefore, "(IP)") && strings.Contains(listAfter, "(IP)") {
					inProgress = true
					inProgressStart = actionDate
				} else if strings.Contains(listBefore, "(IP)") && !strings.Contains(listAfter, "(IP)") {
					inProgress = false
					inProgressSessions = append(inProgressSessions, inProgressSession{
						startDate: inProgressStart,
						duration:  actionDate.Sub(inProgressStart),
					})
				}
			}
		}

		if inProgress {
			inProgressSessions = append(inProgressSessions, inProgressSession{
				startDate: inProgressStart,
				duration:  actionDate.Sub(inProgressStart),
			})
		}

		var category domain.Category
		switch {
		case slices.Contains(card.Labels, "T5"):
			category = domain.CategoryT5
		case slices.Contains(card.Labels, "T4"):
			category = domain.CategoryT4
		case slices.Contains(card.Labels, "T3"):
			category = domain.CategoryT3
		case slices.Contains(card.Labels, "T2"):
			category = domain.CategoryT2
		case slices.Contains(card.Labels, "T1"):
			category = domain.CategoryT1
		default:
			category = domain.CategoryT1
		}

		cardHistories = append(cardHistories, cardHistory{
			Category:           category,
			InProgressSessions: inProgressSessions,
		})
	}

	inv := &domain.Invoice{
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		T5Report: domain.CategoryReport{
			PricePerHour: req.T5Rate,
		},
		T4Report: domain.CategoryReport{
			PricePerHour: req.T4Rate,
		},
		T3Report: domain.CategoryReport{
			PricePerHour: req.T3Rate,
		},
		T2Report: domain.CategoryReport{
			PricePerHour: req.T2Rate,
		},
		T1Report: domain.CategoryReport{
			PricePerHour: req.T1Rate,
		},
	}

	for _, cardHistory := range cardHistories {
		var timeSpent time.Duration
		for _, session := range cardHistory.InProgressSessions {
			timeSpent += session.duration
		}
		switch cardHistory.Category {
		case domain.CategoryT5:
			inv.T5Report.TimeSpent += timeSpent
		case domain.CategoryT4:
			inv.T4Report.TimeSpent += timeSpent
		case domain.CategoryT3:
			inv.T3Report.TimeSpent += timeSpent
		case domain.CategoryT2:
			inv.T2Report.TimeSpent += timeSpent
		case domain.CategoryT1:
			inv.T1Report.TimeSpent += timeSpent
		}
	}

	inv.T5Report.Price = inv.T5Report.TimeSpent.Hours() * inv.T5Report.PricePerHour
	inv.T4Report.Price = inv.T4Report.TimeSpent.Hours() * inv.T4Report.PricePerHour
	inv.T3Report.Price = inv.T3Report.TimeSpent.Hours() * inv.T3Report.PricePerHour
	inv.T2Report.Price = inv.T2Report.TimeSpent.Hours() * inv.T2Report.PricePerHour
	inv.T1Report.Price = inv.T1Report.TimeSpent.Hours() * inv.T1Report.PricePerHour

	inv.TotalPrice = inv.T5Report.Price + inv.T4Report.Price + inv.T3Report.Price + inv.T2Report.Price + inv.T1Report.Price

	return inv, nil
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
