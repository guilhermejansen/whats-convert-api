package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	swaggerFiles "github.com/swaggo/files"
	httpSwagger "github.com/swaggo/http-swagger"

	"whats-convert-api/internal/config"
	"whats-convert-api/internal/handlers"
	"whats-convert-api/internal/pool"
	"whats-convert-api/internal/services"
)

// Server represents the HTTP server
type Server struct {
	app            *fiber.App
	config         *config.Config
	workerPool     *pool.WorkerPool
	bufferPool     *pool.BufferPool
	downloader     *services.Downloader
	audioConverter *services.AudioConverter
	imageConverter *services.ImageConverter
	handler        *handlers.ConverterHandler
	s3Service      *services.S3Service
	uploadManager  *services.UploadManager
	s3Handler      *handlers.S3Handler
	webHandler     *handlers.WebHandler
	metaHandler    *handlers.MetaHandler
}

// New creates a new server instance
func New(cfg *config.Config) *Server {
	if cfg == nil {
		cfg = config.Load()
	}

	// Set runtime optimizations
	runtime.GOMAXPROCS(runtime.NumCPU())
	debug.SetGCPercent(cfg.GOGC)
	debug.SetMemoryLimit(int64(1024 * 1024 * 1024)) // 1GB memory limit

	return &Server{
		config: cfg,
	}
}

// Initialize sets up all server components
func (s *Server) Initialize() error {
	// Initialize buffer pool
	log.Printf("Initializing buffer pool with %d buffers of %d bytes", s.config.BufferPoolSize, s.config.BufferSize)
	s.bufferPool = pool.NewBufferPool(s.config.BufferPoolSize, s.config.BufferSize)

	// Initialize worker pool
	log.Printf("Initializing worker pool with %d workers", s.config.MaxWorkers)
	s.workerPool = pool.NewWorkerPool(s.config.MaxWorkers)
	if err := s.workerPool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Initialize downloader
	s.downloader = services.NewDownloader(s.bufferPool, int64(s.config.BodyLimit))

	// Initialize converters
	s.audioConverter = services.NewAudioConverter(s.workerPool, s.bufferPool, s.downloader)
	s.imageConverter = services.NewImageConverter(s.workerPool, s.bufferPool, s.downloader)

	// Initialize handler
	s.handler = handlers.NewConverterHandler(s.audioConverter, s.imageConverter, s.config.RequestTimeout)

	// Initialize S3 services if enabled
	if s.config.S3.Enabled {
		log.Println("Initializing S3 services...")

		s3Service, err := services.NewS3Service(s.config.S3)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 service: %w", err)
		}
		s.s3Service = s3Service

		// Initialize upload manager
		s.uploadManager = services.NewUploadManager(s.s3Service, s.config.S3.MaxConcurrentUploads)

		// Initialize S3 handler
		s.s3Handler = handlers.NewS3Handler(s.s3Service, s.uploadManager)
	}

	// Initialize web handler
	webHandler, err := handlers.NewWebHandler()
	if err != nil {
		return fmt.Errorf("failed to initialize web handler: %w", err)
	}
	s.webHandler = webHandler

	// Initialize metadata handler with API version
	s.metaHandler = handlers.NewMetaHandler(readAPIVersion(), s.s3Handler != nil)

	// Initialize Fiber app with v3 config
	s.app = fiber.New(fiber.Config{
		ServerHeader:     "MediaConverter",
		StrictRouting:    true,
		CaseSensitive:    true,
		AppName:          "WhatsApp Media Converter API",
		BodyLimit:        s.config.BodyLimit,
		ReadTimeout:      s.config.ReadTimeout,
		WriteTimeout:     s.config.WriteTimeout,
		IdleTimeout:      s.config.WriteTimeout,
		ReadBufferSize:   16 * 1024, // 16KB
		WriteBufferSize:  16 * 1024, // 16KB
		DisableKeepalive: false,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			message := "Internal Server Error"

			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				message = e.Message
			}

			return c.Status(code).JSON(fiber.Map{
				"error":     message,
				"timestamp": time.Now().Unix(),
			})
		},
	})

	// Setup middleware
	s.setupMiddleware()

	// Setup routes
	s.setupRoutes()

	return nil
}

// setupMiddleware configures all middleware
func (s *Server) setupMiddleware() {
	// Request ID middleware
	s.app.Use(requestid.New(requestid.Config{
		Header: "X-Request-ID",
		Generator: func() string {
			return fmt.Sprintf("%d", time.Now().UnixNano())
		},
	}))

	// Logger middleware (minimal for performance)
	s.app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} ${path}\n",
		TimeFormat: "15:04:05",
	}))

	// CORS middleware
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "X-Request-ID"},
		MaxAge:       86400,
	}))

	// Recover middleware
	s.app.Use(recover.New())
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {

	// Web interface routes
	s.webHandler.RegisterWebRoutes(s.app)

	if s.metaHandler != nil {
		s.app.Get("/api", s.metaHandler.APIInfo)
	}

	// Health check
	s.app.Get("/health", s.handler.Health)
	s.app.Get("/stats", s.handler.Stats)

	// Single conversion endpoints
	s.app.Post("/convert/audio", s.handler.ConvertAudio)
	s.app.Post("/convert/image", s.handler.ConvertImage)

	// Batch conversion endpoints
	s.app.Post("/convert/batch/audio", s.handler.ConvertBatchAudio)
	s.app.Post("/convert/batch/image", s.handler.ConvertBatchImage)

	// S3 upload endpoints (if enabled)
	if s.s3Handler != nil {
		s.s3Handler.RegisterS3Routes(s.app)
	}

	if s.config.EnableSwagger {
		s.registerSwaggerRoutes()
	}

	// 404 handler
	s.app.Use(func(c fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{
			"error": "Endpoint not found",
			"path":  c.Path(),
		})
	})
}

func (s *Server) registerSwaggerRoutes() {
	swaggerFiles.Handler.Prefix = "/swagger"
	s.app.Get("/swagger", func(c fiber.Ctx) error {
		return c.Redirect().Status(fiber.StatusTemporaryRedirect).To("/swagger/index.html")
	})
	s.app.Get("/swagger/postman.json", func(c fiber.Ctx) error {
		return c.SendFile("docs/postman_collection.json")
	})
	s.app.Get("/swagger/*", adaptor.HTTPHandler(httpSwagger.Handler(
		httpSwagger.InstanceName("swagger"),
		httpSwagger.DeepLinking(true),
		httpSwagger.AfterScript(`(function(){
		var attempts=0,maxAttempts=50,styleInjected=false;
		function injectStyle(){
			if(styleInjected){return;}
			var style=document.createElement('style');
			style.textContent='.postman-download-btn{margin-left:12px;padding:8px 14px;border-radius:4px;background:#ef5b25;color:#fff;font-weight:600;text-decoration:none;transition:background .2s;} .postman-download-btn:hover{background:#d64f1f;}';
			document.head.appendChild(style);
			styleInjected=true;
		}
		function addPostmanButton(){
			attempts++;
			var container=document.querySelector('.swagger-ui .topbar .download-url-wrapper');
			if(!container){
				if(attempts<maxAttempts){setTimeout(addPostmanButton,200);}return;
			}
			if(container.querySelector('.postman-download-btn')){return;}
			injectStyle();
			var btn=document.createElement('a');
			btn.className='postman-download-btn';
			btn.href='/swagger/postman.json';
			btn.download='whats-convert-api.postman_collection.json';
			btn.textContent='Download Postman Collection';
			container.appendChild(btn);
		}
		if(document.readyState==='complete'){addPostmanButton();}
		else{window.addEventListener('load',addPostmanButton);} 
	})();`),
	)))
}

// Start starts the server
func (s *Server) Start() error {
	// Print startup information
	s.printStartupInfo()

	// Create shutdown channel
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		addr := fmt.Sprintf(":%s", s.config.Port)
		if err := s.app.Listen(addr); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-shutdownCh

	log.Println("Shutting down server...")
	return s.Shutdown()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown Fiber app
	if err := s.app.ShutdownWithContext(ctx); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}

	// Stop worker pool
	if s.workerPool != nil {
		s.workerPool.Stop()
		log.Println("Worker pool stopped")
	}

	// Close downloader
	if s.downloader != nil {
		s.downloader.Close()
		log.Println("Downloader closed")
	}

	log.Println("Server shutdown complete")
	return nil
}

// printStartupInfo prints server configuration
func (s *Server) printStartupInfo() {
	log.Println("========================================")
	log.Println("WhatsApp Media Converter API")
	log.Println("========================================")
	log.Printf("Port:           %s", s.config.Port)
	log.Printf("Workers:        %d", s.config.MaxWorkers)
	log.Printf("Buffer Pool:    %d x %dMB", s.config.BufferPoolSize, s.config.BufferSize/1024/1024)
	log.Printf("Request Timeout: %s", s.config.RequestTimeout)
	log.Printf("Body Limit:     %dMB", s.config.BodyLimit/1024/1024)
	log.Printf("CPU Cores:      %d", runtime.NumCPU())
	log.Printf("Go Version:     %s", runtime.Version())
	log.Printf("GOGC:           %d", s.config.GOGC)
	log.Printf("Memory Limit:   %s", s.config.GoMemLimit)
	log.Printf("Swagger:        %t", s.config.EnableSwagger)
	log.Println("========================================")
	log.Printf("Ready to handle 1000+ requests/second!")
	log.Println("========================================")
	log.Printf("Author:         Guilherme Jansen")
	log.Printf("Email:          suporte@setupautomatizado.com.br")
	log.Println("========================================")
	log.Printf("GitHub:         https://github.com/guilhermejansen")
	log.Println("========================================")
}

func readAPIVersion() string {
	const fallbackVersion = "1.0.0"
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return fallbackVersion
	}

	version := strings.TrimSpace(string(data))
	if version == "" {
		return fallbackVersion
	}

	return version
}

// GetStats returns server statistics
func (s *Server) GetStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"worker_pool": s.workerPool.Stats(),
		"buffer_pool": s.bufferPool.Stats(),
		"memory": map[string]interface{}{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"num_gc":         m.NumGC,
		},
		"goroutines": runtime.NumGoroutine(),
	}
}
