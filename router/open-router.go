package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// SetOpenRouter registers the third-party balance query API.
//
// The namespace deliberately skips GlobalAPIRateLimit: that limiter is keyed by
// source IP, and every request here arrives from a partner's own backend, so it
// would throttle all of that partner's users as one client. Rate limiting is
// applied on the dimensions that actually isolate tenants — source IP as a
// pre-auth backstop, then per application and per credential.
func SetOpenRouter(router *gin.Engine) {
	openRouter := router.Group("/api/open/v1")
	openRouter.Use(middleware.RouteTag("open_api"))
	openRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	openRouter.Use(middleware.BodyStorageCleanup())
	openRouter.Use(middleware.CORS())
	openRouter.Use(middleware.DisableCache())
	openRouter.Use(middleware.OpenApiEnabled())
	{
		authRoute := openRouter.Group("/auth")
		{
			authRoute.POST("/exchange",
				middleware.OpenExchangeIpBackstop(),
				middleware.AnonymousRequestBodyLimit(),
				middleware.OpenAppAuth(),
				middleware.OpenExchangeRateLimit(),
				controller.OpenExchangeCredential,
			)
			authRoute.POST("/revoke",
				middleware.OpenCredentialAuth(),
				controller.OpenRevokeCredential,
			)
		}
		openRouter.GET("/balance",
			middleware.OpenCredentialAuth(),
			middleware.OpenBalanceRateLimit(),
			controller.OpenGetBalance,
		)
	}
}
