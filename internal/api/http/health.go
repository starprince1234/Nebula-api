package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type DatabasePinger interface {
	PingContext(context.Context) error
}

type CachePinger interface {
	Ping(context.Context) error
}

type HealthDependencies struct {
	Database DatabasePinger
	Cache    CachePinger
}

func (s *Server) live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) ready(c *gin.Context) {
	checks := gin.H{"postgres": "unavailable", "redis": "unavailable"}
	ready := s.health.Database != nil && s.health.Cache != nil
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if s.health.Database != nil {
		if err := s.health.Database.PingContext(ctx); err == nil {
			checks["postgres"] = "ok"
		} else {
			ready = false
		}
	}
	if s.health.Cache != nil {
		if err := s.health.Cache.Ping(ctx); err == nil {
			checks["redis"] = "ok"
		} else {
			ready = false
		}
	}

	status := http.StatusOK
	state := "ok"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "unavailable"
	}
	c.JSON(status, gin.H{"status": state, "checks": checks})
}
