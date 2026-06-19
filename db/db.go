package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/umangagarwal/vedx-backend/config"
)

func NewPool(cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("unable to parse DSN: %w", err)
	}

	// Supabase pooler (port 6543) runs PgBouncer in transaction mode.
	// Transaction mode does not support extended query protocol / prepared statements.
	// Switching to SimpleProtocol sends plain SQL text — compatible with any pooler.
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	// Keep 2 connections warm so concurrent page-load calls never wait.
	poolCfg.MinConns = 2
	poolCfg.MaxConns = 10

	// Recycle connections before Supabase's idle-eviction window (~5 min).
	poolCfg.MaxConnIdleTime = 3 * time.Minute
	poolCfg.MaxConnLifetime = 30 * time.Minute

	// Health-check interval — quickly detects and evicts dead connections.
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to reach database: %w", err)
	}
	return pool, nil
}
