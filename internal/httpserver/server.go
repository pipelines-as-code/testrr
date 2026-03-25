package httpserver

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"testrr/internal/auth"
	"testrr/internal/config"
	"testrr/internal/parser"
	"testrr/internal/store"
	"testrr/internal/views"
	webassets "testrr/web"
)

type Server struct {
	cfg       config.Config
	repo      store.Repository
	parsers   parser.Registry
	assets    http.Handler
	artifacts string
}

func New(cfg config.Config, repo store.Repository, parsers parser.Registry) (*Server, error) {
	assetsFS, err := fs.Sub(webassets.FS, "dist")
	if err != nil {
		return nil, fmt.Errorf("open asset fs: %w", err)
	}

	artifactsDir := filepath.Join(cfg.DataDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifacts dir: %w", err)
	}

		return &Server{
		cfg:       cfg,
		repo:      repo,
		parsers:   parsers,
		assets:    http.FileServerFS(assetsFS),
		artifacts: artifactsDir,
	}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", s.assets))
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /projects/{slug}", s.handleDashboard)
	mux.HandleFunc("GET /projects/{slug}/runs", s.handleRunsPage)
	mux.HandleFunc("GET /projects/{slug}/runs/{runID}", s.handleRunPage)
	mux.HandleFunc("GET /projects/{slug}/tests/{testKey}", s.handleTestPage)
	mux.HandleFunc("GET /projects/{slug}/charts/summary", s.handleChartSummary)
	mux.HandleFunc("POST /api/v1/projects/{slug}/runs", s.handleUploadAPI)
	mux.HandleFunc("GET /api/v1/projects/{slug}/runs", s.handleListRunsAPI)
	mux.HandleFunc("GET /api/v1/projects/{slug}/runs/{runID}", s.handleGetRunAPI)
	s.withAccessLog(mux).ServeHTTP(w, r)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	projects, err := s.repo.ListProjects(r.Context())
	if err != nil {
		http.Error(w, "unable to load projects", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, views.HomePage(projects))
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	project, err := s.loadProject(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	data, err := s.repo.GetDashboardData(r.Context(), project.ID, branch, 20)
	if err != nil {
		http.Error(w, "unable to load dashboard", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, views.DashboardPage(project, data, branch))
}

func (s *Server) handleRunsPage(w http.ResponseWriter, r *http.Request) {
	project, err := s.loadProject(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	runs, err := s.repo.ListRunsByProject(r.Context(), project.ID, branch, 50)
	if err != nil {
		http.Error(w, "unable to load runs", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, views.RunsPage(project, runs, branch))
}

func (s *Server) handleRunPage(w http.ResponseWriter, r *http.Request) {
	project, err := s.loadProject(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	runID := r.PathValue("runID")
	run, err := s.repo.GetRun(r.Context(), project.ID, runID)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	results, err := s.repo.ListRunResults(r.Context(), runID)
	if err != nil {
		http.Error(w, "unable to load run results", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, views.RunPage(project, run, results))
}

func (s *Server) handleTestPage(w http.ResponseWriter, r *http.Request) {
	project, err := s.loadProject(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	testKey := r.PathValue("testKey")
	history, err := s.repo.GetTestHistory(r.Context(), project.ID, testKey, 30)
	if err != nil {
		http.Error(w, "unable to load test history", http.StatusInternalServerError)
		return
	}
	s.render(w, http.StatusOK, views.TestPage(project, testKey, history))
}

func (s *Server) handleChartSummary(w http.ResponseWriter, r *http.Request) {
	project, err := s.loadProject(r.Context(), r.PathValue("slug"))
	if err != nil {
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	summary, err := s.repo.GetChartSummary(r.Context(), project.ID, branch, 20)
	if err != nil {
		http.Error(w, "unable to load charts", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleUploadAPI(w http.ResponseWriter, r *http.Request) {
	project, err := s.authenticateAPIRequest(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="testrr"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	run, err := s.ingestRun(w, r.Context(), project, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) handleListRunsAPI(w http.ResponseWriter, r *http.Request) {
	project, err := s.authenticateAPIRequest(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="testrr"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	runs, err := s.repo.ListRunsByProject(r.Context(), project.ID, strings.TrimSpace(r.URL.Query().Get("branch")), 50)
	if err != nil {
		http.Error(w, "unable to list runs", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleGetRunAPI(w http.ResponseWriter, r *http.Request) {
	project, err := s.authenticateAPIRequest(r)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="testrr"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	run, err := s.repo.GetRun(r.Context(), project.ID, r.PathValue("runID"))
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	results, err := s.repo.ListRunResults(r.Context(), run.ID)
	if err != nil {
		http.Error(w, "unable to load results", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Run     store.Run          `json:"run"`
		Results []store.TestResult `json:"results"`
	}{Run: run, Results: results})
}

func (s *Server) authenticateAPIRequest(r *http.Request) (store.Project, error) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return store.Project{}, errors.New("missing basic auth")
	}
	return auth.AuthenticateBasic(r.Context(), s.repo, r.PathValue("slug"), username, password)
}

func (s *Server) ingestRun(w http.ResponseWriter, ctx context.Context, project store.Project, r *http.Request) (store.Run, error) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
		return store.Run{}, fmt.Errorf("parse upload form: %w", err)
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		return store.Run{}, errors.New("at least one report file is required")
	}

	runID := newID()
	artifactsDir := filepath.Join(s.artifacts, runID)
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return store.Run{}, fmt.Errorf("create run artifact dir: %w", err)
	}

	uploadedAt := time.Now().UTC()
	startedAt := parseTimeOrDefault(r.FormValue("started_at"), uploadedAt)
	branch := strings.TrimSpace(r.FormValue("branch"))
	runLabel := strings.TrimSpace(r.FormValue("run_label"))
	if runLabel == "" {
		runLabel = firstNonEmpty(r.FormValue("build_id"), r.FormValue("commit_sha"), uploadedAt.Format("2006-01-02 15:04"))
	}

	previousRun, _ := s.repo.FindPreviousRun(ctx, project.ID, branch)
	previousResults := map[string]string{}
	if previousRun != nil {
		existing, err := s.repo.ListRunResults(ctx, previousRun.ID)
		if err == nil {
			for _, result := range existing {
				previousResults[result.TestKey] = result.Status
			}
		}
	}

	artifacts := make([]store.Artifact, 0, len(files))
	testResults := make([]store.TestResult, 0)
	totalCount := 0
	passedCount := 0
	failedCount := 0
	skippedCount := 0
	var durationMillis int64
	parseError := ""

	for _, header := range files {
		source, err := header.Open()
		if err != nil {
			return store.Run{}, fmt.Errorf("open upload %s: %w", header.Filename, err)
		}

		contents, readErr := io.ReadAll(source)
		source.Close()
		if readErr != nil {
			return store.Run{}, fmt.Errorf("read upload %s: %w", header.Filename, readErr)
		}

		checksum := fmt.Sprintf("%x", sha256.Sum256(contents))
		artifactPath := filepath.Join(artifactsDir, sanitizeFileName(header.Filename)+".gz")
		if err := writeGzipFile(artifactPath, contents); err != nil {
			return store.Run{}, fmt.Errorf("store artifact %s: %w", header.Filename, err)
		}

		artifact := store.Artifact{
			ID:          newID(),
			RunID:       runID,
			FileName:    header.Filename,
			Format:      "junit",
			FilePath:    artifactPath,
			Checksum:    checksum,
			SizeBytes:   int64(len(contents)),
			ParseStatus: "parsed",
		}

		parsedFiles, err := s.parsers.ParseFiles(ctx, []parser.UploadFile{{Name: header.Filename, Contents: contents}})
		if err != nil {
			artifact.ParseStatus = "failed"
			artifact.ParseError = err.Error()
			parseError = err.Error()
			artifacts = append(artifacts, artifact)
			break
		}

		for _, parsedFile := range parsedFiles {
			artifact.Format = parsedFile.Format
			for _, parsed := range parsedFile.TestResults {
				regression := parsed.Status == "failed" && previousResults[parsed.TestKey] != "failed"
				result := store.TestResult{
					ID:             newID(),
					RunID:          runID,
					ProjectID:      project.ID,
					TestKey:        parsed.TestKey,
					SuiteName:      parsed.SuiteName,
					PackageName:    parsed.PackageName,
					ClassName:      parsed.ClassName,
					TestName:       parsed.TestName,
					FileName:       parsed.FileName,
					Status:         parsed.Status,
					DurationMillis: parsed.DurationMillis,
					FailureMessage: parsed.FailureMessage,
					FailureOutput:  parsed.FailureOutput,
					SystemOut:      parsed.SystemOut,
					SystemErr:      parsed.SystemErr,
					Regression:     regression,
				}
				testResults = append(testResults, result)
				totalCount++
				durationMillis += result.DurationMillis
				switch result.Status {
				case "failed":
					failedCount++
				case "skipped":
					skippedCount++
				default:
					passedCount++
				}
			}
		}

		artifacts = append(artifacts, artifact)
	}

	run := store.Run{
		ID:             runID,
		ProjectID:      project.ID,
		Branch:         branch,
		CommitSHA:      strings.TrimSpace(r.FormValue("commit_sha")),
		BuildID:        strings.TrimSpace(r.FormValue("build_id")),
		BuildURL:       strings.TrimSpace(r.FormValue("build_url")),
		Environment:    strings.TrimSpace(r.FormValue("environment")),
		RunLabel:       runLabel,
		Status:         "complete",
		StartedAt:      startedAt,
		UploadedAt:     uploadedAt,
		TotalCount:     totalCount,
		PassedCount:    passedCount,
		FailedCount:    failedCount,
		SkippedCount:   skippedCount,
		DurationMillis: durationMillis,
	}
	if previousRun != nil {
		run.PreviousRunID = previousRun.ID
	}
	for _, result := range testResults {
		if result.Regression {
			run.NewFailures++
		}
	}

	if parseError != "" {
		run.Status = "failed_import"
		run.TotalCount = 0
		run.PassedCount = 0
		run.FailedCount = 0
		run.SkippedCount = 0
		run.DurationMillis = 0
		testResults = nil
	}

	created, err := s.repo.CreateRun(ctx, store.CreateRunInput{
		Run:           run,
		Artifacts:     artifacts,
		TestResults:   testResults,
		PreviousRunID: run.PreviousRunID,
		NewFailures:   run.NewFailures,
	})
	if err != nil {
		return store.Run{}, err
	}
	return created, nil
}

func (s *Server) render(w http.ResponseWriter, status int, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	component.Render(context.Background(), w)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(value)
}

func parseTimeOrDefault(raw string, fallback time.Time) time.Time {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	if unixSeconds, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(unixSeconds, 0).UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC()
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func sanitizeFileName(value string) string {
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	return value
}

func writeGzipFile(path string, contents []byte) error {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(contents); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func newID() string {
	value := make([]byte, 16)
	rand.Read(value)
	return hex.EncodeToString(value)
}

func (s *Server) loadProject(ctx context.Context, slug string) (store.Project, error) {
	return s.repo.GetProjectBySlug(ctx, slug)
}

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, r)
		log.Printf(
			"testrr: method=%s path=%s status=%d duration=%s remote=%s",
			r.Method,
			r.URL.RequestURI(),
			recorder.status,
			time.Since(started).Round(time.Millisecond),
			r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
