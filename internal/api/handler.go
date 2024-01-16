package api

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

	"github.com/go-chi/chi/v5/middleware"

	"github.com/angelofallars/gotemplate/internal/invoice"
	"github.com/angelofallars/gotemplate/pkg/trello"
	"github.com/angelofallars/gotemplate/view/component"
	invoiceview "github.com/angelofallars/gotemplate/view/invoice"
	"github.com/angelofallars/htmx-go"
)

func (a *API) RegisterRoutes() {
	a.router.Use(middleware.Logger)

	a.router.Get("/", handleIndex())
	a.router.Get("/boards", authRequired(handleGetBoards()))
	a.router.Post("/invoice", authRequired(handleCreateInvoice()))

	a.router.Handle("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.Dir("view/assets/"))))
}

func handleIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = invoiceview.Index().Render(context.Background(), w)
	}
}

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

func handleGetBoards() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trelloAPIKey := r.Header.Get("X-Trello-Api-Key")
		trelloAPIToken := r.Header.Get("X-Trello-Api-Token")
		client := trello.New(trelloAPIKey, trelloAPIToken)

		boards, err := client.GetBoards()
		if err != nil {
			// Disable submit by dispatching an event for Alpine.js
			shouldDisableSubmit := errors.Is(err, trello.ErrInvalidKey) || errors.Is(err, trello.ErrInvalidToken)
			if shouldDisableSubmit {
				component.RenderError(w, http.StatusUnauthorized, err, func(r htmx.Response) htmx.Response {
					return r.AddTrigger(htmx.Trigger("disable-submit"))
				})
			} else {
				component.RenderError(w, http.StatusInternalServerError, err)
			}
			return
		}

		props := make([]invoiceview.BoardProps, 0, len(boards))
		for _, board := range boards {
			props = append(props, invoiceview.BoardProps{
				Name: board.Name,
				ID:   board.ID,
			})
		}

		_ = htmx.NewResponse().
			Retarget("#board-id").
			Reswap(htmx.SwapOuterHTML).
			Reselect("#board-id").
			AddTrigger(htmx.Trigger("enable-submit")).
			RenderTempl(r.Context(), w, invoiceview.Boards(props))
	}
}

type cardHistory struct {
	Category           invoice.Category
	InProgressSessions []inProgressSession
}

type inProgressSession struct {
	startDate time.Time
	duration  time.Duration
}

func handleCreateInvoice() http.HandlerFunc {
	// TODO: Heavily refactor this handler by extracting parts into services
	return func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseForm()
		if err != nil {
			component.RenderError(w, http.StatusBadRequest, err)
			return
		}

		req, err := newCreateInvoiceRequest(r.Form)
		if err != nil {
			component.RenderError(w, http.StatusBadRequest, err)
			return
		}

		trelloAPIKey := r.Header.Get("X-Trello-Api-Key")
		trelloAPIToken := r.Header.Get("X-Trello-Api-Token")
		client := trello.New(trelloAPIKey, trelloAPIToken)

		cards, err := client.GetCards(req.trelloBoardID)
		if err != nil {
			component.RenderError(w, http.StatusInternalServerError, err)
			return
		}

		cardHistories := []cardHistory{}

		for _, card := range cards {
			actions, err := client.GetCardActions(card.ID)
			if err != nil {
				component.RenderError(w, http.StatusInternalServerError, err)
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
					component.RenderError(w, http.StatusInternalServerError, err)
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

			var category invoice.Category
			switch {
			case slices.Contains(card.Labels, "T5"):
				category = invoice.CategoryT5
			case slices.Contains(card.Labels, "T4"):
				category = invoice.CategoryT4
			case slices.Contains(card.Labels, "T3"):
				category = invoice.CategoryT3
			case slices.Contains(card.Labels, "T2"):
				category = invoice.CategoryT2
			case slices.Contains(card.Labels, "T1"):
				category = invoice.CategoryT1
			default:
				category = invoice.CategoryT1
			}

			cardHistories = append(cardHistories, cardHistory{
				Category:           category,
				InProgressSessions: inProgressSessions,
			})
		}

		inv := invoice.Invoice{
			StartDate: req.startDate,
			EndDate:   req.endDate,
			T5Report: invoice.CategoryReport{
				PricePerHour: req.t5Rate,
			},
			T4Report: invoice.CategoryReport{
				PricePerHour: req.t4Rate,
			},
			T3Report: invoice.CategoryReport{
				PricePerHour: req.t3Rate,
			},
			T2Report: invoice.CategoryReport{
				PricePerHour: req.t2Rate,
			},
			T1Report: invoice.CategoryReport{
				PricePerHour: req.t1Rate,
			},
		}

		for _, cardHistory := range cardHistories {
			var timeSpent time.Duration
			for _, session := range cardHistory.InProgressSessions {
				timeSpent += session.duration
			}
			switch cardHistory.Category {
			case invoice.CategoryT5:
				inv.T5Report.TimeSpent += timeSpent
			case invoice.CategoryT4:
				inv.T4Report.TimeSpent += timeSpent
			case invoice.CategoryT3:
				inv.T3Report.TimeSpent += timeSpent
			case invoice.CategoryT2:
				inv.T2Report.TimeSpent += timeSpent
			case invoice.CategoryT1:
				inv.T1Report.TimeSpent += timeSpent
			}
		}

		inv.T5Report.Price = inv.T5Report.TimeSpent.Hours() * inv.T5Report.PricePerHour
		inv.T4Report.Price = inv.T4Report.TimeSpent.Hours() * inv.T4Report.PricePerHour
		inv.T3Report.Price = inv.T3Report.TimeSpent.Hours() * inv.T3Report.PricePerHour
		inv.T2Report.Price = inv.T2Report.TimeSpent.Hours() * inv.T2Report.PricePerHour
		inv.T1Report.Price = inv.T1Report.TimeSpent.Hours() * inv.T1Report.PricePerHour

		inv.TotalPrice = inv.T5Report.Price + inv.T4Report.Price + inv.T3Report.Price + inv.T2Report.Price + inv.T1Report.Price

		_ = invoiceview.Invoice(inv).Render(context.Background(), w)
	}
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
		return nil, fmt.Errorf("Parsing end date failed: %w", err)
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

	t4Rate, err := strconv.ParseFloat(t4RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T4 rate failed: %w", err)
	}

	t3Rate, err := strconv.ParseFloat(t3RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T3 rate failed: %w", err)
	}

	t2Rate, err := strconv.ParseFloat(t2RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T2 rate failed: %w", err)
	}

	t1Rate, err := strconv.ParseFloat(t1RateString, 64)
	if err != nil {
		return nil, fmt.Errorf("Parsing T1 rate failed: %w", err)
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
