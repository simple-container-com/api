package mongodb

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DropDatabase connects to MongoDB using the provided URI and drops the specified database.
// The URI must include user credentials with dbAdmin privileges on the target database.
func DropDatabase(ctx context.Context, mongoUri, dbName string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(timeoutCtx, options.Client().ApplyURI(mongoUri))
	if err != nil {
		return errors.Wrapf(err, "failed to connect to MongoDB to drop database %q", dbName)
	}
	defer client.Disconnect(timeoutCtx) //nolint:errcheck

	if err := client.Database(dbName).Drop(timeoutCtx); err != nil {
		return errors.Wrapf(err, "failed to drop MongoDB database %q", dbName)
	}
	return nil
}
