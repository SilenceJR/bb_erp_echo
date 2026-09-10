package file

import "testing"

func TestProductImageOwnerUsesProductModule(t *testing.T) {
	if OwnerProduct != "product" || OwnerMold != "mold" {
		t.Fatalf("owner constants changed: %q %q", OwnerProduct, OwnerMold)
	}
	if !validOwnerType(OwnerProduct) || !validOwnerType(OwnerMold) {
		t.Fatal("product and mold owner types must be valid")
	}
}
