package main

import (
	"context"
	"log"
	"os"

	"github.com/adityasatrio/poc-video-encode-streaming-gcp/internal/handler"
	"github.com/adityasatrio/poc-video-encode-streaming-gcp/internal/service"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	ctx := context.Background()

	// Load environment variables
	projectID := os.Getenv("GCP_PROJECT_ID")
	rawBucket := os.Getenv("RAW_BUCKET")
	hlsBucket := os.Getenv("HLS_BUCKET")
	transcoderLocation := os.Getenv("TRANSCODER_LOCATION")
	if transcoderLocation == "" {
		transcoderLocation = "us-central1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if projectID == "" || rawBucket == "" || hlsBucket == "" {
		log.Fatal("Missing required environment variables: GCP_PROJECT_ID, RAW_BUCKET, HLS_BUCKET")
	}

	// Initialize services
	gcsService, err := service.NewGCSService(ctx, projectID, rawBucket)
	if err != nil {
		log.Fatalf("Failed to initialize GCS service: %v", err)
	}
	defer gcsService.Close()

	transcoderService, err := service.NewTranscoderService(ctx, projectID, transcoderLocation, hlsBucket)
	if err != nil {
		log.Fatalf("Failed to initialize Transcoder service: %v", err)
	}
	defer transcoderService.Close()

	// Initialize Echo
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE},
	}))

	// Initialize handlers
	videoHandler := handler.NewVideoHandler(gcsService, transcoderService)

	// Routes
	e.POST("/videos/upload-url", videoHandler.GenerateUploadURL)
	e.POST("/videos/:video_id/transcode", videoHandler.StartTranscode)
	e.GET("/videos/transcode/:job_id/status", videoHandler.GetTranscodeStatus)

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	// Start server
	log.Printf("Starting server on port %s", port)
	log.Fatal(e.Start(":" + port))
}
