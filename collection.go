package mongogit

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/reearth/mongogit/internal/clock"
	"github.com/reearth/mongogit/version"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Collection struct {
	client *mongoCollection
}

func NewCollection(c *mongo.Collection) *Collection {
	return &Collection{client: newMongoCollection(c)}
}

func (c *Collection) Client() *mongo.Collection {
	return c.client.Client()
}

func (c *Collection) FindOne(
	ctx context.Context,
	filter any,
	q version.Query,
	consumer Consumer,
) error {
	return c.client.FindOne(ctx, apply(q, filter), consumer)
}

func (c *Collection) Find(
	ctx context.Context,
	filter any,
	q version.Query,
	consumer Consumer,
) error {
	opt := options.Find().SetCollation(&options.Collation{
		Locale:   "simple",
		Strength: 1,
	})
	return c.client.Find(ctx, apply(q, filter), consumer, opt)
}

func (c *Collection) Aggregate(
	ctx context.Context,
	pipeline []any,
	q version.Query,
	consumer Consumer,
) error {
	opt := options.Aggregate().SetCollation(&options.Collation{
		Locale:   "simple",
		Strength: 1,
	})
	return c.client.Aggregate(ctx, applyToPipeline(q, pipeline), consumer, opt)
}

func (c *Collection) Paginate(
	ctx context.Context,
	filter any,
	q version.Query,
	s *Sort,
	p *Pagination,
	consumer Consumer,
) (*PageInfo, error) {
	return c.client.Paginate(ctx, apply(q, filter), s, p, consumer)
}

func (c *Collection) Count(ctx context.Context, filter any, q version.Query) (int64, error) {
	return c.client.Count(ctx, apply(q, filter))
}

func (c *Collection) PaginateAggregation(
	ctx context.Context,
	pipeline []any,
	q version.Query,
	s *Sort,
	p *Pagination,
	consumer Consumer,
) (*PageInfo, error) {
	opt := options.Aggregate().SetCollation(&options.Collation{
		Locale:   "simple",
		Strength: 1,
	})
	return c.client.PaginateAggregation(ctx, applyToPipeline(q, pipeline), s, p, consumer, opt)
}

func (c *Collection) CountAggregation(
	ctx context.Context,
	pipeline []any,
	q version.Query,
) (int64, error) {
	return c.client.CountAggregation(ctx, applyToPipeline(q, pipeline))
}

func (c *Collection) SaveOne(
	ctx context.Context,
	id string,
	d any,
	parent *version.VersionOrRef,
) error {
	q := bson.M{
		"id":    id,
		metaKey: true,
	}
	if archived, err := c.IsArchived(ctx, q); err != nil {
		return err
	} else if archived {
		return version.ErrArchived
	}

	actualVr := lo.FromPtrOr(parent, version.Latest.OrVersion())
	meta, err := c.meta(ctx, id, actualVr.Ref())
	if err != nil {
		return err
	}

	var refs []version.Ref
	actualVr.Match(nil, func(r version.Ref) { refs = []version.Ref{r} })
	newmeta := Meta{
		ObjectID: primitive.NewObjectIDFromTimestamp(clock.Now()),
		Version:  version.New(),
		Refs:     refs,
	}
	if meta == nil {
		if !actualVr.IsRef(version.Latest) {
			return ErrNotFound // invalid dest
		}
	} else {
		newmeta.Parents = []version.Version{meta.Version}
	}

	if err := version.MatchVersionOrRef(actualVr, nil, func(r version.Ref) error {
		return c.DeleteRef(ctx, []string{id}, r)
	}); err != nil {
		return err
	}

	if _, err := c.client.Client().InsertOne(ctx, &Document[any]{
		Data: d,
		Meta: newmeta,
	}); err != nil {
		return ErrInternalBy(err)
	}

	return nil
}

func (c *Collection) SaveMany(ctx context.Context, ids []string, docs []any) error {
	// validate input
	if len(ids) != len(docs) {
		return ErrInvalidParams
	}
	if len(ids) == 0 {
		return nil
	}

	// check if any of the ids are archived
	q := bson.M{"id": bson.M{"$in": ids}}
	if archived, err := c.IsArchived(ctx, q); err != nil {
		return err
	} else if archived {
		return version.ErrArchived
	}

	// load items metas
	oldMetas, err := c.metas(ctx, ids)
	if err != nil {
		return err
	}

	newDocs := make([]any, len(ids))

	for i := 0; i < len(ids); i++ {
		id, doc := ids[i], docs[i]

		newMeta := Meta{
			ObjectID: primitive.NewObjectIDFromTimestamp(clock.Now()),
			Version:  version.New(),
			Refs:     []version.Ref{version.Latest},
		}

		meta := oldMetas[id]
		if meta != nil {
			newMeta.Parents = []version.Version{meta.Version}
		}

		newDocs[i] = &Document[any]{
			Data: doc,
			Meta: newMeta,
		}
	}

	// remove latest ref from old docs
	if err := c.DeleteRef(ctx, ids, version.Latest); err != nil {
		return err
	}

	if _, err := c.client.Client().InsertMany(ctx, newDocs); err != nil {
		return ErrInternalBy(err)
	}
	return nil
}

func (c *Collection) SaveAll(
	ctx context.Context,
	ids []string,
	docs []any,
	parents []*version.VersionOrRef,
) error {
	// TODO: optimize to use bulk write
	if len(ids) != len(docs) || (parents != nil && len(ids) != len(parents)) {
		return ErrInvalidParams
	}
	if len(ids) == 0 {
		return nil
	}
	for i := 0; i < len(ids); i++ {
		var parent *version.VersionOrRef = nil
		if parents != nil {
			parent = parents[i]
		}
		err := c.SaveOne(ctx, ids[i], docs[i], parent)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Collection) DeleteRef(ctx context.Context, ids []string, ref version.Ref) error {
	if _, err := c.client.Client().UpdateMany(ctx, bson.M{
		"id":    bson.M{"$in": ids},
		refsKey: bson.M{"$in": []string{ref.String()}},
	}, bson.M{
		"$pull": bson.M{refsKey: ref},
	}); err != nil {
		return ErrInternalBy(err)
	}
	return nil
}

func (c *Collection) UpdateRef(
	ctx context.Context,
	id string,
	ref version.Ref,
	dest *version.VersionOrRef,
) error {
	if err := c.DeleteRef(ctx, []string{id}, ref); err != nil {
		return err
	}

	if dest != nil {
		if _, err := c.client.Client().UpdateOne(ctx, apply(version.Eq(*dest), bson.M{
			"id": id,
		}), bson.M{
			"$push": bson.M{refsKey: ref},
		}); err != nil {
			return ErrInternalBy(err)
		}
	}

	return nil
}

func (c *Collection) IsArchived(ctx context.Context, filter any) (bool, error) {
	cons := SliceConsumer[MetadataDocument]{}
	q := and(filter, "", bson.M{
		metaKey: true,
	})

	if err := c.client.Find(ctx, q, &cons); err != nil {
		if errors.Is(err, ErrNotFound) || err == io.EOF {
			return false, nil
		}
		return false, err
	}
	for _, m := range cons.Result {
		if m.Archived {
			return true, nil
		}
	}
	return false, nil
}

func (c *Collection) ArchiveOne(ctx context.Context, filter bson.M, archived bool) error {
	f := and(filter, "", bson.M{metaKey: true})

	if !archived {
		_, err := c.client.Client().DeleteOne(ctx, f)
		if err != nil {
			return ErrInternalBy(err)
		}
		return nil
	}

	_, err := c.client.Client().ReplaceOne(ctx, f, lo.Assign(bson.M{
		metaKey:    true,
		"archived": archived,
	}, filter), options.Replace().SetUpsert(true))
	if err != nil {
		return ErrInternalBy(err)
	}
	return nil
}

func (c *Collection) Timestamp(
	ctx context.Context,
	filter any,
	q version.Query,
) (time.Time, error) {
	consumer := SliceConsumer[Meta]{}
	f := apply(q, filter)
	if err := c.client.Find(ctx, f, &consumer, options.Find().SetLimit(1).SetSort(bson.D{{Key: "_id", Value: -1}})); err != nil {
		return time.Time{}, err
	}
	if len(consumer.Result) == 0 {
		return time.Time{}, ErrNotFound
	}
	return consumer.Result[0].Timestamp(), nil
}

func (c *Collection) RemoveOne(ctx context.Context, filter any) error {
	return c.client.RemoveAll(ctx, filter)
}

func (c *Collection) Empty(ctx context.Context) error {
	return c.client.Client().Drop(ctx)
}

func (c *Collection) Indexes() []Index {
	return []Index{
		{
			Name:   "mongogit_id",
			Key:    bson.D{{Key: "id", Value: 1}, {Key: versionKey, Value: 1}},
			Unique: true,
		},
		{
			Name:   "mongogit_id_meta",
			Key:    bson.D{{Key: "id", Value: 1}, {Key: metaKey, Value: 1}},
			Unique: true,
			Filter: bson.M{metaKey: true},
		},
		{
			Name: "mongogit_id_refs",
			Key:  bson.D{{Key: "id", Value: 1}, {Key: refsKey, Value: 1}},
		},
		{
			Name: "mongogit_refs",
			Key:  bson.D{{Key: refsKey, Value: 1}},
		},
		{
			Name: "mongogit_parents",
			Key:  bson.D{{Key: parentsKey, Value: 1}},
		},
		{
			Name: "mongogit_meta_refs",
			Key:  bson.D{{Key: metaKey, Value: 1}, {Key: refsKey, Value: 1}},
		},
	}
}

func (c *Collection) meta(ctx context.Context, id string, v *version.VersionOrRef) (*Meta, error) {
	consumer := SliceConsumer[Meta]{}
	q := apply(version.Eq(lo.FromPtrOr(v, version.Latest.OrVersion())), bson.M{
		"id": id,
	})
	if err := c.client.FindOne(ctx, q, &consumer); err != nil {
		if errors.Is(err, ErrNotFound) && (v == nil || v.IsRef(version.Latest)) {
			return nil, nil
		}
		return nil, err
	}
	return &consumer.Result[0], nil
}

func (c *Collection) metas(ctx context.Context, ids []string) (map[string]*Meta, error) {
	type idDoc struct {
		ID string `bson:"id"`
	}
	consumer := SliceConsumer[Document[*idDoc]]{}
	q := apply(version.Eq(version.Latest.OrVersion()), bson.M{
		"id": bson.M{"$in": ids},
	})
	if err := c.client.Find(ctx, q, &consumer); err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return lo.SliceToMap(
		consumer.Result,
		func(m Document[*idDoc]) (string, *Meta) { return m.Data.ID, &m.Meta },
	), nil
}
