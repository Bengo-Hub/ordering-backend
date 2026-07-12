//go:build ignore

// rehash_atlas.go recomputes atlas.sum for the migrations directory using atlas's
// OWN canonical checksum algorithm (via the ariga.io/atlas library that is already a
// project dependency). The previous hand-rolled sha256 implementation computed the
// directory-total hash differently from atlas, so `atlas migrate` / the ent migrate
// tool rejected the resulting atlas.sum with "checksum mismatch". Using the library
// guarantees the sum matches what atlas validates against.
//
// Run from ordering-backend root: go run ./scripts/rehash_atlas.go
package main

import (
	"fmt"

	"ariga.io/atlas/sql/migrate"
)

func main() {
	const dir = "internal/ent/migrate/migrations"
	d, err := migrate.NewLocalDir(dir)
	if err != nil {
		panic(err)
	}
	sum, err := d.Checksum()
	if err != nil {
		panic(err)
	}
	if err := migrate.WriteSumFile(d, sum); err != nil {
		panic(err)
	}
	fmt.Printf("atlas.sum rehashed (%d files) via atlas lib\n", len(sum))
}
