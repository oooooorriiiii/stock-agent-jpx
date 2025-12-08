package jquants

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	BaseURL      = "https://api.jquants.com/v1"
	AuthEndpoint = "/token/auth_refresh"
	FinsEndpoint = "/fins/statements"
)

type Client struct {
	RefreshToken string
	IDToken      string
}

func NewClient(refreshToken string) *Client {
	return &Client{RefreshToken: refreshToken}
}

func (c *Client) Authenticate() error {
	url := fmt.Sprintf("%s%s?refreshtoken=%s", BaseURL, AuthEndpoint, c.RefreshToken)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed: status %d body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		IDToken string `json:"idToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	c.IDToken = result.IDToken
	return nil
}

type FinancialStatement struct {
	LocalCode       string `json:"LocalCode"`
	DisclosedDate   string `json:"DisclosedDate"`
	
	// 実績
	OperatingProfit string `json:"OperatingProfit"`
	
	// 今期予想
	ForecastNetSales        string `json:"ForecastNetSales"`
	ForecastOperatingProfit string `json:"ForecastOperatingProfit"`

	// 来期予想
	NextYearForecastNetSales        string `json:"NextYearForecastNetSales"`
	NextYearForecastOperatingProfit string `json:"NextYearForecastOperatingProfit"`
}

// 修正: targetDate引数を追加
func (c *Client) GetStatements(targetDate string) ([]FinancialStatement, error) {
	if c.IDToken == "" {
		if err := c.Authenticate(); err != nil {
			return nil, err
		}
	}

	// 修正: dateパラメータを付与
	url := fmt.Sprintf("%s%s?date=%s", BaseURL, FinsEndpoint, targetDate)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+c.IDToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error: %d %s", resp.StatusCode, string(body))
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		Statements []FinancialStatement `json:"statements"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, err
	}
	return result.Statements, nil
}