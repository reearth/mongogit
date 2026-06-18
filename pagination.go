package mongogit

type Cursor string

func CursorFromRef(c *string) *Cursor {
	if c == nil {
		return nil
	}
	d := Cursor(*c)
	return &d
}

func (c Cursor) Ref() *Cursor {
	return &c
}

func (c *Cursor) CopyRef() *Cursor {
	if c == nil {
		return nil
	}
	d := *c
	return &d
}

func (c *Cursor) StringRef() *string {
	if c == nil {
		return nil
	}
	s := string(*c)
	return &s
}

// CursorPagination is Relay-style cursor pagination.
type CursorPagination struct {
	Before *Cursor
	After  *Cursor
	First  *int64
	Last   *int64
}

func (p *CursorPagination) Clone() *CursorPagination {
	if p == nil {
		return nil
	}
	return &CursorPagination{
		Before: p.Before.CopyRef(),
		After:  p.After.CopyRef(),
		First:  cloneInt64(p.First),
		Last:   cloneInt64(p.Last),
	}
}

func (p CursorPagination) Wrap() *Pagination {
	return &Pagination{Cursor: &p}
}

type OffsetPagination struct {
	Offset int64
	Limit  int64
}

func (p OffsetPagination) Wrap() *Pagination {
	return &Pagination{Offset: &p}
}

type Pagination struct {
	Cursor *CursorPagination
	Offset *OffsetPagination
}

func (p *Pagination) Clone() *Pagination {
	if p == nil {
		return nil
	}
	var offset *OffsetPagination
	if p.Offset != nil {
		o := *p.Offset
		offset = &o
	}
	return &Pagination{
		Cursor: p.Cursor.Clone(),
		Offset: offset,
	}
}

type Sort struct {
	Key      string
	Reverted bool
}

type PageInfo struct {
	TotalCount      int64
	StartCursor     *Cursor
	EndCursor       *Cursor
	HasNextPage     bool
	HasPreviousPage bool
}

func NewPageInfo(totalCount int64, startCursor, endCursor *Cursor, hasNextPage, hasPreviousPage bool) *PageInfo {
	return &PageInfo{
		TotalCount:      totalCount,
		StartCursor:     startCursor.CopyRef(),
		EndCursor:       endCursor.CopyRef(),
		HasNextPage:     hasNextPage,
		HasPreviousPage: hasPreviousPage,
	}
}

func EmptyPageInfo() *PageInfo {
	return &PageInfo{}
}

func (p *PageInfo) OrEmpty() *PageInfo {
	if p == nil {
		return EmptyPageInfo()
	}
	return p
}

func (p *PageInfo) Clone() *PageInfo {
	if p == nil {
		return nil
	}
	return &PageInfo{
		TotalCount:      p.TotalCount,
		StartCursor:     p.StartCursor.CopyRef(),
		EndCursor:       p.EndCursor.CopyRef(),
		HasNextPage:     p.HasNextPage,
		HasPreviousPage: p.HasPreviousPage,
	}
}

func cloneInt64(v *int64) *int64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
