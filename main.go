package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

var (
	Version   = "0.0.0"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

type HealthOutput struct {
	Body struct {
		Status string `json:"status" doc:"Health status"`
	}
}

type VersionOutput struct {
	Body struct {
		Version   string `json:"version" doc:"Semantic version"`
		GitCommit string `json:"gitCommit,omitempty" doc:"Git commit hash"`
		BuildTime string `json:"buildTime,omitempty" doc:"Build timestamp (UTC)"`
	}
}

type EchoInput struct {
	Body any `json:"body" doc:"Arbitrary JSON to echo back"`
}

type EchoOutput struct {
	Body any
}

func registerRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "healthz",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
	}, func(_ context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getVersion",
		Method:      http.MethodGet,
		Path:        "/version",
		Summary:     "Build version info",
	}, func(_ context.Context, _ *struct{}) (*VersionOutput, error) {
		out := &VersionOutput{}
		out.Body.Version = Version
		out.Body.GitCommit = GitCommit
		out.Body.BuildTime = BuildTime
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "echo",
		Method:      http.MethodPost,
		Path:        "/echo",
		Summary:     "Echo JSON body",
	}, func(_ context.Context, in *EchoInput) (*EchoOutput, error) {
		return &EchoOutput{Body: in.Body}, nil
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Unikernel Demo", Version))
	registerRoutes(api)

	fmt.Printf("Listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
