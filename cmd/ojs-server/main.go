package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/openjobspec/ojs-backend-lite/internal/core"
	"github.com/openjobspec/ojs-backend-lite/internal/events"
	ojsgrpc "github.com/openjobspec/ojs-backend-lite/internal/grpc"
	"github.com/openjobspec/ojs-backend-lite/internal/memory"
	"github.com/openjobspec/ojs-backend-lite/internal/scheduler"
	"github.com/openjobspec/ojs-backend-lite/internal/server"
)

func main() {
	cfg := server.LoadConfig()
	if cfg.APIKey == "" && !cfg.AllowInsecureNoAuth {
		slog.Error("refusing to start without API authentication", "hint", "set OJS_API_KEY or OJS_ALLOW_INSECURE_NO_AUTH=true for local development")
		os.Exit(1)
	}

	// Create in-memory backend
	var opts []memory.Option
	opts = append(opts, memory.WithRetention(cfg.RetentionCompleted, cfg.RetentionCancelled, cfg.RetentionDiscarded))
	if cfg.PersistPath != "" {
		opts = append(opts, memory.WithPersist(cfg.PersistPath))
	}
	backend, err := memory.New(opts...)
	if err != nil {
		slog.Error("failed to initialize backend", "error", err)
		os.Exit(1)
	}
	defer backend.Close()

	// Start background scheduler
	sched := scheduler.New(backend)
	sched.Start()
	defer sched.Stop()

	// Initialize event broker
	broker := events.NewBroker()
	defer broker.Close()

	// Create HTTP server with real-time support
	router := server.NewRouterWithRealtime(backend, cfg, broker, broker)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Start HTTP server
	go func() {
		slog.Info("OJS HTTP server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	// Start gRPC server
	grpcServer := grpc.NewServer()
	ojsgrpc.Register(grpcServer, backend, ojsgrpc.WithEventSubscriber(broker))
	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			slog.Error("failed to listen for gRPC", "port", cfg.GRPCPort, "error", err)
			os.Exit(1)
		}
		slog.Info("OJS gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	// Print startup banner
	printBanner(cfg)

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	grpcServer.GracefulStop()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("server stopped")
}

func printBanner(cfg server.Config) {
	banner := `
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║      ██████╗      ██╗███████╗      ██╗     ██╗████████╗███████╗
║     ██╔═══██╗     ██║██╔════╝      ██║     ██║╚══██╔══╝██╔════╝
║     ██║   ██║     ██║███████╗█████╗██║     ██║   ██║   █████╗
║     ██║   ██║██   ██║╚════██║╚════╝██║     ██║   ██║   ██╔══╝
║     ╚██████╔╝╚█████╔╝███████║      ███████╗██║   ██║   ███████╗
║      ╚═════╝  ╚════╝ ╚══════╝      ╚══════╝╚═╝   ╚═╝   ╚══════╝
║                                                           ║
║                    Open Job Spec - Lite                  ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
`
	fmt.Print(banner)
	fmt.Printf("  Version:            %s\n", core.OJSVersion)
	fmt.Printf("  Backend:            memory (in-process)\n")
	fmt.Printf("  Conformance Level:  4\n")
	fmt.Println()
	fmt.Printf("  HTTP Server:        http://localhost:%s\n", cfg.Port)
	fmt.Printf("  gRPC Server:        localhost:%s\n", cfg.GRPCPort)
	fmt.Printf("  Admin UI:           http://localhost:%s/ojs/admin/\n", cfg.Port)
	fmt.Println()
	fmt.Printf("  API Endpoints:\n")
	fmt.Printf("    - Manifest:       http://localhost:%s/ojs/manifest\n", cfg.Port)
	fmt.Printf("    - Health:         http://localhost:%s/ojs/v1/health\n", cfg.Port)
	fmt.Printf("    - Admin Stats:    http://localhost:%s/ojs/v1/admin/stats\n", cfg.Port)
	fmt.Println()

	if cfg.APIKey != "" {
		fmt.Println("  🔒 Authentication:  ENABLED (API key required)")
	} else {
		fmt.Println("  ⚠️  Authentication:  DISABLED (development mode)")
	}
	fmt.Println()
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println()
}
