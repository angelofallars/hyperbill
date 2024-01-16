// Package trello provides a thin Trello JSON API client
// that only fetches the necessary information to calculate billing.
package trello

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	apiKey string
	token  string
}

const baseURL = "https://api.trello.com/1/"

func New(apiKey, token string) *Client {
	return &Client{
		apiKey: apiKey,
		token:  token,
	}
}

var (
	ErrInvalidKey   = errors.New("The provided Trello API key is invalid.")
	ErrInvalidToken = errors.New("The provided Trello API token is invalid.")
)

func (c *Client) get(path string) (*http.Response, error) {
	resp, err := http.Get(fmt.Sprintf("%s%s?key=%s&token=%s", baseURL,
		path, c.apiKey, c.token))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode > 299 {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		body := string(bodyBytes)
		switch body {
		case "invalid key":
			return nil, ErrInvalidKey
		case "invalid app token":
			return nil, ErrInvalidToken
		default:
			return nil, fmt.Errorf("Trello request failed: %s", string(body))
		}
	}

	return resp, nil
}

type GetBoardCardsResponse struct {
	ID     string  `json:"id"`
	Labels []Label `json:"labels"`
}

type Label struct {
	Name string `json:"name"`
}

type Card struct {
	ID     string
	Labels []string
}

// Calls GET https://api.trello.com/1/boards/{boardID}/cards
func (c *Client) GetCards(boardID string) ([]Card, error) {
	resp, err := c.get(fmt.Sprintf("boards/%s/cards", boardID))
	if err != nil {
		return nil, err
	}

	respCards := []GetBoardCardsResponse{}
	err = json.NewDecoder(resp.Body).Decode(&respCards)
	if err != nil {
		return nil, err
	}

	cards := []Card{}
	for _, card := range respCards {
		labels := []string{}
		for _, label := range card.Labels {
			labels = append(labels, label.Name)
		}
		cards = append(cards, Card{ID: card.ID, Labels: labels})
	}

	return cards, nil
}

type Action struct {
	ID              string         `json:"id"`
	IDMemberCreator string         `json:"idMemberCreator"`
	Data            map[string]any `json:"data"`
	Type            string         `json:"type"`
	Date            string         `json:"date"`
}

// Calls GET https://api.trello.com/1/cards/{boardID}/actions
func (c *Client) GetCardActions(cardID string) ([]Action, error) {
	// TODO: make use of batch requests to speed this up
	resp, err := c.get(fmt.Sprintf("cards/%s/actions", cardID))
	if err != nil {
		return nil, err
	}

	var actions []Action
	err = json.NewDecoder(resp.Body).Decode(&actions)
	if err != nil {
		return nil, err
	}

	return actions, nil
}

type Board struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Calls GET https://api.trello.com/1/members/me/boards
func (c *Client) GetBoards() ([]Board, error) {
	resp, err := c.get("members/me/boards")
	if err != nil {
		return nil, err
	}

	var boards []Board
	err = json.NewDecoder(resp.Body).Decode(&boards)
	if err != nil {
		return nil, err
	}

	return boards, nil
}
