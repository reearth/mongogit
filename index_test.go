package mongogit

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestIndexModel(t *testing.T) {
	i := Index{Name: "x", Key: bson.D{{Key: "a", Value: 1}}, Unique: true}
	m := i.Model()
	if m.Options == nil || m.Options.Name == nil || *m.Options.Name != "x" {
		t.Fatalf("bad model: %#v", m.Options)
	}
}

func TestIndexEqual(t *testing.T) {
	a := Index{Name: "x", Key: bson.D{{Key: "a", Value: 1}}}
	b := Index{Name: "x", Key: bson.D{{Key: "a", Value: 1}}}
	if !a.Equal(b) {
		t.Fatalf("expected equal")
	}
}
