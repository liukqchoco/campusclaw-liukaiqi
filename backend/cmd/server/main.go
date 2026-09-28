package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"campusclaw/backend/internal/config"
	"campusclaw/backend/internal/httpapi"
	"campusclaw/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("配置加载失败", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o750); err != nil {
		logger.Error("创建上传目录失败", "error", err)
		os.Exit(1)
	}

	dataStore, err := store.Open(cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword)
	if err != nil {
		logger.Error("数据库配置失败", "error", err)
		os.Exit(1)
	}
	defer dataStore.Close()

	startupContext, startupCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer startupCancel()
	if err := dataStore.WaitForDatabase(startupContext, 45, 2*time.Second); err != nil {
		logger.Error("数据库连接失败", "error", err)
		os.Exit(1)
	}
	if err := dataStore.Migrate(startupContext); err != nil {
		logger.Error("数据库迁移失败", "error", err)
		os.Exit(1)
	}
	if err := dataStore.Seed(startupContext, cfg.TeacherPassword, cfg.StudentPassword, cfg.UploadDir); err != nil {
		logger.Error("种子数据初始化失败", "error", err)
		os.Exit(1)
	}

	api, err := httpapi.New(cfg, dataStore, logger)
	if err != nil {
		logger.Error("API 初始化失败", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.APIPort,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	runContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("CampusClaw API 已启动", "port", cfg.APIPort)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-runContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			logger.Error("API 关闭失败", "error", err)
		}
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("API 运行失败", "error", err)
			os.Exit(1)
		}
	}
}
