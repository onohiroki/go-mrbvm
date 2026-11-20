module github.com/onohiroki/go-mrbvm

go 1.24

require github.com/onohiroki/go-mrbc v0.0.0

// Local development against the sibling checkout. Replace this with a
// tagged go-mrbc release once one is available.
replace github.com/onohiroki/go-mrbc => ../go-mrbc
