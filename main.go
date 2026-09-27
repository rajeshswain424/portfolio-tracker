package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"text/tabwriter"
	"time"
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

type Result struct {
	Holding Holding
	Price   Price
	Err     error
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

func worker(jobs <-chan Holding, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for h := range jobs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		price, err := fetchPrice(ctx, h.Symbol)
		cancel()
		results <- Result{Holding: h, Price: price, Err: err}
	}
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

func fetchPrice(ctx context.Context, symbol string) (Price, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s", symbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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

func runReport(holdings []Holding) {
	jobs := make(chan Holding, len(holdings))
	results := make(chan Result, len(holdings))

	var wg sync.WaitGroup
	numWorkers := 5
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(jobs, results, &wg)
	}

	for _, h := range holdings {
		jobs <- h
	}
	close(jobs)

	wg.Wait()
	close(results)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SYMBOL\tQTY\tAVG\tCURRENT\tGAIN/LOSS\tCURRENCY")

	var totalGainLoss float64
	for r := range results {
		if r.Err != nil {
			fmt.Fprintf(w, "%s\terror: %v\n", r.Holding.Symbol, r.Err)
			continue
		}
		gainLoss := (r.Price.Value - r.Holding.AvgPrice) * r.Holding.Quantity
		totalGainLoss += gainLoss
		fmt.Fprintf(w, "%s\t%.2f\t%.2f\t%.2f\t%.2f\t%s\n",
			r.Holding.Symbol, r.Holding.Quantity, r.Holding.AvgPrice, r.Price.Value, gainLoss, r.Price.Currency)
	}
	w.Flush()

	fmt.Printf("\nTotal gain/loss: %.2f\n", totalGainLoss)
	fmt.Printf("Last updated: %s\n", time.Now().Format("15:04:05"))
}

func main() {
	watch := flag.Bool("watch", false, "refresh prices every 30 seconds")
	flag.Parse()

	holdings, err := LoadHoldings("holdings.json")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	if !*watch {
		runReport(holdings)
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	runReport(holdings)
	for range ticker.C {
		fmt.Print("\033[H\033[2J")
		runReport(holdings)
	}
}
