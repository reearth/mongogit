# mongogit

Git-style versioned documents for MongoDB, in Go.

`mongogit` stores documents with version history — each write creates a new
immutable version with parents and named refs (like `latest`), so you can read
any version or ref of a document and walk its history.

## Install

```
go get github.com/reearth/mongogit
```

## Status

Extracted from [reearthx](https://github.com/reearth/reearthx). MIT licensed.
