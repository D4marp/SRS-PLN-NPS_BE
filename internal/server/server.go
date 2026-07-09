package server

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/bookify-rooms/backend/internal/config"
	"github.com/bookify-rooms/backend/internal/middleware"
	"github.com/bookify-rooms/backend/internal/utils"
)

type Server struct {
	cfg    *config.Config
	db     *sql.DB
	router *gin.Engine
}

func New(cfg *config.Config, db *sql.DB) *Server {
	s := &Server{cfg: cfg, db: db}
	s.setupRouter()
	return s
}

func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%s", s.cfg.Host, s.cfg.Port)
	s.logListenInfo(addr)
	return s.router.Run(addr)
}

func (s *Server) logListenInfo(addr string) {
	log.Printf("Listening on %s (all interfaces — reachable from LAN)", addr)
	lan := utils.LocalAPIURLs(s.cfg.Port)
	if len(lan) == 0 {
		log.Println("No LAN IPv4 detected; set BASE_URL to this machine's Wi-Fi IP for mobile/uploads")
		return
	}
	log.Println("Mobile app / same Wi-Fi — use one of these as API base URL:")
	for _, u := range lan {
		log.Printf("  → %s", u)
	}
	if utils.BaseURLUsesLoopback(s.cfg.BaseURL) {
		log.Printf("WARNING: BASE_URL=%s — phones cannot reach localhost; set BASE_URL to a LAN URL above", s.cfg.BaseURL)
	} else if !strings.Contains(s.cfg.BaseURL, ":"+s.cfg.Port) {
		log.Printf("NOTE: BASE_URL=%s (used for upload URLs in API responses)", s.cfg.BaseURL)
	}
}

func (s *Server) setupRouter() {
	r := gin.Default()

	r.Use(middleware.CORS(s.cfg.AllowedOrigins))
	r.Static("/uploads", s.cfg.UploadsDir)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "bookify-rooms-backend",
			"baseUrl": s.cfg.BaseURL,
			"lanUrls": utils.LocalAPIURLs(s.cfg.Port),
		})
	})

	s.registerRoutes(r)
	s.router = r
}
