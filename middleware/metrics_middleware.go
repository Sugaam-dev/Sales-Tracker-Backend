package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

// MetricsMiddleware measures HTTP request duration and records database pool utilization metrics.
func MetricsMiddleware(log *slog.Logger, pool *pgxpool.Pool, gormDB *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		// Process request
		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		// Collect pgxpool metrics if available
		var pgxAcquired, pgxIdle, pgxTotal int32
		if pool != nil {
			stat := pool.Stat()
			pgxAcquired = stat.AcquiredConns()
			pgxIdle = stat.IdleConns()
			pgxTotal = stat.TotalConns()
		}

		// Collect GORM / database/sql metrics if available
		var sqlOpen, sqlInUse, sqlIdle int
		if gormDB != nil {
			if sqlDB, err := gormDB.DB(); err == nil {
				dbStats := sqlDB.Stats()
				sqlOpen = dbStats.OpenConnections
				sqlInUse = dbStats.InUse
				sqlIdle = dbStats.Idle
			}
		}

		// Log request telemetry
		log.Info("http_request_telemetry",
			"method", method,
			"path", path,
			"status", status,
			"duration_ms", duration.Milliseconds(),
			"duration", duration.String(),
			"pgx_acquired", pgxAcquired,
			"pgx_idle", pgxIdle,
			"pgx_total", pgxTotal,
			"sql_open", sqlOpen,
			"sql_in_use", sqlInUse,
			"sql_idle", sqlIdle,
		)
	}
}
