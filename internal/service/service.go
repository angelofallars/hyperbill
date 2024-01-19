package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/angelofallars/hyperbill/internal/domain"
	"github.com/angelofallars/hyperbill/pkg/trello"
)

type Invoice interface {
	Create(ctx context.Context, client *trello.Client, req CreateInvoiceRequest) (*domain.Invoice, error)
	GetBoards(ctx context.Context, client *trello.Client) ([]domain.Board, error)
}

type invoice struct{}

func NewInvoice() *invoice {
	return &invoice{}
}

type CreateInvoiceRequest struct {
	TrelloBoardID string
	StartDate     time.Time
	EndDate       time.Time
	T5Rate        float64
	T4Rate        float64
	T3Rate        float64
	T2Rate        float64
	T1Rate        float64
}

func (i *invoice) Create(ctx context.Context, client *trello.Client, req CreateInvoiceRequest) (*domain.Invoice, error) {
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

func (i *invoice) GetBoards(ctx context.Context, client *trello.Client) ([]domain.Board, error) {
	respBoards, err := client.GetBoards()
	if err != nil {
		return nil, err
	}

	boards := make([]domain.Board, 0, len(respBoards))
	for _, respBoard := range respBoards {
		boards = append(boards, domain.Board{
			ID:   respBoard.ID,
			Name: respBoard.Name,
		})
	}

	return boards, nil
}
