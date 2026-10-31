package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	accessstore "github.com/mothercat-enterprise/mothercat/backend/services/access/monolith"
	auditstore "github.com/mothercat-enterprise/mothercat/backend/services/audit/monolith"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/persistence"
	organizationstore "github.com/mothercat-enterprise/mothercat/backend/services/organization/monolith"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	if err := run(); err != nil {
		slog.Error("MongoDB setup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	uri := strings.TrimSpace(os.Getenv("MONGODB_URI"))
	name := strings.TrimSpace(os.Getenv("MONGODB_DATABASE"))
	if uri == "" || name == "" || strings.ContainsAny(name, "/\\.") {
		return errors.New("MONGODB_URI and a valid MONGODB_DATABASE are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return errors.New("connect to MongoDB")
	}
	defer func() {
		disconnectCtx, cancelDisconnect := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelDisconnect()
		if err := client.Disconnect(disconnectCtx); err != nil {
			slog.Error("disconnect MongoDB client", "error", err)
		}
	}()
	if err := client.Ping(ctx, nil); err != nil {
		return errors.New("ping MongoDB")
	}
	db := client.Database(name)
	if err := persistence.EnsureIndexes(ctx, db); err != nil {
		return err
	}
	if err := organizationstore.EnsureIndexes(ctx, db); err != nil {
		return err
	}
	if err := accessstore.EnsureIndexes(ctx, db); err != nil {
		return err
	}
	if err := auditstore.EnsureIndexes(ctx, db); err != nil {
		return err
	}
	slog.Info("MongoDB indexes and baseline Staff role are ready", "database", name)
	return nil
}
