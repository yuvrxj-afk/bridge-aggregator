package main

import (
	"bridge-aggregator/internal/api"

	"github.com/gin-gonic/gin"
)

func main() {

	r := gin.Default()

	r.POST("/quote", api.QuoteHandler)
	r.GET("/", func(ctx *gin.Context) {
		ctx.JSON(200,gin.H{"status":"OK"})
	})

	r.Run(":8080")
}