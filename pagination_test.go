package mongogit

import "testing"

func TestNewPageInfo(t *testing.T) {
	c := Cursor("x")
	pi := NewPageInfo(5, c.Ref(), c.Ref(), true, false)
	if pi.TotalCount != 5 || !pi.HasNextPage || pi.HasPreviousPage {
		t.Fatalf("unexpected: %#v", pi)
	}
	if pi.StartCursor == nil || *pi.StartCursor != c {
		t.Fatalf("bad start cursor: %#v", pi.StartCursor)
	}
}

func TestPaginationClone(t *testing.T) {
	first := int64(3)
	p := CursorPagination{First: &first}.Wrap()
	cl := p.Clone()
	if cl.Cursor.First == p.Cursor.First {
		t.Fatalf("expected First to be deep-copied")
	}
	if *cl.Cursor.First != 3 {
		t.Fatalf("unexpected value: %v", *cl.Cursor.First)
	}
}
