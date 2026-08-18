package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"CitadelDesktop/Server/Accounts"
	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/PrivateMetrics"
)

type HostedOptions struct {
	DataRoot                string
	Offline                 bool
	StaticConfigPath        string
	Dynamic                 bool
	CellID                  string
	MaxRuntimes             int
	ControlTokenEnvironment string
	SessionKeyEnvironment   string
	PrivateMetricsURL       string
	CheckpointURL           string
	DashboardOrigins        string
	SecureCookies           bool
}

func hostedModeEnabled(dynamic bool, staticConfigPath string) bool {
	return dynamic || strings.TrimSpace(staticConfigPath) != ""
}

func runHosted(rootContext context.Context, listener net.Listener, options HostedOptions) error {
	if listener == nil {
		return fmt.Errorf("hosted listener is required")
	}
	if options.Dynamic && strings.TrimSpace(options.StaticConfigPath) != "" {
		return fmt.Errorf("dynamic hosted mode and a static tenant config cannot be enabled together")
	}
	if options.MaxRuntimes < 1 || options.MaxRuntimes > 64 {
		return fmt.Errorf("tenant max runtimes must be between 1 and 64")
	}
	if strings.TrimSpace(options.PrivateMetricsURL) != "" && !options.Dynamic {
		return fmt.Errorf("private metrics publishing requires dynamic hosted mode")
	}
	if strings.TrimSpace(options.CheckpointURL) != "" && strings.TrimSpace(options.PrivateMetricsURL) == "" {
		return fmt.Errorf("dashboard checkpoints require the private metrics endpoint and its runtime grants")
	}
	dashboardOrigins, originErr := Accounts.NewDashboardOriginPolicy([]string{options.DashboardOrigins})
	if originErr != nil {
		return originErr
	}
	startupContext, cancelStartup := context.WithTimeout(rootContext, 90*time.Second)
	defer cancelStartup()

	var loaded Accounts.LoadedTenantConfig
	var err error
	if strings.TrimSpace(options.StaticConfigPath) != "" {
		loaded, err = Accounts.LoadTenantConfig(options.StaticConfigPath, os.LookupEnv)
		if err != nil {
			return err
		}
		options.MaxRuntimes = loaded.MaxAccounts
	}
	privateMetricsClient, err := PrivateMetrics.NewClient(PrivateMetrics.ClientConfig{
		Endpoint: options.PrivateMetricsURL, CheckpointEndpoint: options.CheckpointURL, ClientVersion: App.Version,
	})
	if err != nil {
		return err
	}
	supervisor, err := Accounts.New(startupContext, Accounts.Config{
		DataRoot: options.DataRoot, MaxAccounts: options.MaxRuntimes,
		Offline: options.Offline, RuntimeContext: rootContext,
		PrivateMetricsClient: privateMetricsClient,
		UpdateEndpoint:       os.Getenv("CITADEL_UPDATE_URL"), UpdateInstallSupported: false,
	})
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if closed {
			return
		}
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = supervisor.Close(shutdownContext)
	}()
	if supervisor.StartupError() != nil {
		log.Printf("Hosted shared services started degraded: %v", supervisor.StartupError())
	}
	supervisor.Start()

	var dashboardAuth *Accounts.TenantAuthenticator
	var orchestrator *Accounts.Orchestrator
	if options.Dynamic {
		sessionKey := []byte(os.Getenv(options.SessionKeyEnvironment))
		dashboardAuth, err = Accounts.NewDynamicTenantAuthenticator(sessionKey, options.SecureCookies)
		if err != nil {
			return err
		}
		controlToken, exists := os.LookupEnv(options.ControlTokenEnvironment)
		if !exists {
			return fmt.Errorf("hosted orchestrator token environment %q is not set", options.ControlTokenEnvironment)
		}
		orchestrator, err = Accounts.NewOrchestrator(Accounts.OrchestratorConfig{
			CellID: options.CellID, Token: controlToken,
			Supervisor: supervisor, DashboardAuth: dashboardAuth,
		})
		if err != nil {
			return err
		}
		orchestrator.Start(rootContext)
	} else {
		dashboardAuth, err = Accounts.NewTenantAuthenticator(loaded, options.SecureCookies)
		if err != nil {
			return err
		}
		for _, account := range loaded.Accounts {
			if _, err := supervisor.AddAccount(startupContext, Accounts.AccountConfig{
				ID: string(account.ID), BackgroundOnly: true, StartSession: account.StartSession,
			}); err != nil {
				return fmt.Errorf("start static runtime %q: %w", account.ID, err)
			}
		}
	}
	cancelStartup()

	frontend := frontendHandler("Client/dist")
	dashboardAuth.SetDashboardOrigins(dashboardOrigins)
	mux := http.NewServeMux()
	mux.Handle("/tenant/login", dashboardAuth.LoginHandler())
	mux.Handle("/accounts/", supervisor.HandlerWithOrigins(dashboardAuth, frontend, dashboardOrigins))
	if orchestrator != nil {
		mux.Handle("/orchestrator/", orchestrator.Handler())
	}
	// Frontend assets contain no account data and can be cached/shared. Every
	// state, command, and event request still enters through an authenticated
	// /accounts/{runtimeId} shard.
	mux.Handle("/", frontend)
	server := &http.Server{
		Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second,
		WriteTimeout: 0,
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	composition := "static"
	if options.Dynamic {
		composition = "dynamic"
	}
	log.Printf("CitadelOps %s hosted %s cell %s listening at %s", App.Version, composition, options.CellID, listener.Addr())
	select {
	case <-rootContext.Done():
	case serveErr := <-serveErrors:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			return fmt.Errorf("hosted server stopped: %w", serveErr)
		}
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownContext)
	closeErr := supervisor.Close(shutdownContext)
	closed = true
	return closeErr
}
