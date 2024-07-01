package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

var namesStore = struct {
	sync.Mutex
	data map[string][]string
}{
	data: make(map[string][]string),
}

func printState() {
	namesStore.Lock()
	defer namesStore.Unlock()

	yesterday := time.Now().AddDate(0, 0, -1)

	fmt.Printf("############\nCurrent State at: %v\n", time.Now())
	for dateStr, names := range namesStore.data {
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			fmt.Printf("Error parsing date %s: %v\n", dateStr, err)
			continue
		}

		if date.After(yesterday) && len(names) > 0 {
			fmt.Printf("Date: %s, Names: %v\n", dateStr, names)
		}
	}
	println("############")

}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		monthParam := r.URL.Query().Get("month")
		yearParam := r.URL.Query().Get("year")

		now := time.Now()
		currentYear, currentMonth := now.Year(), now.Month()

		if monthParam != "" && yearParam != "" {
			year, err := strconv.Atoi(yearParam)
			if err == nil {
				monthInt, err := strconv.Atoi(monthParam)
				if err == nil && monthInt >= 1 && monthInt <= 12 {
					currentYear = year
					currentMonth = time.Month(monthInt)
				}
			}
		}

		firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, time.UTC)
		lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
		firstDayOfWeek := int(firstOfMonth.Weekday())
		daysInMonth := lastOfMonth.Day()

		days := make([]map[string]interface{}, 0, 42)

		for i := 0; i < firstDayOfWeek; i++ {
			days = append(days, map[string]interface{}{"day": 0, "count": 0, "names": []string{}, "date": ""})
		}

		for d := 1; d <= daysInMonth; d++ {
			date := firstOfMonth.AddDate(0, 0, d-1).Format("2006-01-02")
			namesStore.Lock()
			names := namesStore.data[date]
			namesStore.Unlock()
			days = append(days, map[string]interface{}{"day": d, "count": len(names), "names": names, "date": date})
		}

		for len(days) < 42 {
			days = append(days, map[string]interface{}{"day": 0, "count": 0, "names": []string{}, "date": ""})
		}

		prevMonth := currentMonth - 1
		prevYear := currentYear
		if prevMonth < time.January {
			prevMonth = time.December
			prevYear--
		}

		nextMonth := currentMonth + 1
		nextYear := currentYear
		if nextMonth > time.December {
			nextMonth = time.January
			nextYear++
		}

		data := map[string]interface{}{
			"Month":       currentMonth.String(),
			"MonthNumber": int(currentMonth),
			"Year":        currentYear,
			"Days":        days,
			"PrevMonth":   int(prevMonth),
			"PrevYear":    prevYear,
			"NextMonth":   int(nextMonth),
			"NextYear":    nextYear,
		}

		t.ExecuteTemplate(w, "index.html.tmpl", data)
	})

	http.HandleFunc("/submit-name", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		date := r.FormValue("date")
		name := r.FormValue("name")

		namesStore.Lock()
		exists := false
		for _, n := range namesStore.data[date] {
			if n == name {
				exists = true
				log.Printf("Did not add name %s to date %s as they already signed up", name, date)
			}
		}

		if !exists {
			namesStore.data[date] = append(namesStore.data[date], name)
		}
		namesStore.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})

		log.Printf("Added name %s to date %s. New count: %d", name, date, len(namesStore.data[date]))

		printState()
	})

	http.HandleFunc("/remove-name", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Date string `json:"date"`
			Name string `json:"name"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		namesStore.Lock()
		names := namesStore.data[req.Date]
		for i, n := range names {
			if n == req.Name {
				namesStore.data[req.Date] = append(names[:i], names[i+1:]...)
				break
			}
		}
		namesStore.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})

		printState()
	})

	http.HandleFunc("/names", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")

		namesStore.Lock()
		names := namesStore.data[date]
		namesStore.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"names": names,
		})
	})

	log.Println("Listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
