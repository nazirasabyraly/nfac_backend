package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/go-chi/cors"
	"github.com/robfig/cron/v3"
	"net/smtp"
)

var db *sql.DB

type Capsule struct {
	ID            int       `json:"id"`
	Email         string    `json:"email"`
	Subject       string    `json:"subject"`
	Message       string    `json:"message"`
	AttachmentURL *string   `json:"attachment_url"`
	SendAt        time.Time `json:"send_at"`
	Sent          bool      `json:"sent"`
	CreatedAt     time.Time `json:"created_at"`
}

func startCapsuleChecker() {
	c := cron.New()

	c.AddFunc("@every 1m", func() {
		log.Println("Проверка капсул на отправку...")

		now := time.Now()

		rows, err := db.Query(`SELECT id, email, subject, message FROM capsules WHERE send_at <= $1 AND sent = FALSE`, now)
		if err != nil {
			log.Println("Ошибка запроса капсул:", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var id int
			var email, subject, message string
			if err := rows.Scan(&id, &email, &subject, &message); err != nil {
				log.Println("Ошибка чтения капсулы:", err)
				continue
			}

			err = sendEmail(email, subject, message)
			if err != nil {
				log.Println("Ошибка отправки письма:", err)
				continue
			}

			_, err = db.Exec(`UPDATE capsules SET sent = TRUE WHERE id = $1`, id)
			if err != nil {
				log.Println("Ошибка обновления статуса капсулы:", err)
				continue
			}

			log.Printf("Капсула ID %d отправлена на %s\n", id, email)
		}
	})

	c.Start()
}

func sendEmail(to, subject, body string) error {
	from := "timecapsule@example.com"
	username := "69639969ff4882"
	password := "5093b8b0d424f4"
	smtpHost := "sandbox.smtp.mailtrap.io"
	smtpPort := "587"

	msg := "From: " + from + "\n" +
		"To: " + to + "\n" +
		"Subject: " + subject + "\n\n" +
		body

	auth := smtp.PlainAuth("", username, password, smtpHost)

	return smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{to}, []byte(msg))
}
func deleteCapsuleHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		http.Error(w, "Missing capsule ID", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("DELETE FROM capsules WHERE id = $1", id)
	if err != nil {
		http.Error(w, "Failed to delete capsule", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func main() {
	var err error

	db, err = sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных:", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal("Ошибка при пинге базы данных:", err)
	}

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://nfac-frontend.vercel.app"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"}, // ← ДОБАВИЛИ "DELETE"
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
	}))

	r.Use(middleware.Logger)

	r.Post("/capsules", createCapsuleHandler)
	r.Get("/capsules", listCapsulesHandler)
	r.Delete("/capsules/{id}", deleteCapsuleHandler)

	startCapsuleChecker()

	log.Println("Сервер запущен на порту :8080...")
	http.ListenAndServe(":8080", r)
}

func createCapsuleHandler(w http.ResponseWriter, r *http.Request) {
	var capsule Capsule
	err := json.NewDecoder(r.Body).Decode(&capsule)
	if err != nil {
		http.Error(w, "Невалидные данные", http.StatusBadRequest)
		return
	}

	query := `
        INSERT INTO capsules (email, subject, message, attachment_url, send_at)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, created_at
    `
	err = db.QueryRow(query, capsule.Email, capsule.Subject, capsule.Message, capsule.AttachmentURL, capsule.SendAt).
		Scan(&capsule.ID, &capsule.CreatedAt)
	if err != nil {
		log.Println("Ошибка сохранения в базу данных:", err)
		http.Error(w, "Ошибка сохранения в базу данных", http.StatusInternalServerError)
		return
	}

	capsule.Sent = false

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(capsule)
}

func listCapsulesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, email, subject, message, attachment_url, send_at, sent, created_at FROM capsules ORDER BY send_at`)
	if err != nil {
		http.Error(w, "Ошибка запроса к базе данных", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var capsules []Capsule
	for rows.Next() {
		var capsule Capsule
		err := rows.Scan(&capsule.ID, &capsule.Email, &capsule.Subject, &capsule.Message, &capsule.AttachmentURL, &capsule.SendAt, &capsule.Sent, &capsule.CreatedAt)
		if err != nil {
			http.Error(w, "Ошибка чтения строки", http.StatusInternalServerError)
			return
		}
		capsules = append(capsules, capsule)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(capsules)
}
