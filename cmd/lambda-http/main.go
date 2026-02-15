package main

// Build the Lambda handler binary:
//   GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/lambda-http

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"

	"resume-backend/internal/bootstrap"
	"resume-backend/internal/shared/config"
)

var (
	initOnce  sync.Once
	initErr   error
	ginLambda *ginadapter.GinLambdaV2
)

func initApp() {
	cfg := config.Load()
	app, err := bootstrap.Build(cfg)
	if err != nil {
		initErr = err
		return
	}
	ginLambda = ginadapter.NewV2(app.Router)
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	initOnce.Do(initApp)
	if initErr != nil {
		log.Printf("bootstrap error: %v", initErr)
		body, _ := json.Marshal(map[string]string{"error": "bootstrap failed"})
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 500,
			Body:       string(body),
			Headers:    map[string]string{"Content-Type": "application/json"},
		}, initErr
	}
	if ginLambda == nil {
		return events.APIGatewayV2HTTPResponse{
			StatusCode: 500,
			Body:       `{"error":"router not initialized"}`,
			Headers:    map[string]string{"Content-Type": "application/json"},
		}, nil
	}
	return ginLambda.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(handler)
}
