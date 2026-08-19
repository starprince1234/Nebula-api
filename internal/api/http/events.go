package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) events(c *gin.Context) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		writeError(c, fmt.Errorf("streaming is unsupported"))
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher.Flush()

	lastID := strings.TrimSpace(c.GetHeader("Last-Event-ID"))
	for {
		events, err := s.cache.ReadEvents(
			c.Request.Context(),
			identity(c).UserID.String(),
			lastID,
			s.config.SSEHeartbeat,
		)
		if err != nil {
			return
		}
		if len(events) == 0 {
			if _, err := fmt.Fprint(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
			continue
		}
		for _, event := range events {
			eventID := strings.ReplaceAll(strings.ReplaceAll(event.ID, "\r", ""), "\n", "")
			eventType := strings.ReplaceAll(strings.ReplaceAll(event.Type, "\r", ""), "\n", "")
			if _, err := fmt.Fprintf(c.Writer, "id: %s\nevent: %s\ndata: %s\n\n", eventID, eventType, event.Data); err != nil {
				return
			}
			lastID = event.ID
		}
		flusher.Flush()
	}
}
