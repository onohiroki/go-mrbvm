package main

import (
	"context"
	"math/rand"
	"testing"

	"github.com/onohiroki/go-mrbc/mrbc"
)

func TestRandomProgramCompiles(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		source := randomProgram(rng)
		if _, err := mrbc.Compile(context.Background(), []byte(source), mrbc.Options{FilePath: "random.rb"}); err != nil {
			t.Fatalf("case %d does not compile: %v\n%s", i, err, source)
		}
	}
}
