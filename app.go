package main

import (
	"embed"
	"encoding/json"
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
		firstDayOfWeek := int(firstOfMonth.Weekday())

		lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
		daysInMonth := lastOfMonth.Day()

		days := make([]string, 0, 42)

		for i := 0; i < firstDayOfWeek; i++ {
			days = append(days, "")
		}
		for d := 1; d <= daysInMonth; d++ {
			days = append(days, strconv.Itoa(d))
		}
		for len(days) < 42 {
			days = append(days, "")
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
		namesStore.data[date] = append(namesStore.data[date], name)
		namesStore.Unlock()

		http.Redirect(w, r, "/", http.StatusSeeOther)
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

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
