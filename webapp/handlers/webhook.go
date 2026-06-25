package handlers

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/osg-htc/k8s-integration-tests/webapp/util"
)

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (a *App) handleEmailWebhook(w http.ResponseWriter, r *http.Request) {
	// Authenticate via Authorization: Bearer <token>
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(a.email.Token)) != 1 {
		jsonError(w, "unauthorized", http.StatusForbidden)
		return
	}

	// Guard: return 500 if email is not configured rather than failing at startup,
	// so the webapp can run without email relay in testing environments.
	if a.email.Token == "" || a.email.ResultsDomain == "" || a.email.SMTPRelay == "" ||
		a.email.SMTPPort == "" || a.email.FromAddress == "" || len(a.email.ToAddresses) == 0 {
		jsonError(w, "email webhook not configured", http.StatusInternalServerError)
		return
	}

	// Parse request body
	var req struct {
		RunID int64 `json:"runID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RunID == 0 {
		jsonError(w, "invalid request body: runID required", http.StatusBadRequest)
		return
	}

	summary, err := util.FetchRunSummary(r.Context(), a.client, a.owner, a.repo, req.RunID)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
			jsonError(w, "run not found", http.StatusNotFound)
			return
		}
		jsonError(w, fmt.Sprintf("fetching run: %v", err), http.StatusInternalServerError)
		return
	}

	runURL := fmt.Sprintf("https://%s/runs/%d", a.email.ResultsDomain, req.RunID)

	// Render HTML email body
	var body bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&body, "email", map[string]any{
		"Run":    summary.Run,
		"Suites": summary.Suites,
		"RunURL": runURL,
	}); err != nil {
		jsonError(w, fmt.Sprintf("rendering email: %v", err), http.StatusInternalServerError)
		return
	}

	conclusion := summary.Run.GetConclusion()
	if conclusion == "" {
		conclusion = summary.Run.GetStatus()
	}
	subject := fmt.Sprintf("K8s Integration Test Results — Run #%d (%s)", req.RunID, conclusion)
	msg := buildMIMEMessage(a.email.FromAddress, a.email.ToAddresses, subject, body.String())

	addr := fmt.Sprintf("%s:%s", a.email.SMTPRelay, a.email.SMTPPort)
	if err := smtp.SendMail(addr, nil, a.email.FromAddress, a.email.ToAddresses, msg); err != nil {
		jsonError(w, fmt.Sprintf("sending email: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

func buildMIMEMessage(from string, to []string, subject, htmlBody string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: text/html; charset=UTF-8\r\n")
	fmt.Fprintf(&buf, "\r\n")
	fmt.Fprintf(&buf, "%s", htmlBody)
	return buf.Bytes()
}
