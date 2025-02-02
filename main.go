package main

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/jszwec/csvutil"
)

var (
	//go:embed chart.js index.html
	staticFS embed.FS
)

// Row in CSV
type Row struct {
	Date   time.Time
	Close  float64
	Volume int
}

// Table of data
type Table struct {
	Date   []time.Time
	Price  []float64
	Volume []int
}

func main() {
	http.Handle("/", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("/data", dataHandler)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

// unmarshalTime unmarshal data in CSV to time
func unmarshalTime(data []byte, t *time.Time) error {
	var err error
	*t, err = time.Parse("2006-01-02", string(data))
	return err
}

// parseData parses data from r and returns a table with columns filled
func parseData(r io.Reader) (Table, error) {
	dec, err := csvutil.NewDecoder(csv.NewReader(r))
	if err != nil {
		return Table{}, err
	}
	dec.Register(unmarshalTime)

	var table Table
	for {
		var row Row
		err := dec.Decode(&row)

		if err == io.EOF {
			break
		}

		if err != nil {
			return Table{}, err
		}

		table.Date = append(table.Date, row.Date)
		table.Price = append(table.Price, row.Close)
		table.Volume = append(table.Volume, row.Volume)
	}

	return table, nil
}

// buildURL builds URL for downloading CSV from Yahoo! finance
func buildURL(symbol string, start, end time.Time) string {
	u := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/download/%s", url.PathEscape(symbol))
	// u := "https://query1.finance.yahoo.com/v7/finance/download/MSFT?period1=1609286400&period2=1640822400&interval=1d&events=history"
	v := url.Values{
		"period1":  {fmt.Sprintf("%d", start.Unix())},
		"period2":  {fmt.Sprintf("%d", end.Unix())},
		"interval": {"1d"},
		"events":   {"history"},
	}

	return fmt.Sprintf("%s?%s", u, v.Encode())
}

// stockData returns stock data from Yahoo! finance
func stockData(symbol string, start, end time.Time) (Table, error) {
	u := buildURL(symbol, start, end)
	resp, err := http.Get(u)
	if err != nil {
		return Table{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Table{}, fmt.Errorf("%s", resp.Status)
	}
	defer resp.Body.Close()

	return parseData(resp.Body)
}

// dataHandler returns JSON data for symbol
func dataHandler(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "empty symbol", http.StatusBadRequest)
		return
	}
	log.Printf("data: %q", symbol)
	start := time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2021, time.December, 31, 0, 0, 0, 0, time.UTC)
	table, err := stockData(symbol, start, end)
	if err != nil {
		log.Printf("get %q: %s", symbol, err)
		http.Error(w, "can't fetch data", http.StatusInternalServerError)
		return
	}

	if err := tableJSON(symbol, table, w); err != nil {
		log.Printf("table: %s", err)
	}
}

// tableJSON writes table data as JSON into w
func tableJSON(symbol string, table Table, w io.Writer) error {
	reply := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"x":    table.Date,
				"y":    table.Price,
				"name": "Price",
				"type": "scatter",
			},
			{
				"x":     table.Date,
				"y":     table.Volume,
				"name":  "Volume",
				"type":  "bar",
				"yaxis": "y2",
			},
		},
		"layout": map[string]interface{}{
			"title": symbol,
			"grid": map[string]int{
				"rows":    2,
				"columns": 1,
			},
		},
	}

	return json.NewEncoder(w).Encode(reply)
}
