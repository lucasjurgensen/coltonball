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

		days := make([]map[string]interface{}, 0, 42)

		for i := 0; i < firstDayOfWeek; i++ {
			days = append(days, map[string]interface{}{"day": "", "count": 0, "names": []string{}})
		}
		for d := 1; d <= daysInMonth; d++ {
			date := firstOfMonth.AddDate(0, 0, d-1).Format("2006-01-02")
			namesStore.Lock()
			names := namesStore.data[date]
			count := len(names)
			namesStore.Unlock()
			log.Printf("Date: %s, Count: %d, Names: %v", date, count, names)
			days = append(days, map[string]interface{}{"day": d, "count": count, "names": names})
		}
		for len(days) < 42 {
			days = append(days, map[string]interface{}{"day": "", "count": 0, "names": []string{}})
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
		dateStr := r.FormValue("date")
		name := r.FormValue("name")

		// Parse the date and reformat it to ensure consistency
		date, err := time.Parse("2006-1-2", dateStr)
		if err != nil {
			http.Error(w, "Invalid date format", http.StatusBadRequest)
			return
		}
		formattedDate := date.Format("2006-01-02")

		namesStore.Lock()
		namesStore.data[formattedDate] = append(namesStore.data[formattedDate], name)
		log.Printf("Added name %s to date %s. New count: %d", name, formattedDate, len(namesStore.data[formattedDate]))
		namesStore.Unlock()
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
