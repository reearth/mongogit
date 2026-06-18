package mongogit

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestAnd(t *testing.T) {
	got := and(bson.M{"a": 1}, "b", bson.M{"$gt": 2})
	m, ok := got.(bson.M)
	if !ok || m["a"] != 1 {
		t.Fatalf("unexpected: %#v", got)
	}
	if _, ok := m["b"]; !ok {
		t.Fatalf("missing key b: %#v", got)
	}
}

func TestAppendE(t *testing.T) {
	got := appendE(bson.M{"a": 1}, bson.E{Key: "b", Value: 2})
	m := got.(bson.M)
	if m["a"] != 1 || m["b"] != 2 {
		t.Fatalf("unexpected: %#v", got)
	}
}
