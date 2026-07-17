package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type EmailRequest struct {
	DisplayName string `json:"displayName" validate:"required"`
	EmailTo     string `json:"emailTo" validate:"required,email"`
	Subject     string `json:"subject" validate:"required,min=1,max=200"`
	Body        string `json:"body" validate:"required"`
}

type ErrorResponse struct {
	Error   string   `json:"error"`
	Details []string `json:"details,omitempty"`
}

var validate *validator.Validate

func init() {
	validate = validator.New()
}

func SendGmailREST(displayName, to, subject, body string) error {
	ctx := context.Background()

	fromEmail := os.Getenv("GMAIL")
	clientID := os.Getenv("GMAIL_CLIENT_ID")
	clientSecret := os.Getenv("GMAIL_CLIENT_SECRET")
	refreshToken := os.Getenv("GMAIL_REFRESH_TOKEN")

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gmail.GmailSendScope},
	}

	token := &oauth2.Token{
		RefreshToken: refreshToken,
	}

	client := config.Client(ctx, token)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("failed to create gmail client: %w", err)
	}

	messageStr := fmt.Sprintf(
		"From: \"%s\" <%s>\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/html; charset=utf-8\r\n\r\n%s",
		displayName, fromEmail, to, subject, body,
	)

	encodedMessage := base64.URLEncoding.EncodeToString([]byte(messageStr))

	msg := &gmail.Message{
		Raw: encodedMessage,
	}

	_, err = srv.Users.Messages.Send("me", msg).Do()
	if err != nil {
		return fmt.Errorf("failed to send via API: %w", err)
	}

	return nil
}

func handleSendEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed", nil)
		return
	}

	var req EmailRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body structure", nil)
		return
	}

	err = validate.Struct(req)
	if err != nil {
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			validationErrors = append(validationErrors, fmt.Sprintf("Field '%s' failed validation validation checking for '%s'", err.Field(), err.Tag()))
		}
		writeJSONError(w, http.StatusBadRequest, "Validation validation failed", validationErrors)
		return
	}

	err = SendGmailREST(req.DisplayName, req.EmailTo, req.Subject, req.Body)
	if err != nil {
		log.Printf("Internal Sending failure: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to send email through downstream gateway", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Email dispatched out successfully"})
}

func writeJSONError(w http.ResponseWriter, status int, errMsg string, details []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   errMsg,
		Details: details,
	})
}

func main() {
	_ = godotenv.Load(".env.local")

	if os.Getenv("GMAIL_REFRESH_TOKEN") == "" {
		log.Fatal("FATAL: Critical environmental dependency GMAIL_REFRESH_TOKEN missing.")
	}

	http.HandleFunc("/api/v1/emails/send", handleSendEmail)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server initializing execution engine on port %s...", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Fatal execution crash: %v", err)
	}
}
