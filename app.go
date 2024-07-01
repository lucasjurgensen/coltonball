package main

import (
	"database/sql"
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

	_ "github.com/mattn/go-sqlite3"
)

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

var db *sql.DB
var dbMutex sync.Mutex

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "/var/lib/litefs/names.db")
	if err != nil {
		log.Fatal(err)
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS names (
		"id" INTEGER PRIMARY KEY AUTOINCREMENT,
		"date" TEXT,
		"name" TEXT
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatal(err)
	}
}

func printState() {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	yesterday := time.Now().AddDate(0, 0, -1)

	fmt.Printf("############\nCurrent State at: %v\n", time.Now())
	rows, err := db.Query("SELECT date, name FROM names WHERE date > ?", yesterday.Format("2006-01-02"))
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	namesMap := make(map[string][]string)
	for rows.Next() {
		var dateStr string
		var name string
		err = rows.Scan(&dateStr, &name)
		if err != nil {
			log.Fatal(err)
		}
		namesMap[dateStr] = append(namesMap[dateStr], name)
	}

	for dateStr, names := range namesMap {
		fmt.Printf("Date: %s, Names: %v\n", dateStr, names)
	}
	println("############")
}

func main() {
	initDB()
	defer db.Close()

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

		dbMutex.Lock()
		for d := 1; d <= daysInMonth; d++ {
			date := firstOfMonth.AddDate(0, 0, d-1).Format("2006-01-02")
			rows, err := db.Query("SELECT name FROM names WHERE date = ?", date)
			if err != nil {
				log.Fatal(err)
			}
			var names []string
			for rows.Next() {
				var name string
				err = rows.Scan(&name)
				if err != nil {
					log.Fatal(err)
				}
				names = append(names, name)
			}
			rows.Close()
			days = append(days, map[string]interface{}{"day": d, "count": len(names), "names": names, "date": date})
		}
		dbMutex.Unlock()

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

		dbMutex.Lock()
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM names WHERE date = ? AND name = ?)", date, name).Scan(&exists)
		if err != nil {
			log.Fatal(err)
		}

		if !exists {
			_, err = db.Exec("INSERT INTO names (date, name) VALUES (?, ?)", date, name)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			log.Printf("Did not add name %s to date %s as they already signed up", name, date)
		}
		dbMutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})

		log.Printf("Added name %s to date %s. New count: %d", name, date, len(name)) // This log line may need adjustment

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

		dbMutex.Lock()
		_, err = db.Exec("DELETE FROM names WHERE date = ? AND name = ?", req.Date, req.Name)
		dbMutex.Unlock()
		if err != nil {
			log.Fatal(err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})

		printState()
	})

	http.HandleFunc("/names", func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")

		dbMutex.Lock()
		rows, err := db.Query("SELECT name FROM names WHERE date = ?", date)
		dbMutex.Unlock()
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()

		var names []string
		for rows.Next() {
			var name string
			err = rows.Scan(&name)
			if err != nil {
				log.Fatal(err)
			}
			names = append(names, name)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"names": names,
		})
	})

	log.Println("Listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
