package mongogit

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const idKey = "id"

var (
	findOptions = []*options.FindOptions{
		options.Find().SetAllowDiskUse(true),
	}

	aggregateOptions = []*options.AggregateOptions{
		options.Aggregate().SetAllowDiskUse(true),
	}
)

type mongoCollection struct {
	collection *mongo.Collection
}

func newMongoCollection(c *mongo.Collection) *mongoCollection {
	return &mongoCollection{collection: c}
}

func (c *mongoCollection) Client() *mongo.Collection {
	return c.collection
}

func (c *mongoCollection) Find(ctx context.Context, filter any, consumer Consumer, options ...*options.FindOptions) error {
	cursor, err := c.collection.Find(ctx, filter, append(findOptions, options...)...)
	if errors.Is(err, mongo.ErrNilDocument) || errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	if err != nil {
		return wrapError(err)
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	for {
		c := cursor.Next(ctx)
		if err := cursor.Err(); err != nil && !errors.Is(err, io.EOF) {
			return wrapError(err)
		}

		if !c {
			if err := consumer.Consume(nil); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			break
		}

		if err := consumer.Consume(cursor.Current); err != nil {
			return err
		}
	}
	return nil
}

func (c *mongoCollection) FindOne(ctx context.Context, filter any, consumer Consumer, options ...*options.FindOneOptions) error {
	raw, err := c.collection.FindOne(ctx, filter, options...).Raw()
	if err != nil {
		if errors.Is(err, mongo.ErrNilDocument) || errors.Is(err, mongo.ErrNoDocuments) {
			return ErrNotFound
		}
		return wrapError(err)
	}
	if err := consumer.Consume(raw); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (c *mongoCollection) Count(ctx context.Context, filter any, options ...*options.CountOptions) (int64, error) {
	count, err := c.collection.CountDocuments(ctx, filter, options...)
	if err != nil {
		return 0, wrapError(err)
	}
	return count, nil
}

func (c *mongoCollection) Aggregate(ctx context.Context, pipeline []any, consumer Consumer, options ...*options.AggregateOptions) error {
	cursor, err := c.collection.Aggregate(ctx, pipeline, append(aggregateOptions, options...)...)
	if errors.Is(err, mongo.ErrNilDocument) || errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	if err != nil {
		return wrapError(err)
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	for {
		c := cursor.Next(ctx)
		if err := cursor.Err(); err != nil && !errors.Is(err, io.EOF) {
			return wrapError(err)
		}

		if !c {
			if err := consumer.Consume(nil); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			break
		}

		if err := consumer.Consume(cursor.Current); err != nil {
			return err
		}
	}
	return nil
}

func (c *mongoCollection) CountAggregation(ctx context.Context, pipeline []any) (int64, error) {
	var result struct {
		Count int64 `bson:"count"`
	}
	p := append(pipeline, bson.M{"$count": "count"})
	cursor, err := c.collection.Aggregate(ctx, p)
	if err != nil {
		return 0, wrapError(err)
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()
	if !cursor.Next(ctx) {
		return 0, nil
	}
	if err := cursor.Decode(&result); err != nil {
		return 0, wrapError(err)
	}
	return result.Count, nil
}

func (c *mongoCollection) RemoveAll(ctx context.Context, f any) error {
	_, err := c.collection.DeleteMany(ctx, f)
	if err != nil {
		return wrapError(err)
	}
	return nil
}

func getCursor(raw bson.Raw) (*Cursor, error) {
	val, err := raw.LookupErr(idKey)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup cursor: %v", err.Error())
	}
	var s string
	if err := val.Unmarshal(&s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %v", err.Error())
	}
	c := Cursor(s)
	return &c, nil
}

func wrapError(err error) error {
	return ErrInternalBy(err)
}

func (c *mongoCollection) Paginate(ctx context.Context, rawFilter any, s *Sort, p *Pagination, consumer Consumer, opts ...*options.FindOptions) (*PageInfo, error) {
	if p == nil || (p.Cursor == nil && p.Offset == nil) {
		return nil, nil
	}

	pFilter, err := c.pageFilter(ctx, *p, s)
	if err != nil {
		return nil, ErrInternalBy(err)
	}

	filter := rawFilter
	if pFilter != nil {
		filter = and(rawFilter, "", pFilter)
	}

	return c.paginate(ctx, rawFilter, s, p, filter, consumer, opts)
}

func (c *mongoCollection) PaginateAggregation(ctx context.Context, pipeline []any, s *Sort, p *Pagination, consumer Consumer, opts ...*options.AggregateOptions) (*PageInfo, error) {
	if p == nil || p.Cursor == nil && p.Offset == nil {
		return nil, nil
	}

	pFilter, pOpts, err := c.aggregateFilter(ctx, *p, s)
	if err != nil {
		return nil, ErrInternalBy(err)
	}

	pPipeline := append(pipeline, pFilter...)

	cursor, err := c.collection.Aggregate(ctx, pPipeline, append([]*options.AggregateOptions{pOpts}, opts...)...)
	if err != nil {
		return nil, ErrInternalBy(fmt.Errorf("failed to find: %w", err))
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	count, err := c.CountAggregation(ctx, pipeline)
	if err != nil {
		return nil, ErrInternalBy(fmt.Errorf("failed to count: %w", err))
	}

	items, startCursor, endCursor, hasMore, err := consume(ctx, cursor, limit(*p))
	if err != nil {
		return nil, err
	}

	if p.Cursor != nil && p.Cursor.Last != nil {
		reverse(items)
	}

	for _, item := range items {
		if err := consumer.Consume(item); err != nil {
			return nil, err
		}
	}

	hasNextPage, hasPreviousPage := pageInfo(p, hasMore)

	return NewPageInfo(count, startCursor, endCursor, hasNextPage, hasPreviousPage), nil
}

// pageInfo follows the Relay GraphQL connection spec:
// ref: https://facebook.github.io/relay/graphql/connections.htm#sec-undefined.PageInfo.Fields
// If first is set, false can be returned unless it can be efficiently determined whether or not a previous page exists.
// If last is set, false can be returned unless it can be efficiently determined whether or not a next page exists.
// Returning absolutely false because the existing implementation cannot determine it efficiently.
func pageInfo(p *Pagination, hasMore bool) (bool, bool) {
	hasNextPage := (p.Cursor != nil && p.Cursor.First != nil || p.Offset != nil) && hasMore
	hasPreviousPage := (p.Cursor != nil && p.Cursor.Last != nil) && hasMore
	return hasNextPage, hasPreviousPage
}

func consume(ctx context.Context, cursor *mongo.Cursor, limit int64) ([]bson.Raw, *Cursor, *Cursor, bool, error) {
	i := int64(0)
	var startCursor, endCursor *Cursor
	var items []bson.Raw

	for cursor.Next(ctx) {
		if i < limit-1 {
			var item bson.Raw
			if err := cursor.Decode(&item); err != nil {
				return nil, nil, nil, false, ErrInternalBy(fmt.Errorf("failed to decode item: %w", err))
			}

			cur, err := getCursor(item)
			if err != nil {
				return nil, nil, nil, false, ErrInternalBy(fmt.Errorf("failed to get cursor: %w", err))
			}

			if startCursor == nil {
				startCursor = cur
			}
			endCursor = cur

			items = append(items, item)
		}

		i++
	}

	if err := cursor.Err(); err != nil {
		return nil, nil, nil, false, ErrInternalBy(fmt.Errorf("failed to read cursor: %w", err))
	}
	return items, startCursor, endCursor, i == limit, nil
}

func reverse(items []bson.Raw) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func (c *mongoCollection) aggregateFilter(ctx context.Context, p Pagination, s *Sort) ([]any, *options.AggregateOptions, error) {
	if p.Cursor == nil && p.Offset == nil {
		return nil, nil, errors.New("invalid pagination")
	}

	stages := []any{bson.M{"$sort": sortFilter(p, s)}}

	if p.Offset != nil {
		stages = append(stages, bson.M{"$skip": p.Offset.Offset})
	}

	f, err := c.pageFilter(ctx, p, s)
	if err != nil {
		return nil, nil, err
	}
	if f != nil {
		stages = append(stages, bson.M{"$match": f})
	}
	return append(stages, bson.M{"$limit": limit(p)}), aggregateOptionsFromPagination(p, s), err
}

func aggregateOptionsFromPagination(_ Pagination, _ *Sort) *options.AggregateOptions {
	collation := options.Collation{
		Locale:   "en",
		Strength: 2,
	}
	return options.Aggregate().SetAllowDiskUse(true).SetCollation(&collation)
}

func (c *mongoCollection) pageFilter(ctx context.Context, p Pagination, s *Sort) (bson.M, error) {
	if p.Cursor == nil {
		return nil, nil
	}

	var filter bson.M
	sortKey := idKey
	sortOrder := 1

	if s != nil && s.Key != "" {
		sortKey = s.Key
		if s.Reverted {
			sortOrder = -1
		}
	}

	var cursor *Cursor
	var op string

	if p.Cursor.After != nil {
		cursor = p.Cursor.After
		op = "$gt"
	} else if p.Cursor.Before != nil {
		cursor = p.Cursor.Before
		op = "$lt"
	}

	if cursor != nil {
		cursorDoc, err := c.getCursorDocument(ctx, *cursor)
		if err != nil {
			return nil, err
		}

		filter = bson.M{
			"$or": []bson.M{
				{sortKey: bson.M{op: cursorDoc[sortKey]}},
				{
					sortKey: cursorDoc[sortKey],
					idKey:   bson.M{op: cursorDoc[idKey]},
				},
			},
		}

		if sortOrder == -1 {
			if op == "$gt" {
				op = "$lt"
			} else {
				op = "$gt"
			}
			filter = bson.M{
				"$or": []bson.M{
					{sortKey: bson.M{op: cursorDoc[sortKey]}},
					{
						sortKey: cursorDoc[sortKey],
						idKey:   bson.M{op: cursorDoc[idKey]},
					},
				},
			}
		}
	}

	return filter, nil
}

func (c *mongoCollection) getCursorDocument(ctx context.Context, cursor Cursor) (bson.M, error) {
	var cursorDoc bson.M
	err := c.collection.FindOne(ctx, bson.M{idKey: cursor}).Decode(&cursorDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to find cursor element: %w", err)
	}
	return cursorDoc, nil
}

func sortFilter(p Pagination, s *Sort) bson.D {
	var sortOptions bson.D
	if s != nil && s.Key != "" && s.Key != idKey {
		sortOptions = append(sortOptions, bson.E{Key: s.Key, Value: sortDirection(p, s)})
	}
	return append(sortOptions, bson.E{Key: idKey, Value: sortDirection(p, s)})
}

func limit(p Pagination) int64 {
	const defaultLimit = 20
	var limit *int64

	if p.Offset != nil {
		limit = &p.Offset.Limit
	} else if p.Cursor != nil {
		if p.Cursor.First != nil {
			limit = p.Cursor.First
		} else if p.Cursor.Last != nil {
			limit = p.Cursor.Last
		}
	}

	if limit != nil && *limit > 0 {
		// Read one more element so that we can see whether there's a further one
		return *limit + 1
	}

	return defaultLimit + 1
}

func sortDirection(p Pagination, s *Sort) int {
	reverted := false
	if s != nil {
		reverted = s.Reverted
	}

	reverted = reverted || (p.Cursor != nil && p.Cursor.Last != nil)

	if reverted {
		return -1
	}
	return 1
}

func (c *mongoCollection) paginate(ctx context.Context, rawFilter any, s *Sort, p *Pagination, filter any, consumer Consumer, opts []*options.FindOptions) (*PageInfo, error) {
	sortKey := idKey
	sortOrder := 1
	if s != nil && s.Key != "" {
		sortKey = s.Key
		if s.Reverted {
			sortOrder = -1
		}
	}

	if p.Cursor != nil && p.Cursor.Last != nil {
		sortOrder *= -1
	}

	sort := bson.D{{Key: sortKey, Value: sortOrder}}
	if sortKey != idKey {
		sort = append(sort, bson.E{Key: idKey, Value: sortOrder})
	}

	findOpts := options.Find().
		SetSort(sort).
		SetLimit(limit(*p))

	if p.Offset != nil {
		findOpts.SetSkip(p.Offset.Offset)
	}

	cursor, err := c.collection.Find(ctx, filter, append([]*options.FindOptions{findOpts}, opts...)...)
	if err != nil {
		return nil, ErrInternalBy(fmt.Errorf("failed to find: %w", err))
	}
	defer func() {
		_ = cursor.Close(ctx)
	}()

	count, err := c.collection.CountDocuments(ctx, rawFilter)
	if err != nil {
		return nil, ErrInternalBy(fmt.Errorf("failed to count: %w", err))
	}

	items, startCursor, endCursor, hasMore, err := consume(ctx, cursor, limit(*p))
	if err != nil {
		return nil, err
	}

	if p.Cursor != nil && p.Cursor.Last != nil {
		reverse(items)
		startCursor, endCursor = endCursor, startCursor
	}

	for _, item := range items {
		if err := consumer.Consume(item); err != nil {
			return nil, err
		}
	}

	hasNextPage, hasPreviousPage := pageInfo(p, hasMore)

	return NewPageInfo(count, startCursor, endCursor, hasNextPage, hasPreviousPage), nil
}
