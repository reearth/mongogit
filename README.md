# mongogit

Git-style versioned documents for MongoDB, in Go.

`mongogit` stores documents with version history. Each write creates a new
immutable version identified by a UUID, with links to its parent versions and
a set of named refs (such as `latest`). You can read a document at any version
or ref and walk its history, archive it, and move refs between versions.

## Install

```
go get github.com/reearth/mongogit
```

Requires Go 1.24 or newer and a MongoDB server.

## Usage

```go
package main

import (
	"context"

	"github.com/reearth/mongogit"
	"github.com/reearth/mongogit/version"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type Item struct {
	ID   string `bson:"id"`
	Name string `bson:"name"`
}

func run(ctx context.Context, raw *mongo.Collection) error {
	coll := mongogit.NewCollection(raw)

	// Create the first version of item "a" (nil parent = a fresh "latest").
	if err := coll.SaveOne(ctx, "a", Item{ID: "a", Name: "hello"}, nil); err != nil {
		return err
	}

	// Read the latest version.
	c := mongogit.SliceConsumer[Item]{}
	if err := coll.FindOne(ctx, bson.M{"id": "a"}, version.Eq(version.Latest.OrVersion()), &c); err != nil {
		return err
	}
	_ = c.Result // []Item holding the latest version's data
	return nil
}
```

`NewCollection` takes a raw `*mongo.Collection` from the official driver, so the
library composes with any existing connection setup. Queries take a
`version.Query` (`version.All()` or `version.Eq(...)`) to select which versions
to return.

## Versioning model

- **Version** — a UUID assigned to each write; immutable once created.
- **Parents** — the versions a write descends from, forming the history graph.
- **Refs** — named pointers to a version. `latest` tracks the newest write; you
  can add, move, and delete your own refs with `UpdateRef`.
- **Archived** — a document can be archived; writes to an archived document
  return `version.ErrArchived`.

## Running tests

The tests need a running MongoDB. They read its connection string from the
`MONGO_URI` environment variable and skip (rather than fail) when it is unset:

```
docker run -d --rm -p 27017:27017 --name mongogit-test mongo:6
MONGO_URI="mongodb://localhost:27017" go test ./...
docker stop mongogit-test
```

## License

MIT. See [LICENSE](LICENSE).
