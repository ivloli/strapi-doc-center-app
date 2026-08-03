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
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/ivloli/strapi-doc-center/search-service/docs"
	"github.com/ivloli/strapi-doc-center/search-service/internal/document"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/meilisearch/meilisearch-go"
	httpSwagger "github.com/swaggo/http-swagger"
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
	DocID         string `json:"docId"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	URL           string `json:"url"`
	SourceVersion string `json:"sourceVersion"`
}

type sourceMetadata struct {
	ID            string
	DocID         string
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

type searchListRequest struct {
	Keyword    string            `json:"keyword" example:"快速开始"`
	Pagination paginationRequest `json:"pagination"`
}

type paginationRequest struct {
	Page     int `json:"page" example:"1"`
	PageSize int `json:"pageSize" example:"20"`
	Total    int `json:"total,omitempty" example:"0"`
}

type healthData struct {
	Status string `json:"status" example:"ok"`
}

type healthResponse struct {
	Code    int        `json:"code" example:"200"`
	Message string     `json:"message" example:"success"`
	Data    healthData `json:"data"`
}

type errorResponse struct {
	Code    int    `json:"code" example:"400"`
	Message string `json:"message" example:"keyword is required"`
	Data    any    `json:"data"`
}

type syncData struct {
	Status string `json:"status" example:"synced"`
}

type syncResponse struct {
	Code    int      `json:"code" example:"200"`
	Message string   `json:"message" example:"success"`
	Data    syncData `json:"data"`
}

type formattedSearchHit struct {
	Title   string `json:"title" example:"<mark>快速</mark>开始"`
	Content string `json:"content" example:"...使用 <mark>快速</mark>开始完成配置..."`
}

type meiliSearchHit struct {
	ID        string              `json:"id"`
	DocID     string              `json:"docId"`
	Title     string              `json:"title"`
	Content   string              `json:"content"`
	Formatted *formattedSearchHit `json:"_formatted"`
}

type searchHighlight struct {
	Title   string `json:"title" example:"<mark>快速</mark>开始"`
	Summary string `json:"summary" example:"...使用 <mark>快速</mark>开始完成配置..."`
}

type searchHit struct {
	ID        string          `json:"id" example:"doc_9fd2d776b6a4521d685e27ede29f052003c8353455ec5341b3831089f14e1220"`
	DocID     string          `json:"docId" example:"test-kirito"`
	Title     string          `json:"title" example:"快速开始"`
	Path      string          `json:"path" example:"/test-kirito"`
	URL       string          `json:"url" example:"https://help.test.starviewcloud.com/test-kirito"`
	Summary   string          `json:"summary" example:"...使用快速开始完成配置..."`
	Highlight searchHighlight `json:"highlight"`
}

type pagination struct {
	Page     int    `json:"page" example:"1"`
	PageSize int    `json:"pageSize" example:"20"`
	Total    string `json:"total" example:"7"`
}

type searchData struct {
	List       []searchHit `json:"list"`
	Pagination pagination  `json:"pagination"`
}

type searchResponse struct {
	Code    int        `json:"code" example:"200"`
	Message string     `json:"message" example:"success"`
	Data    searchData `json:"data"`
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

// @title 文档中心搜索服务 API
// @version 1.0
// @description 面向公开已发布文档的全文搜索与索引同步服务。
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 内部同步接口使用 Bearer Token 鉴权。
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

// loadConfig 从环境变量加载服务运行、Meilisearch 和同步配置。
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

// env 读取环境变量；未设置时使用给定默认值。
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// newMeiliClient 创建使用受限 API Key 的 Meilisearch 客户端。
func newMeiliClient(baseURL, key string) *meiliClient {
	return &meiliClient{client: meilisearch.New(baseURL, meilisearch.WithAPIKey(key))}
}

// routes 注册公开搜索、健康检查、内部同步和 Swagger UI 路由。
func (s *service) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /search/list", s.search)
	mux.HandleFunc("POST /internal/sync", s.internalSync)
	mux.Handle("GET /swagger/", httpSwagger.WrapHandler)
	return mux
}

// health 返回服务进程健康状态。
// @Summary 健康检查
// @Description 用于负载均衡与监控系统检测 Go 服务是否存活。
// @Tags 系统
// @Produce json
// @Success 200 {object} healthResponse
// @Router /healthz [get]
func (s *service) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Code:    http.StatusOK,
		Message: "success",
		Data:    healthData{Status: "ok"},
	})
}

// search 查询 Meilisearch 并标准化分页响应。
// @Summary 搜索公开文档
// @Description 在标题和正文中搜索已发布且具有已发布菜单路径的文档。
// @Tags 搜索
// @Accept json
// @Produce json
// @Param request body searchListRequest true "搜索条件与分页参数"
// @Success 200 {object} searchResponse
// @Failure 400 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /search/list [post]
func (s *service) search(w http.ResponseWriter, r *http.Request) {
	var request searchListRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	query := strings.TrimSpace(request.Keyword)
	if query == "" {
		writeError(w, http.StatusBadRequest, "keyword is required")
		return
	}
	page := boundedInt(request.Pagination.Page, 1, 1, 100000)
	pageSize := boundedInt(request.Pagination.PageSize, 20, 1, 100)

	result, err := s.meili.search(r.Context(), s.config.index, query, page, pageSize)
	if err != nil {
		log.Printf("search failed: %v", err)
		writeError(w, http.StatusServiceUnavailable, "search is temporarily unavailable")
		return
	}
	total := number(result["totalHits"])
	if total == 0 {
		total = number(result["estimatedTotalHits"])
	}
	hits, err := searchResultHits(result["hits"], requestBaseURL(r))
	if err != nil {
		log.Printf("decode search hits: %v", err)
		writeError(w, http.StatusServiceUnavailable, "search result is temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusOK, searchResponse{
		Code:    http.StatusOK,
		Message: "success",
		Data: searchData{
			List: hits,
			Pagination: pagination{
				Page:     page,
				PageSize: pageSize,
				Total:    strconv.Itoa(total),
			},
		},
	})
}

// internalSync 接收 Strapi 生命周期通知，并按 docId 执行幂等单篇同步。
// @Summary 同步单篇文档索引
// @Description 文档存在且公开时写入索引；不存在、下架或菜单不可见时删除索引。仅限 Strapi 内网调用。
// @Tags 内部同步
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body syncRequest true "需要同步的文档标识"
// @Success 200 {object} syncResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Router /internal/sync [post]
func (s *service) internalSync(w http.ResponseWriter, r *http.Request) {
	if s.config.internalSyncToken == "" {
		http.NotFound(w, r)
		return
	}
	providedToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(s.config.internalSyncToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var request syncRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.DocID) == "" {
		writeError(w, http.StatusBadRequest, "docId is required")
		return
	}
	if err := s.syncDocument(r.Context(), strings.TrimSpace(request.DocID)); err != nil {
		log.Printf("incremental sync for %q failed: %v", request.DocID, err)
		writeError(w, http.StatusServiceUnavailable, "sync failed")
		return
	}
	writeJSON(w, http.StatusOK, syncResponse{
		Code:    http.StatusOK,
		Message: "success",
		Data:    syncData{Status: "synced"},
	})
}

// syncLoop 定期执行元数据对账，补偿 Hook 丢失、重启和硬删除场景。
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

// sync 串行执行一次两阶段对账，避免定时任务与 Hook 增量同步并发写入同一索引。
func (s *service) sync(ctx context.Context) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	return s.syncLocked(ctx)
}

// syncLocked 先比较 PG 与 Meilisearch 元数据，仅加载并写入发生变化的全文。
func (s *service) syncLocked(ctx context.Context) error {
	source, err := s.readMetadata(ctx, nil)
	if err != nil {
		return err
	}
	indexed, err := s.meili.documentMetadata(ctx, s.config.index)
	if err != nil {
		return err
	}

	changedDocIDs := make([]string, 0)
	for id, metadata := range source {
		if indexedMetadata, found := indexed[id]; !found || indexedMetadata.SourceVersion != metadata.SourceVersion {
			changedDocIDs = append(changedDocIDs, metadata.DocID)
		}
	}
	documents, err := s.readDocuments(ctx, changedDocIDs)
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

// syncDocument 同步单篇文档；源数据不存在或不公开时将其从索引删除，因此可安全重复调用。
func (s *service) syncDocument(ctx context.Context, docID string) error {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	source, err := s.readMetadata(ctx, []string{docID})
	if err != nil {
		return err
	}
	indexID := documentIndexID(docID)
	if _, found := source[indexID]; !found {
		return s.meili.deleteDocuments(ctx, s.config.index, []string{indexID})
	}
	documents, err := s.readDocuments(ctx, []string{docID})
	if err != nil {
		return err
	}
	return s.upsertBatches(ctx, documents)
}

// upsertBatches 按配置批次提交变更文档，并等待每个 Meilisearch 异步任务完成。
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

// readMetadata 只读取公开文档的标识、更新时间和菜单 URL，用于轻量级差异判断。
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
		indexID := documentIndexID(id.String)
		metadata[indexID] = sourceMetadata{
			ID:            indexID,
			DocID:         id.String,
			URL:           url.String,
			SourceVersion: sourceVersion(id.String, timestampValue(docUpdatedAt), url.String, timestampValue(menuUpdatedAt)),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source metadata: %w", err)
	}
	return metadata, nil
}

// readDocuments 仅为变化的 docId 读取标题和正文，并生成最终索引文档。
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
			ID:            documentIndexID(id.String),
			DocID:         id.String,
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

// sourceVersion 以稳定 JSON 计算 SHA-256，安全区分空字符串与 null 元数据。
func sourceVersion(docID string, docUpdatedAt *string, url string, menuUpdatedAt *string) string {
	payload, _ := json.Marshal(struct {
		DocID         string  `json:"docId"`
		DocUpdatedAt  *string `json:"docUpdatedAt"`
		URL           string  `json:"url"`
		MenuUpdatedAt *string `json:"menuUpdatedAt"`
	}{docID, docUpdatedAt, url, menuUpdatedAt})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// documentIndexID converts an arbitrary Strapi docId into a Meilisearch-safe primary key.
func documentIndexID(docID string) string {
	sum := sha256.Sum256([]byte(docID))
	return "doc_" + hex.EncodeToString(sum[:])
}

// timestampValue 规范化可空时间戳，避免时区格式差异造成无效索引更新。
func timestampValue(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}

// textValue 将 PG 可空文本转换为可安全索引的字符串。
func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// ensureIndex 创建搜索索引并配置标题优先于正文的可搜索字段顺序。
func (s *service) ensureIndex(ctx context.Context) error {
	if err := s.meili.createIndex(ctx, s.config.index); err != nil {
		return err
	}
	return s.meili.configureIndex(ctx, s.config.index)
}

// createIndex 在索引不存在时创建以 id 为主键的 Meilisearch 索引。
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

// configureIndex 写入搜索字段和返回字段配置。
func (m *meiliClient) configureIndex(ctx context.Context, index string) error {
	task, err := m.client.Index(index).UpdateSettingsWithContext(ctx, &meilisearch.Settings{
		SearchableAttributes: []string{"title", "content"},
		DisplayedAttributes:  []string{"id", "docId", "title", "content", "url", "sourceVersion"},
	})
	if err != nil {
		return fmt.Errorf("configure index: %w", err)
	}
	return m.waitTask(ctx, task)
}

// upsertDocuments 将文档新增或覆盖到指定索引。
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

// documentMetadata 分页读取索引中的 id 和 sourceVersion，避免拉取正文参与对账。
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

// deleteDocuments 批量删除下架、删除或菜单不可见的索引文档。
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

// search 调用 Meilisearch 并请求标题高亮和正文裁剪摘要。
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

// waitTask 等待 Meilisearch 异步写操作结束，并将失败任务转换为错误。
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

// positiveInt 解析受范围约束的正整数查询参数，非法值回退为默认值。
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

// boundedInt 将请求体中的整数限制在接口允许范围内，非法值回退为默认值。
func boundedInt(value, fallback, min, max int) int {
	if value < min || value > max {
		return fallback
	}
	return value
}

// number 将 JSON 数值转换为分页计算所需的 int。
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

// searchResultHits 将 Meilisearch 内部命中结果转换为前端可直接使用的文档链接与高亮摘要。
func searchResultHits(value any, baseURL string) ([]searchHit, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var sourceHits []meiliSearchHit
	if err := json.Unmarshal(encoded, &sourceHits); err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(sourceHits))
	for _, source := range sourceHits {
		path := documentPath(source.DocID)
		titleHighlight := source.Title
		summaryHighlight := truncateSummary(source.Content, 180)
		if source.Formatted != nil {
			if source.Formatted.Title != "" {
				titleHighlight = source.Formatted.Title
			}
			if source.Formatted.Content != "" {
				summaryHighlight = source.Formatted.Content
			}
		}
		url := path
		if baseURL != "" {
			url = baseURL + path
		}
		hits = append(hits, searchHit{
			ID:      source.ID,
			DocID:   source.DocID,
			Title:   source.Title,
			Path:    path,
			URL:     url,
			Summary: stripHighlightTags(summaryHighlight),
			Highlight: searchHighlight{
				Title:   titleHighlight,
				Summary: summaryHighlight,
			},
		})
	}
	return hits, nil
}

// requestBaseURL derives the public document host from trusted reverse-proxy headers.
func requestBaseURL(r *http.Request) string {
	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	protocol := firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))
	if protocol == "" {
		protocol = "http"
		if r.TLS != nil {
			protocol = "https"
		}
	}
	return protocol + "://" + host
}

// firstForwardedValue extracts the first proxy value when multiple proxies append headers.
func firstForwardedValue(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

// documentPath encodes the Strapi business identifier as the frontend document route.
func documentPath(docID string) string {
	return "/" + url.PathEscape(docID)
}

// stripHighlightTags keeps the cropped text while removing the only markup emitted by Meilisearch.
func stripHighlightTags(value string) string {
	value = strings.ReplaceAll(value, "<mark>", "")
	return strings.ReplaceAll(value, "</mark>", "")
}

// truncateSummary limits a fallback summary by Unicode code points instead of bytes.
func truncateSummary(value string, limit int) string {
	characters := []rune(value)
	if len(characters) <= limit {
		return value
	}
	return string(characters[:limit]) + "..."
}

// writeJSON 统一写入 JSON 响应及 HTTP 状态码。
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError 以统一 JSON 结构返回客户端可读的错误消息。
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Code: status, Message: message, Data: nil})
}
