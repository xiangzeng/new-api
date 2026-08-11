package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// SetOpenRouter registers the self-service balance query API: a user issues a
// read-only balance key in their profile and reads their own balance with it
// from their own program.
//
// The namespace deliberately skips GlobalAPIRateLimit, which is keyed by source
// IP and sized for browser traffic. Rate limiting here is per balance key, the
// dimension that actually isolates one caller from another.
func SetOpenRouter(router *gin.Engine) {
	openRouter := router.Group("/api/open/v1")
	openRouter.Use(middleware.RouteTag("open_api"))
	openRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	openRouter.Use(middleware.BodyStorageCleanup())
	openRouter.Use(middleware.CORS())
	openRouter.Use(middleware.DisableCache())
	{
		openRouter.POST("/auth/revoke",
			middleware.OpenCredentialAuth(),
			controller.OpenRevokeCredential,
		)
		openRouter.GET("/balance",
			middleware.OpenCredentialAuth(),
			middleware.OpenBalanceRateLimit(),
			controller.OpenGetBalance,
		)
	}
}
