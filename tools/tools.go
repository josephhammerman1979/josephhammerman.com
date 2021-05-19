// +build tools

// NOTE: the above blank line after the build tools line is IMPORTANT. Leave it.
// https://golang.org/pkg/go/build/#hdr-Build_Constraints
package tools

// place "tools" here. You can then use them with `go run`
//
//
// Note that if the tool has a dependency on a project requiring CGO,
// you will probably also need to `go get` it in the Makefile somewhere
// as go's vendoring doesn't pull in directories without .go files which
// can leave CGO things out.
// Still include it here, though, and then don't use `GO111MODULE=off` with
// the `go get` and it should be good.
import (
	_ "github.com/kisielk/errcheck"
	_ "github.com/mgechev/revive"
	// uncomment for mockery
	// _ "github.com/vektra/mockery/v2"
)
