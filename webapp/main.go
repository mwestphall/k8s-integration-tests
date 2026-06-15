package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/google/go-github/v68/github"
	"github.com/osg-htc/k8s-integration-tests/webapp/handlers"
	"github.com/osg-htc/k8s-integration-tests/webapp/util"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var rawStaticFS embed.FS

func main() {
	ctx := context.Background()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}
	owner := os.Getenv("GITHUB_OWNER")
	if owner == "" {
		log.Fatal("GITHUB_OWNER environment variable is required")
	}
	repo := os.Getenv("GITHUB_REPO")
	if repo == "" {
		log.Fatal("GITHUB_REPO environment variable is required")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	funcMap := template.FuncMap{
		"statusClass": func(conclusion string) string {
			switch conclusion {
			case "success":
				return "status-success"
			case "failure", "timed_out", "action_required":
				return "status-failure"
			case "cancelled", "skipped":
				return "status-cancelled"
			default:
				return "status-pending"
			}
		},
		"formatTime": func(t github.Timestamp) string {
			return t.UTC().Format("2006-01-02 15:04 UTC")
		},
		"jobConclusion": util.JobConclusion,
		"add":           func(a, b int) int { return a + b },
	}

	tmpl := template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"),
	)

	staticFS, err := fs.Sub(rawStaticFS, "static")
	if err != nil {
		log.Fatalf("preparing static FS: %v", err)
	}

	client := util.NewGitHubClient(ctx, token)
	app := handlers.NewApp(client, owner, repo, tmpl, staticFS)

	mux := http.NewServeMux()
	app.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
