package invoice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
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

type cardHistory struct {
	Category           domain.Category
	InProgressSessions []inProgressSession
}

type inProgressSession struct {
	startDate time.Time
	duration  time.Duration
}

func handleCreateInvoice(w http.ResponseWriter, r *http.Request) {
	// TODO: Heavily refactor this handler by extracting parts into services
	err := r.ParseForm()
	if err != nil {
		showError(w, http.StatusBadRequest, err)
		return
	}

	req, err := newCreateInvoiceRequest(r.Form)
	if err != nil {
		showError(w, http.StatusBadRequest, err)
		return
	}

	credentials, err := auth.GetTrelloCredentials(r.Context())
	if err != nil {
		showError(w, http.StatusUnauthorized, err)
		return
	}
	client := trello.New(credentials.Key, credentials.Token)

	cards, err := client.GetCards(req.trelloBoardID)
	if err != nil {
		showError(w, http.StatusInternalServerError, err)
		return
	}

	cardHistories := []cardHistory{}

	for _, card := range cards {
		actions, err := client.GetCardActions(card.ID)
		if err != nil {
			showError(w, http.StatusInternalServerError, err)
			return
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
				showError(w, http.StatusInternalServerError, err)
				return
			}

			unixDate := actionDate.UnixNano()
			if unixDate < req.startDate.UnixNano() || unixDate > req.endDate.UnixNano() {
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

	inv := domain.Invoice{
		StartDate: req.startDate,
		EndDate:   req.endDate,
		T5Report: domain.CategoryReport{
			PricePerHour: req.t5Rate,
		},
		T4Report: domain.CategoryReport{
			PricePerHour: req.t4Rate,
		},
		T3Report: domain.CategoryReport{
			PricePerHour: req.t3Rate,
		},
		T2Report: domain.CategoryReport{
			PricePerHour: req.t2Rate,
		},
		T1Report: domain.CategoryReport{
			PricePerHour: req.t1Rate,
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

	clearError(w)

	_ = Invoice(inv).Render(context.Background(), w)
}

type createInvoiceRequest struct {
	trelloBoardID string
	startDate     time.Time
	endDate       time.Time
	t5Rate        float64
	t4Rate        float64
	t3Rate        float64
	t2Rate        float64
	t1Rate        float64
}

func newCreateInvoiceRequest(form url.Values) (*createInvoiceRequest, error) {
	trelloBoardID := form.Get("board-id")
	startDateString := form.Get("start-date")
	endDateString := form.Get("end-date")
	t5RateString := form.Get("t5")
	t4RateString := form.Get("t4")
	t3RateString := form.Get("t3")
	t2RateString := form.Get("t2")
	t1RateString := form.Get("t1")

	if matched, _ := regexp.MatchString("^[0-9a-fA-F]{24}$", trelloBoardID); !matched {
		return nil, fmt.Errorf("Invalid trello board ID: %s", trelloBoardID)
	}

	startDate, err := time.Parse(time.DateOnly, startDateString)
	if err != nil {
		return nil, fmt.Errorf("Parsing start date failed: %w", err)
	}

	endDate, err := time.Parse(time.DateOnly, endDateString)
	if err != nil {
		return nil, fmt.Errorf("Parsing end date failed: %w", err)
	}

	if startDate.UnixMicro() >= endDate.UnixMicro() {
		return nil, errors.New("Start date must be earlier than end date.")
	}

	t5Rate, err := strconv.ParseFloat(t5RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T5 rate failed: %w", err)
	}
	if t5Rate < 0 {
		return nil, errors.New("T5 rate cannot be less than zero")
	}

	t4Rate, err := strconv.ParseFloat(t4RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T4 rate failed: %w", err)
	}
	if t4Rate < 0 {
		return nil, errors.New("T4 rate cannot be less than zero")
	}

	t3Rate, err := strconv.ParseFloat(t3RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T3 rate failed: %w", err)
	}
	if t3Rate < 0 {
		return nil, errors.New("T3 rate cannot be less than zero")
	}

	t2Rate, err := strconv.ParseFloat(t2RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T2 rate failed: %w", err)
	}
	if t2Rate < 0 {
		return nil, errors.New("T2 rate cannot be less than zero")
	}

	t1Rate, err := strconv.ParseFloat(t1RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T1 rate failed: %w", err)
	}
	if t1Rate < 0 {
		return nil, errors.New("T1 rate cannot be less than zero")
	}

	return &createInvoiceRequest{
		trelloBoardID: trelloBoardID,
		startDate:     startDate,
		endDate:       endDate,
		t5Rate:        t5Rate,
		t4Rate:        t4Rate,
		t3Rate:        t3Rate,
		t2Rate:        t2Rate,
		t1Rate:        t1Rate,
	}, nil
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
