package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ivloli/strapi-doc-center/search-service/internal/document"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meilisearch/meilisearch-go"
)

type config struct {
	host              string
	port              string
	databaseURL       string
	meiliURL          string
	meiliKey          string
	index             string
	syncInterval      time.Duration
	syncBatchSize     int
	syncOnStart       bool
	internalSyncToken string
}

type indexedDocument struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	URL           string `json:"url"`
	SourceVersion string `json:"sourceVersion"`
}

type sourceMetadata struct {
	ID            string
	URL           string
	SourceVersion string
}

type indexedMetadata struct {
	ID            string
	SourceVersion string
}

type syncRequest struct {
	DocID string `json:"docId"`
}

type meiliClient struct {
	client meilisearch.ServiceManager
}

type service struct {
	pool   *pgxpool.Pool
	meili  *meiliClient
	config config
	syncMu sync.Mutex
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		log.Fatalf("connect PostgreSQL: %v", err)
	}
	defer pool.Close()

	svc := &service{
		pool:   pool,
		meili:  newMeiliClient(cfg.meiliURL, cfg.meiliKey),
		config: cfg,
	}
	if err := svc.ensureIndex(ctx); err != nil {
		log.Fatalf("configure Meilisearch: %v", err)
	}
	if cfg.syncOnStart {
		if err := svc.sync(ctx); err != nil {
			log.Printf("initial reconciliation failed: %v", err)
		}
	}

	go svc.syncLoop(ctx)
	server := &http.Server{
		Addr:              cfg.host + ":" + cfg.port,
		Handler:           svc.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("search service listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func loadConfig() (config, error) {
	interval, err := time.ParseDuration(env("SYNC_INTERVAL", "1h"))
	if err != nil || interval <= 0 {
		return config{}, fmt.Errorf("SYNC_INTERVAL must be a positive duration")
	}
	batchSize, err := strconv.Atoi(env("SYNC_BATCH_SIZE", "500"))
	if err != nil || batchSize < 1 || batchSize > 5000 {
		return config{}, fmt.Errorf("SYNC_BATCH_SIZE must be between 1 and 5000")
	}
	cfg := config{
		host:              env("SEARCH_HOST", "127.0.0.1"),
		port:              env("SEARCH_PORT", "8080"),
		databaseURL:       os.Getenv("DATABASE_URL"),
		meiliURL:          strings.TrimRight(os.Getenv("MEILI_URL"), "/"),
		meiliKey:          os.Getenv("MEILI_API_KEY"),
		index:             env("MEILI_INDEX", "docs_public"),
		syncInterval:      interval,
		syncBatchSize:     batchSize,
		syncOnStart:       env("SYNC_ON_START", "true") != "false",
		internalSyncToken: os.Getenv("SEARCH_SYNC_TOKEN"),
	}
	if cfg.databaseURL == "" || cfg.meiliURL == "" || cfg.meiliKey == "" {
		return config{}, fmt.Errorf("DATABASE_URL, MEILI_URL, and MEILI_API_KEY are required")
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func newMeiliClient(baseURL, key string) *meiliClient {
	return &meiliClient{client: meilisearch.New(baseURL, meilisearch.WithAPIKey(key))}
}

func (s *service) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("POST /internal/sync", s.internalSync)
	return mux
}

func (s *service) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *service) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	page := positiveInt(r.URL.Query().Get("page"), 1, 1, 100000)
	pageSize := positiveInt(r.URL.Query().Get("pageSize"), 20, 1, 100)

	result, err := s.meili.search(r.Context(), s.config.index, query, page, pageSize)
	if err != nil {
		log.Printf("search failed: %v", err)
		http.Error(w, "search is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	total := number(result["totalHits"])
	if total == 0 {
		total = number(result["estimatedTotalHits"])
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hits":       result["hits"],
		"page":       page,
		"pageSize":   pageSize,
		"totalHits":  total,
		"totalPages": (total + pageSize - 1) / pageSize,
	})
}

func (s *service) internalSync(w http.ResponseWriter, r *http.Request) {
	if s.config.internalSyncToken == "" {
		http.NotFound(w, r)
		return
	}
	providedToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(s.config.internalSyncToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var request syncRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.DocID) == "" {
		http.Error(w, "docId is required", http.StatusBadRequest)
		return
	}
	if err := s.syncDocument(r.Context(), strings.TrimSpace(request.DocID)); err != nil {
		log.Printf("incremental sync for %q failed: %v", request.DocID, err)
		http.Error(w, "sync failed", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "synced"})
}

func (s *service) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.sync(ctx); err != nil {
				log.Printf("reconciliation failed: %v", err)
			}
		}
	}
}

// sync compares lightweight source metadata first; unchanged documents never have their body read.
func (s *service) sync(ctx context.Context) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.syncLocked(ctx)
}

func (s *service) syncLocked(ctx context.Context) error {
	source, err := s.readMetadata(ctx, nil)
	if err != nil {
		return err
	}
	indexed, err := s.meili.documentMetadata(ctx, s.config.index)
	if err != nil {
		return err
	}

	changedIDs := make([]string, 0)
	for id, metadata := range source {
		if indexedMetadata, found := indexed[id]; !found || indexedMetadata.SourceVersion != metadata.SourceVersion {
			changedIDs = append(changedIDs, id)
		}
	}
	documents, err := s.readDocuments(ctx, changedIDs)
	if err != nil {
		return err
	}
	if err := s.upsertBatches(ctx, documents); err != nil {
		return err
	}

	staleIDs := make([]string, 0)
	for id := range indexed {
		if _, found := source[id]; !found {
			staleIDs = append(staleIDs, id)
		}
	}
	if len(staleIDs) > 0 {
		if err := s.meili.deleteDocuments(ctx, s.config.index, staleIDs); err != nil {
			return err
		}
	}
	log.Printf("reconciliation complete: %d public documents, %d changed, %d stale removed", len(source), len(documents), len(staleIDs))
	return nil
}

// syncDocument is idempotent: an absent or non-public source document becomes a delete operation.
func (s *service) syncDocument(ctx context.Context, docID string) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	source, err := s.readMetadata(ctx, []string{docID})
	if err != nil {
		return err
	}
	if _, found := source[docID]; !found {
		return s.meili.deleteDocuments(ctx, s.config.index, []string{docID})
	}
	documents, err := s.readDocuments(ctx, []string{docID})
	if err != nil {
		return err
	}
	return s.upsertBatches(ctx, documents)
}

func (s *service) upsertBatches(ctx context.Context, documents []indexedDocument) error {
	for start := 0; start < len(documents); start += s.config.syncBatchSize {
		end := start + s.config.syncBatchSize
		if end > len(documents) {
			end = len(documents)
		}
		if err := s.meili.upsertDocuments(ctx, s.config.index, documents[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (s *service) readMetadata(ctx context.Context, docIDs []string) (map[string]sourceMetadata, error) {
	query := `
SELECT d.doc_id, d.updated_at, m.value, m.updated_at
FROM docs d
JOIN menus m ON m.doc_id = d.doc_id
WHERE d.published_at IS NOT NULL
  AND m.published_at IS NOT NULL`
	args := []any{}
	if docIDs != nil {
		query += "\n  AND d.doc_id = ANY($1::text[])"
		args = append(args, docIDs)
	}
	query += "\nORDER BY d.doc_id"
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read source metadata: %w", err)
	}
	defer rows.Close()

	metadata := make(map[string]sourceMetadata)
	for rows.Next() {
		var id, url pgtype.Text
		var docUpdatedAt, menuUpdatedAt pgtype.Timestamptz
		if err := rows.Scan(&id, &docUpdatedAt, &url, &menuUpdatedAt); err != nil {
			return nil, fmt.Errorf("scan source metadata: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" {
			log.Printf("skip source document without docId")
			continue
		}
		if !url.Valid || strings.TrimSpace(url.String) == "" {
			log.Printf("skip source document %q without a menu URL", id.String)
			continue
		}
		metadata[id.String] = sourceMetadata{
			ID:            id.String,
			URL:           url.String,
			SourceVersion: sourceVersion(id.String, timestampValue(docUpdatedAt), url.String, timestampValue(menuUpdatedAt)),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source metadata: %w", err)
	}
	return metadata, nil
}

func (s *service) readDocuments(ctx context.Context, docIDs []string) ([]indexedDocument, error) {
	if len(docIDs) == 0 {
		return nil, nil
	}
	sort.Strings(docIDs)
	const query = `
SELECT d.doc_id, d.title, d.content, d.updated_at, m.value, m.updated_at
FROM docs d
JOIN menus m ON m.doc_id = d.doc_id
WHERE d.published_at IS NOT NULL
  AND m.published_at IS NOT NULL
  AND d.doc_id = ANY($1::text[])
ORDER BY d.doc_id`
	rows, err := s.pool.Query(ctx, query, docIDs)
	if err != nil {
		return nil, fmt.Errorf("read changed documents: %w", err)
	}
	defer rows.Close()

	documents := make([]indexedDocument, 0, len(docIDs))
	for rows.Next() {
		var id, title, content, url pgtype.Text
		var docUpdatedAt, menuUpdatedAt pgtype.Timestamptz
		if err := rows.Scan(&id, &title, &content, &docUpdatedAt, &url, &menuUpdatedAt); err != nil {
			return nil, fmt.Errorf("scan changed document: %w", err)
		}
		if !id.Valid || strings.TrimSpace(id.String) == "" || !url.Valid || strings.TrimSpace(url.String) == "" {
			continue
		}
		documents = append(documents, indexedDocument{
			ID:            id.String,
			Title:         textValue(title),
			Content:       document.PlainText(textValue(content)),
			URL:           url.String,
			SourceVersion: sourceVersion(id.String, timestampValue(docUpdatedAt), url.String, timestampValue(menuUpdatedAt)),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate changed documents: %w", err)
	}
	return documents, nil
}

func sourceVersion(docID string, docUpdatedAt *string, url string, menuUpdatedAt *string) string {
	// JSON keeps null distinct from an empty string and gives every version a stable field order.
	payload, _ := json.Marshal(struct {
		DocID         string  `json:"docId"`
		DocUpdatedAt  *string `json:"docUpdatedAt"`
		URL           string  `json:"url"`
		MenuUpdatedAt *string `json:"menuUpdatedAt"`
	}{docID, docUpdatedAt, url, menuUpdatedAt})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func timestampValue(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func (s *service) ensureIndex(ctx context.Context) error {
	if err := s.meili.createIndex(ctx, s.config.index); err != nil {
		return err
	}
	return s.meili.configureIndex(ctx, s.config.index)
}

func (m *meiliClient) createIndex(ctx context.Context, index string) error {
	if _, err := m.client.GetIndexWithContext(ctx, index); err == nil {
		return nil
	}
	task, err := m.client.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{Uid: index, PrimaryKey: "id"})
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	return m.waitTask(ctx, task)
}

func (m *meiliClient) configureIndex(ctx context.Context, index string) error {
	task, err := m.client.Index(index).UpdateSettingsWithContext(ctx, &meilisearch.Settings{
		SearchableAttributes: []string{"title", "content"},
		DisplayedAttributes:  []string{"id", "title", "content", "url", "sourceVersion"},
	})
	if err != nil {
		return fmt.Errorf("configure index: %w", err)
	}
	return m.waitTask(ctx, task)
}

func (m *meiliClient) upsertDocuments(ctx context.Context, index string, documents []indexedDocument) error {
	if len(documents) == 0 {
		return nil
	}
	task, err := m.client.Index(index).AddDocumentsWithContext(ctx, documents, nil)
	if err != nil {
		return fmt.Errorf("upsert documents: %w", err)
	}
	return m.waitTask(ctx, task)
}

func (m *meiliClient) documentMetadata(ctx context.Context, index string) (map[string]indexedMetadata, error) {
	metadata := make(map[string]indexedMetadata)
	for offset := 0; ; offset += 1000 {
		var result meilisearch.DocumentsResult
		err := m.client.Index(index).GetDocumentsWithContext(ctx, &meilisearch.DocumentsQuery{
			Fields: []string{"id", "sourceVersion"},
			Limit:  1000,
			Offset: int64(offset),
		}, &result)
		if err != nil {
			return nil, fmt.Errorf("list indexed documents: %w", err)
		}
		for _, item := range result.Results {
			var entry indexedMetadata
			if raw, found := item["id"]; found {
				_ = json.Unmarshal(raw, &entry.ID)
			}
			if raw, found := item["sourceVersion"]; found {
				_ = json.Unmarshal(raw, &entry.SourceVersion)
			}
			if entry.ID != "" {
				metadata[entry.ID] = entry
			}
		}
		if len(result.Results) < 1000 {
			break
		}
	}
	return metadata, nil
}

func (m *meiliClient) deleteDocuments(ctx context.Context, index string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	task, err := m.client.Index(index).DeleteDocumentsWithContext(ctx, ids, nil)
	if err != nil {
		return fmt.Errorf("delete stale documents: %w", err)
	}
	return m.waitTask(ctx, task)
}

func (m *meiliClient) search(ctx context.Context, index, query string, page, pageSize int) (map[string]any, error) {
	raw, err := m.client.Index(index).SearchRawWithContext(ctx, query, &meilisearch.SearchRequest{
		Offset:                int64((page - 1) * pageSize),
		Limit:                 int64(pageSize),
		AttributesToHighlight: []string{"title", "content"},
		AttributesToCrop:      []string{"content:180"},
		HighlightPreTag:       "<mark>",
		HighlightPostTag:      "</mark>",
		CropMarker:            "...",
	})
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(*raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (m *meiliClient) waitTask(ctx context.Context, task *meilisearch.TaskInfo) error {
	if task == nil || task.TaskUID == 0 {
		return fmt.Errorf("Meilisearch did not return taskUid")
	}
	completed, err := m.client.WaitForTaskWithContext(ctx, task.TaskUID, 100*time.Millisecond)
	if err != nil {
		return err
	}
	if completed.Status != meilisearch.TaskStatusSucceeded {
		return fmt.Errorf("Meilisearch task %d %s: %v", task.TaskUID, completed.Status, completed.Error)
	}
	return nil
}

func positiveInt(value string, fallback, min, max int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < min || parsed > max {
		return fallback
	}
	return parsed
}

func number(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case json.Number:
		parsed, _ := number.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
