package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type Holding struct {
	Symbol   string  `json:"symbol"`
	Quantity float64 `json:"quantity"`
	AvgPrice float64 `json:"avg_price"`
}

type Price struct {
	Symbol   string
	Currency string
	Value    float64
}

type yahooResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol             string  `json:"symbol"`
				Currency           string  `json:"currency"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
		} `json:"result"`
		Error *string `json:"error"`
	} `json:"chart"`
}

func LoadHoldings(path string) ([]Holding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading holdings file: %w", err)
	}

	var holdings []Holding
	if err := json.Unmarshal(data, &holdings); err != nil {
		return nil, fmt.Errorf("parsing holdings json: %w", err)
	}

	return holdings, nil
}

func fetchPrice(symbol string) (Price, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s", symbol)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Price{}, fmt.Errorf("building request for %s: %w", symbol, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Price{}, fmt.Errorf("fetching %s: %w", symbol, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Price{}, fmt.Errorf("reading response for %s: %w", symbol, err)
	}

	var parsed yahooResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Price{}, fmt.Errorf("parsing response for %s: %w", symbol, err)
	}

	if parsed.Chart.Error != nil {
		return Price{}, fmt.Errorf("yahoo returned error for %s: %s", symbol, *parsed.Chart.Error)
	}
	if len(parsed.Chart.Result) == 0 {
		return Price{}, fmt.Errorf("no data returned for %s", symbol)
	}

	meta := parsed.Chart.Result[0].Meta
	return Price{
		Symbol:   meta.Symbol,
		Currency: meta.Currency,
		Value:    meta.RegularMarketPrice,
	}, nil
}

func main() {
	price, err := fetchPrice("RELIANCE.NS")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Printf("%s: %.2f %s\n", price.Symbol, price.Value, price.Currency)
}
