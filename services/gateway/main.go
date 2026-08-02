package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultPort = "8080"
const defaultHost = "127.0.0.1"

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

type serviceInfoResponse struct {
	Service    string `json:"service"`
	APIVersion string `json:"api_version"`
	Status     string `json:"status"`
}

var readinessCheck = func(context.Context) error { return nil }

func newHandler() http.Handler {
	mux := http.NewServeMux()
	health := requireMethod(http.MethodGet, func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, healthResponse{
			Service: "gateway",
			Status:  "ok",
		})
	})

	mux.HandleFunc("/healthz", health)
	mux.HandleFunc("/readyz", requireMethod(http.MethodGet, func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if err := readinessCheck(ctx); err != nil {
			slog.ErrorContext(request.Context(), "gateway readiness check failed",
				"request_id", requestIDFromContext(request.Context()),
				"error", err,
			)
			writeAPIError(writer, request, http.StatusServiceUnavailable, "service_not_ready", "服务暂未就绪")
			return
		}
		writeJSON(writer, http.StatusOK, healthResponse{Service: "gateway", Status: "ok"})
	}))
	mux.HandleFunc("/api/v1", requireMethod(http.MethodGet, func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, serviceInfoResponse{
			Service:    "gateway",
			APIVersion: "v1",
			Status:     "ok",
		})
	}))
	mux.HandleFunc("/api/v1/auth/register", authRegisterHandler)
	mux.HandleFunc("/api/v1/auth/login", authLoginHandler)
	mux.HandleFunc("/api/v1/auth/logout", authLogoutHandler)
	mux.HandleFunc("/api/v1/auth/logout-all", authLogoutAllHandler)
	mux.HandleFunc("/api/v1/auth/sessions", authSessionsHandler)
	mux.HandleFunc("/api/v1/auth/sessions/{sessionID}", authSessionHandler)
	mux.HandleFunc("/api/v1/auth/password", authPasswordHandler)
	mux.HandleFunc("/api/v1/auth/password-reset/request", authPasswordResetRequestHandler)
	mux.HandleFunc("/api/v1/auth/password-reset/confirm", authPasswordResetConfirmHandler)
	mux.HandleFunc("/api/v1/auth/me", authMeHandler)
	mux.HandleFunc("/api/v1/auth/avatar", authAvatarHandler)
	mux.HandleFunc("/api/v1/auth/avatar-frame", authAvatarFrameHandler)
	mux.HandleFunc("/api/v1/auth/api-keys", userAPIKeysHandler)
	mux.HandleFunc("/api/v1/auth/api-keys/{keyID}", userAPIKeyHandler)
	mux.HandleFunc("/api/v1/users/{id}/profile", userProfileHandler)
	mux.HandleFunc("/api/v1/auth/oauth/{provider}/start", oauthStartHandler)
	mux.HandleFunc("/api/v1/auth/oauth/{provider}/callback", oauthCallbackHandler)
	mux.HandleFunc("/api/v1/reports", reportsHandler)
	mux.HandleFunc("/api/v1/favorites", favoritesHandler)
	mux.HandleFunc("/api/v1/follows", followsHandler)
	mux.HandleFunc("/api/v1/notifications", notificationsHandler)
	mux.HandleFunc("/api/v1/notifications/events", notificationEventsHandler)
	mux.HandleFunc("/api/v1/notifications/read-all", notificationsReadAllHandler)
	mux.HandleFunc("/api/v1/notifications/{notificationID}/read", notificationReadHandler)
	mux.HandleFunc("/api/v1/files/presign-upload", objectUploadAuthorizationHandler)
	mux.HandleFunc("/api/v1/files/author-asset", authorInlineAssetHandler)
	mux.HandleFunc("/api/v1/files/frame-asset", frameAssetHandler)
	mux.HandleFunc("/api/v1/author/projects", authorProjectsHandler)
	// 文档路由必须注册在 /api/v1/author/projects/ 之前，否则会被项目前缀路由抢先匹配。
	mux.HandleFunc("/api/v1/author/projects/{projectID}/documents", authorProjectDocumentsHandler)
	mux.HandleFunc("/api/v1/author/projects/{projectID}/documents/", authorProjectDocumentsHandler)
	mux.HandleFunc("/api/v1/author/projects/", authorProjectHandler)
	mux.HandleFunc("/api/v1/search", searchHandler)
	mux.HandleFunc("/api/v1/search/hot", searchHotTermsHandler)
	mux.HandleFunc("/api/v1/admin/reviews", adminReviewsHandler)
	mux.HandleFunc("/api/v1/admin/reviews/", adminReviewActionHandler)
	mux.HandleFunc("/api/v1/admin/search/reindex", searchReindexHandler)
	mux.HandleFunc("/api/v1/admin/stats", adminStatsHandler)
	mux.HandleFunc("/api/v1/admin/users", adminUsersHandler)
	mux.HandleFunc("/api/v1/admin/users/{userID}/ban", adminUserBanHandler)
	mux.HandleFunc("/api/v1/admin/projects", adminProjectsHandler)
	mux.HandleFunc("/api/v1/admin/projects/{projectID}/takedown", adminProjectTakedownHandler)
	mux.HandleFunc("/api/v1/admin/api-keys", adminAPIKeysHandler)
	mux.HandleFunc("/api/v1/admin/api-keys/{keyID}", adminAPIKeyHandler)
	mux.HandleFunc("/api/v1/admin/audit", adminAuditHandler)
	mux.HandleFunc("/api/v1/admin/user-stats", adminUserStatsHandler)
	mux.HandleFunc("/api/v1/admin/reports", adminReportsHandler)
	mux.HandleFunc("/api/v1/admin/reports/{reportID}/{action}", adminReportActionHandler)
	mux.HandleFunc("/api/v1/open/projects", openProjectsHandler)
	mux.HandleFunc("/api/v1/open/projects/{projectID}/submit", openProjectSubmitHandler)
	mux.HandleFunc("/api/v1/open/projects/{projectID}/documents", openProjectDocumentsHandler)
	mux.HandleFunc("/api/v1/open/files/presign", openPresignHandler)
	mux.HandleFunc("/api/v1/projects", requireMethod(http.MethodGet, projectListHandler))
	mux.HandleFunc("/api/v1/projects/{slug}/favorite", projectFavoriteHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/follow", projectFollowHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/share", projectShareHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/view", projectViewHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/collaboration/access", collaborationAccessHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/collaboration/ws", projectCollaborationWebSocketHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/collaborators", projectCollaboratorsHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/collaborators/{userID}", projectCollaboratorHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/resources/{kind}", projectResourceDownloadHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/code", projectCodeTreeHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/code/file", projectCodeFileHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/code/file/download", projectCodeFileDownloadHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/assets", projectInlineAssetHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/assistant", assistantHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments/events", documentCommentEventsHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments/{commentID}/like", documentCommentLikeHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments/{commentID}/replies/{replyID}", documentCommentReplyHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments/{commentID}/replies", documentCommentRepliesHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments/{commentID}", documentCommentHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}/comments", documentCommentsHandler)
	mux.HandleFunc("/api/v1/projects/{slug}/documents", requireMethod(http.MethodGet, documentListHandler))
	mux.HandleFunc("/api/v1/projects/{slug}/documents/{documentSlug}", requireMethod(http.MethodGet, documentDetailHandler))
	mux.HandleFunc("/api/v1/projects/", requireMethod(http.MethodGet, projectDetailHandler))
	mux.HandleFunc("/api/v1/", requireMethod(http.MethodGet, func(writer http.ResponseWriter, request *http.Request) {
		writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "请求的接口不存在")
	}))
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writeAPIError(writer, request, http.StatusNotFound, "route_not_found", "请求的接口不存在")
	})

	logger := slog.Default()
	handler := corsMiddleware(allowedOriginsFromEnvironment(), mux)
	handler = accessLogMiddleware(logger, handler)
	handler = securityHeadersMiddleware(handler)
	return requestIDMiddleware(handler)
}

func listenAddress() string {
	host := os.Getenv("HOST")
	if host == "" {
		host = defaultHost
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	return net.JoinHostPort(host, port)
}

func healthcheckURL() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz"
}

func checkHealth(ctx context.Context, url string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("health endpoint returned " + response.Status)
	}
	return nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := checkHealth(ctx, healthcheckURL()); err != nil {
			slog.Error("gateway healthcheck failed", "error", err)
			os.Exit(1)
		}
		return
	}

	var database *sql.DB
	if databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); databaseURL != "" {
		connectContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		database, err = openMySQLDatabase(connectContext, databaseURL)
		cancel()
		if err != nil {
			slog.Error("gateway database initialization failed", "error", err)
			os.Exit(1)
		}
		defer database.Close()
		commentRepositoryStore = newMySQLCommentRepository(database)
		authRepositoryStore = newMySQLAuthRepository(database)
		favoriteRepositoryStore = newMySQLFavoriteRepository(database)
		followRepositoryStore = newMySQLFollowRepository(database)
		managedProjectRepositoryStore = newMySQLManagedProjectRepository(database)
		collaborationRepositoryStore = newMySQLCollaborationRepository(database)
		notificationRepositoryStore = newMySQLNotificationRepository(database)
		projectDocumentRepositoryStore = newMySQLProjectDocumentRepository(database)
		commentLikeRepositoryStore = newMySQLCommentLikeRepository(database)
		projectMetricsRepositoryStore = newMySQLProjectMetricsRepository(database)
		banRepositoryStore = newMySQLBanRepository(database)
		apiKeyRepositoryStore = newMySQLAPIKeyRepository(database)
		adminAuditRepositoryStore = newMySQLAdminAuditRepository(database)
		contentReportRepositoryStore = newMySQLContentReportRepository(database)
		readinessCheck = database.PingContext
		slog.Info("mysql comment repository enabled")
	}

	if redisURL := strings.TrimSpace(os.Getenv("REDIS_URL")); redisURL != "" {
		options, err := redis.ParseURL(redisURL)
		if err != nil {
			slog.Error("gateway redis configuration is invalid", "error", err)
			os.Exit(1)
		}
		redisClient := redis.NewClient(options)
		connectContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = redisClient.Ping(connectContext).Err()
		cancel()
		if err != nil {
			_ = redisClient.Close()
			slog.Error("gateway redis initialization failed", "error", err)
			os.Exit(1)
		}
		defer redisClient.Close()
		authRateLimiter = newRedisAuthLimiter(redisClient)
		previousReadinessCheck := readinessCheck
		readinessCheck = func(ctx context.Context) error {
			if err := previousReadinessCheck(ctx); err != nil {
				return err
			}
			return redisClient.Ping(ctx).Err()
		}
		slog.Info("redis authentication rate limiter enabled")
	}

	if smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST")); smtpHost != "" {
		smtpPort := 465
		if rawPort := strings.TrimSpace(os.Getenv("SMTP_PORT")); rawPort != "" {
			parsedPort, err := strconv.Atoi(rawPort)
			if err != nil {
				slog.Error("gateway SMTP port is invalid", "error", err)
				os.Exit(1)
			}
			smtpPort = parsedPort
		}
		if strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")) == "" {
			slog.Error("gateway password reset requires PUBLIC_BASE_URL")
			os.Exit(1)
		}
		delivery, err := newSMTPPasswordResetDelivery(
			smtpHost, smtpPort, os.Getenv("SMTP_USERNAME"),
			os.Getenv("SMTP_PASSWORD"), os.Getenv("SMTP_FROM"),
		)
		if err != nil {
			slog.Error("gateway SMTP configuration is invalid", "error", err)
			os.Exit(1)
		}
		passwordResetDeliveryStore = delivery
		slog.Info("SMTP password reset delivery enabled", "host", smtpHost, "port", smtpPort)
	}

	if accessKeyID := strings.TrimSpace(os.Getenv("OSS_ACCESS_KEY_ID")); accessKeyID != "" {
		storage, err := newAliyunObjectStorage(
			os.Getenv("OSS_REGION"), os.Getenv("OSS_ENDPOINT"), os.Getenv("OSS_BUCKET"),
			accessKeyID, os.Getenv("OSS_ACCESS_KEY_SECRET"),
		)
		if err != nil {
			slog.Error("gateway OSS configuration is invalid", "error", err)
			os.Exit(1)
		}
		objectStorageStore = storage
		slog.Info("Aliyun OSS object storage enabled",
			"region", os.Getenv("OSS_REGION"), "bucket", os.Getenv("OSS_BUCKET"))
	}

	if elasticURL := strings.TrimSpace(os.Getenv("ELASTICSEARCH_URL")); elasticURL != "" {
		indexName := strings.TrimSpace(os.Getenv("ELASTICSEARCH_INDEX"))
		if indexName == "" {
			indexName = "open-resouce-documents"
		}
		searchIndexStore = newElasticSearchIndex(
			elasticURL, indexName,
			os.Getenv("ELASTICSEARCH_USERNAME"), os.Getenv("ELASTICSEARCH_PASSWORD"),
		)
		// 建索引失败不阻断启动：搜索是附加能力，不应拖垮整个服务。
		warmSearchIndex()
		slog.Info("Elasticsearch document search enabled", "index", indexName)
	}

	// AI 项目助手：仅当配置了 ANTHROPIC_API_KEY 时启用。
	// 未配置时 aiAssistantStore 保持为 nil，助手接口返回 503 assistant_unavailable。
	if apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); apiKey != "" {
		baseURL := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL"))
		if baseURL == "" {
			baseURL = defaultAnthropicBaseURL
		}
		model := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
		if model == "" {
			model = defaultAnthropicModel
		}
		aiAssistantStore = newAnthropicAssistant(baseURL, apiKey, model)
		slog.Info("Anthropic AI assistant enabled", "model", model)
	}

	// IP 归属地。只配 v4 也能工作（IPv6 来源的归属地为空）；
	// 两个都不配时整个功能静默降级，不影响评论与登录。
	v4DBPath := strings.TrimSpace(os.Getenv("IP_REGION_V4_DB_PATH"))
	v6DBPath := strings.TrimSpace(os.Getenv("IP_REGION_V6_DB_PATH"))
	if v4DBPath != "" || v6DBPath != "" {
		resolver, err := newIP2RegionResolver(v4DBPath, v6DBPath)
		if err != nil {
			// 不退出：归属地只是一个展示字段，数据文件缺失或损坏
			// 不应让整个网关启动失败。
			slog.Warn("IP region database unavailable, region display disabled", "error", err)
		} else {
			ipRegionResolverStore = resolver
			slog.Info("IP region resolution enabled",
				"ipv4", v4DBPath != "", "ipv6", v6DBPath != "")
		}
	}

	server := &http.Server{
		Addr:              listenAddress(),
		Handler:           newHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-shutdownSignal.Done()

		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownContext); err != nil {
			slog.Error("gateway shutdown failed", "error", err)
		}
		// 连接排完后再等待在途的 best-effort 写入（加经验、写索引、
		// 累计指标），否则重启刚好撞上时这些已经发生的动作会默默丢失。
		waitForBackgroundTasks()
	}()

	slog.Info("gateway listening", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("gateway stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}
